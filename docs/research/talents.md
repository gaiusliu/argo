# Skill 库（talents）设计调研

> 调研对象：Claude-Code、OpenCode、Hermes Agent、CrewAI、OpenClaw、Nanoclaw、pi

---

## 1. 全局共识：Skill = Markdown 文件 + YAML Frontmatter

七个项目对 "Skill" 的定义惊人地一致——**基于 Markdown 的指令文件，通过 YAML frontmatter 声明元数据，通过系统提示注入 LLM 上下文**。这不是巧合，Anthropic 的 [agentskills.io](https://agentskills.io) 规范已成为事实标准。

核心共识：
- Skill 是**内容机制**，不是**代码机制**——它们是注入到系统提示中的 Markdown 指令
- **SKILL.md 文件名**是标准
- **YAML frontmatter**（name、description）是标准
- **系统提示中列出可用 Skill**，模型自行决定何时 read/加载
- 需要更强能力时，通过 **Extension/Plugin 系统**补充（TypeScript/Python 代码钩子）

---

## 2. Claude-Code：三种 Skill 来源 + Fork/Inline 双模式

**核心文件**：`src/skills/bundledSkills.ts`、`loadSkillsDir.ts`、`src/tools/SkillTool/`

### Skill 三种来源

```
来源1：捆绑 Skill（编译进 CLI）
  在 src/skills/bundled/ 中 → initBundledSkills() → bundledSkills[]
  示例：verify, simplify, debug, remember, update-config, loop

来源2：磁盘 Skill（用户/项目目录）
  ├── ~/.claude/skills/*/SKILL.md      (用户级)
  ├── .claude/skills/*/SKILL.md        (项目级，向上遍历)
  └── --add-dir 路径                    (额外路径)

来源3：MCP Skill
  MCP 服务器的 prompt 转换为 Skill（当前存根）
```

### SKILL.md 格式

```markdown
---
description: 简短描述
when_to_use: 何时使用此技能的线索
model: sonnet | opus
allowed-tools: [BashTool, FileEditTool, ...]
context: inline | fork       # 执行模式
user-invocable: true
arguments: [file-path]
argument-hint: <file-path>
effort: high
shell: true                  # 启用 !inline shell 命令
paths: ["src/**/*.ts"]       # 条件激活（匹配文件时）
hooks:
  PreToolUse: [...]
---

技能指令内容...
${argument-name} 变量替换
${CLAUDE_SKILL_DIR} 技能目录
!shell-command 加载时执行
```

### 两种执行模式

```
Inline 模式（context: inline）:
  技能内容注入 system prompt → LLM 看到指令 → 正常使用工具

Fork 模式（context: fork）:
  runForkedAgent() → 隔离子 agent → query() 循环 → 结果合并
  Fork agent 有自己的 canUseTool 权限（只能操作允许的文件）
```

### 动态激活

```
文件操作时：
  discoverSkillDirsForPaths(filePaths, cwd):
    对于每个文件路径，向上遍历到 cwd
      在每个目录检查 .claude/skills/
      按深度排序（最深优先）

activateConditionalSkillsForPaths(filePaths, cwd):
  条件 Skill（带 paths frontmatter）→ ignore 库匹配 → 动态激活
```

### 设计思想

1. **Markdown 作为 DSL**：无需编程，变量替换、shell 执行、模型覆盖、工具限制全在 frontmatter 表达
2. **与 MCP 统一**：MCP Skill 与本地 Skill 合并到同一 `Command[]` 列表，SkillTool 无缝查找
3. **惰性评估**：捆绑 Skill 文件在首次调用时才提取到磁盘；条件 Skill 在文件操作时才检查
4. **Fork 隔离 = 安全**：`context: fork` 创建完全隔离的子 Agent，不污染主会话状态
5. **安全约束**：MCP Skill 是远程且不信任的——永不执行内联 shell 命令

---

## 3. OpenCode：纯内容 Skill + Plugin 钩子系统

**核心文件**：`packages/opencode/src/skill/index.ts`、`skill/discovery.ts`、`tool/skill.ts`

### Skill 定义（极简）

```markdown
---
name: my-skill
description: What this skill does
---

# Skill Content
Markdown instructions, workflows, references...
```

### 发现流程

```
Skill.Service:
  ├── 内置 Skill: "customize-opencode" (硬编码)
  ├── 配置路径: config.directories()/{skill,skills}/**/SKILL.md
  ├── 外部路径: ~/.claude/skills/**/SKILL.md
  ├── Skill URLs: cfg.skills.urls → Discovery.Service.pull(url)
  └── Skill 路径: cfg.skills.paths → 扫描目录

Discovery.Service.pull(url):
  fetch index.json → 下载 skill 文件到缓存
```

### Skill 执行

```
1. System prompt 中列出可用 skill（名称 + 描述）
2. LLM 识别匹配的 skill → 调用 skill 工具
3. SkillTool.execute():
     ├── 读取 SKILL.md 内容
     ├── 扫描 skill 目录的其他文件（最多 10 个）
     └── 以 <skill_content name="..."> XML 包装返回
4. 内容注入到 assistant message → LLM 遵循指令
```

### Skill vs Plugin

```
Skill：
  - 纯 Markdown 文件
  - 没有可执行钩子、生命周期回调、API 访问
  - 通过 skill 工具加载到上下文

Plugin（packages/opencode/src/plugin/）：
  - 代码模块，通过钩子融入 Agent 生命周期
  - 可修改 chat params、headers、tool definitions
  - 可注入 shell env vars
```

### 设计思想

1. **Skill 是内容，Plugin 是代码**——泾渭分明。Skill 无需运行时安全沙箱
2. **Discovery 支持远程 URL**：从 index.json 下载 skill，支持中心化分发
3. **文件名去重 + 覆盖**：内置 skill 先注册，用户 skill 用同名覆盖
4. **content 仅加载一次**：首次加载后缓存全部内容

---

## 4. Hermes Agent：渐进式披露 + 三激活路径

**核心文件**：`agent/skill_utils.py`、`skill_commands.py`、`skill_preprocessing.py`

### Skill 格式

```markdown
---
name: skill-name              # max 64 chars
description: Brief summary    # max 1024 chars
version: 1.0.0
platforms: [macos, linux]
metadata:
  hermes:
    tags: [tag1, tag2]
    config:                   # 配置变量
      - key: wiki.path
        default: ~/wiki
    fallback_for_toolsets: []
    requires_toolsets: []
---
# Skill Title
Full instructions here...
```

### 三激活路径

```
路径1：Slash 命令 /skill-name
  → skill_commands.build_skill_invocation_message()
      ├── 查找 _skill_commands map
      ├── _load_skill_payload() → tools.skills_tool.skill_view()
      ├── 预处理：
      │     ├── 模板变量: ${HERMES_SKILL_DIR}, ${HERMES_SESSION_ID}
      │     ├── 内联 shell: !`command` → stdout 注入
      │     └── 配置注入: 解析 skill 的 config vars
      ├── 构建激活消息: "[IMPORTANT: The user has invoked the "{name}" skill...]"
      └── 注入到对话

路径2：工具 API skills_tool.skill_view(name)
  → LLM 自主决定何时调用

路径3：CLI 预加载 hermes --skill
  → build_preloaded_skills_prompt() → session-wide guidance
```

### 技能配置系统

Skill 可声明 `config` 变量，值存储在 `config.yaml` 的 `skills.config.<key>` 下。激活时自动解析并注入，agent 无需读取 config.yaml。

### 外部 Skill 目录

```
skills.external_dirs 配置 → 扫描外部目录的 SKILL.md
```

### 设计思想

1. **渐进式披露**：元数据（名称+描述）→ 指令（full body）→ 资源（文件列表）三级加载
2. **平台门控**：`platforms: [macos]` 确保 skill 只在兼容 OS 出现
3. **模板变量**让 skill 可以访问运行时信息（会话 ID、skill 目录路径）
4. **命名空间**：外部/插件 skill 用 `namespace:skill-name` 格式

---

## 5. CrewAI：渐进式披露 + Crew 级共享

**核心文件**：`skills/loader.py`、`skills/models.py`、`skills/parser.py`

### Skill 格式

```
my-skill/
  SKILL.md          # YAML frontmatter + Markdown body
  scripts/          # 可选可执行资源
  references/       # 可选参考文档
  assets/           # 可选二进制资产
```

### 渐进式披露三级

| 级别 | 常量 | 加载内容 | 使用场景 |
|------|------|---------|---------|
| 1 | METADATA | 仅 frontmatter（名称+描述） | 初始发现，列出可用 skill |
| 2 | INSTRUCTIONS | 完整 SKILL.md body | 注入系统提示 |
| 3 | RESOURCES | body + 目录资源 | 需要引用额外文件时 |

### Crew 级共享

```python
crew = Crew(
    agents=[researcher, writer],
    skills=[Path("./team-skills/")]  # 所有 Agent 共享
)
```

同一个 skill 被所有 Agent 发现一次，但在各自提示装配时独立注入。

### 设计思想

1. **按需激活**：元数据优先（零成本发现），仅在需要时升级到指令级别
2. **文件系统驱动**：skill 是目录 → 可版本控制、可独立分发
3. **XML 包裹为缓存锚点**：`<skill name="...">` 使 Anthropic 的 prompt cache 跨调用稳定

---

## 6. OpenClaw：XML 注入 + 多层来源

**核心文件**：`src/agents/skills/types.ts`、`skill-contract.ts`、`workspace.ts`

### Skill 发现

```
loadWorkspaceSkillEntries():
  ├── 内置 skills/ 目录（~73 个预定义 skill）
  ├── 用户工作区 skill 目录
  ├── 插件注册 skill
  ├── 包 bundled skill
  └── 已编译 skill（本地 .ts 文件）
```

### System Prompt 注入

```xml
<available_skills>
  <skill>
    <name>github</name>
    <description>Interact with GitHub</description>
    <location>~/.../skills/github/SKILL.md</location>
  </skill>
</available_skills>
```

LLM 通过 `read` 工具按需加载具体 SKILL.md 内容。

### Skill 命令规范

```typescript
SkillCommandDispatchSpec: { kind: "tool", toolName: "github" }
// 有确定性行为的命令可以直接映射到工具
```

### 路径压缩

```
/home/user/project/.claude/skills/github/SKILL.md
→ ~/project/.claude/skills/github/SKILL.md
// 每个 skill path 节省 ~5-6 tokens, N 个 skill 共节省 ~400-600 tokens
```

### 设计思想

1. **XML 编码的系统提示注入**：Skill 不以代码形式"执行"，而是序列化为 XML 注入 system prompt
2. **声明式前置条件**：`requires: { bins: [gh], env: [GITHUB_TOKEN] }`，不满足条件的 skill 自动隐藏
3. **路径压缩节省 token**：虽是细节，但对大规模 skill 集合影响显著
4. **四个加载来源**：内置、用户工作区、插件、编译文件——优先级和覆盖由配置控制

---

## 7. Nanoclaw：四种 Skill 类型 + 容器内被动加载

**核心文件**：技能分布在 `.claude/skills/` 和 `container/skills/` 两个位置

### 四种 Skill 类型

```
Type A: 通道/Provider 安装 Skill
  用途: 从分支复制模块、接入 import、安装依赖
  触发: /add-<name> 命令（如 /add-discord, /add-opencode）

Type B: 实用 Skill（带代码）
  用途: 与代码文件一起运行业务逻辑
  触发: 用户命令

Type C: 运维 Skill（纯指令）
  用途: 设置工作流、调试、故障排除
  触发: 用户命令

Type D: 容器 Skill（容器内加载）
  用途: 告诉容器内 agent 它的能力
  机制: SKILL.md 作为 CLAUDE.md 的指令片段加载
```

### 容器 Skill 加载

```
容器启动:
  host 的 group-init.ts 组合 CLAUDE.md:
    ├── 基础指令 (/app/CLAUDE.md)
    ├── 每个已启用模块的指令片段
    ├── 每组的局部指令 CLAUDE.local.md
    └── 容器 skill SKILL.md 文件（通过 /app/skills/ 只读挂载）

容器 Skill 的 allowed-tools frontmatter:
  allowed-tools: Bash(agent-browser:*)
  → agent 只能运行 agent-browser 子命令，不允许任意 shell
```

### 设计思想

1. **Skill 的四种角色**：安装、实用、运维、容器指令——按加载方式而非功能分类
2. **容器 Skill 是被动的、基于提示词的**——不像传统插件注册 JS 代码，而是通过 SKILL.md 注入指令
3. **`allowed-tools` 前缀模式**：`Bash(agent-browser:*)` 精细控制工具权限

---

## 8. pi：Skill + Extension 双系统

**核心文件**：`packages/agent/src/harness/skills.ts`、`packages/coding-agent/src/core/skills.ts`

### Skill 定义（符合 agentskills.io）

```markdown
---
name: my-skill
description: When to use this...
---

# Instructions
...
```

### Skill 加载

```
loadSkills():
  ├── 全局: {agentDir}/skills/       (--user scope)
  ├── 项目: {cwd}/.pi/skills/        (--project scope)
  ├── 显式路径: --skill path
  └── 递归搜索子目录，遵守 .gitignore
```

### System Prompt 注入

```xml
<available_skills>
  <skill>
    <name>skill-name</name>
    <description>When to use for X...</description>
    <location>/abs/path/to/SKILL.md</location>
  </skill>
</available_skills>
```

### 模型驱动调用

```
harness.skill(name, instructions) → 包装为:
  <skill name="..." location="...">
  References are relative to {dir}.
  {content}
  </skill>
  {额外指令}
→ executeTurn() → 正常 Agent 循环轮次
```

### Extension 系统（互补）

Extension 是 TypeScript 模块，导出 `(api: ExtensionAPI) => void`：

```
Extension API 提供：
├── 事件钩子: on("turn_start"), on("tool_call"), on("agent_end") ...
├── 工具注册: registerTool(definition)
├── 命令注册: registerCommand("/name", handler)
├── Provider 注册: registerProvider()
├── 模型/思考控制: setModel(), setThinkingLevel()
├── 会话操作: sendMessage(), setLabel(), switchSession()
└── UI 控制: setFooter(), setWidget(), setTheme()
```

### Skill vs Extension 对比

```
Skill:
  - 只读 Markdown 文件
  - 无 JS 执行
  - 从任意位置加载
  - 模型可发现（XML 列表）
  - 适合：工具使用说明、编码规则、API 参考

Extension:
  - 主动 TypeScript 模块
  - 订阅事件、修改行为
  - 注册工具、provider、命令
  - 适合：复杂工作流、外部集成、平台功能
```

### 设计思想

1. **Skill 和 Extension 服务不同目的**：简单指导用 Skill，复杂逻辑用 Extension
2. **技能加载器环境无关**：`packages/agent` 中的加载器通过 `ExecutionEnv` 抽象文件系统——可在浏览器、Node.js、Bun 中运行
3. **50+ 示例 Extension**：从蛇形游戏到 `/plan` 命令和工作流管理器
4. **jiti 动态导入**：Extension 在运行时通过 `jiti` 加载 TypeScript，无需预编译

---

## 9. 横向对比总结

| 维度 | Claude-Code | OpenCode | Hermes Agent | CrewAI | OpenClaw | Nanoclaw | pi |
|------|------------|----------|-------------|--------|----------|----------|-----|
| **Skill 格式** | SKILL.md + YAML | SKILL.md + YAML | SKILL.md + YAML | SKILL.md + YAML | SKILL.md + YAML | SKILL.md + YAML | SKILL.md + YAML |
| **Skill 来源** | 捆绑 + 磁盘 + MCP | 内置 + 磁盘 + URL | 磁盘 + 外部目录 + 插件 | 磁盘 + Crew 级 | 内置 + 工作区 + 插件 + 编译 | CLI skills + 容器 skills | 全局 + 项目 + 路径 |
| **执行模式** | inline / fork | inline（SkillTool） | 三路径激活 | inline | inline（read 工具） | 被动提示注入 | inline（harness.skill） |
| **代码扩展** | 无独立扩展系统 | Plugin 钩子 | Plugin 系统 | 无（Flow 替代） | Plugin SDK | Module 系统 | Extension API |
| **条件激活** | paths frontmatter 匹配文件 | 无 | platforms 门控 | 无 | requires 声明式 | 无 | 无 |
| **渐进加载** | 惰性提取到磁盘 | 首次加载后缓存 | 三级披露 | 三级披露 | read 工具按需 | 启动时组装 | 启动时加载 |
| **Skill 数量** | ~20+ 捆绑 + 用户 | 1 内置 + 用户 | 外部目录配置 | 用户定义 | ~73 内置 + 用户 | ~30+ 安装/运维 | 用户定义 + 50+ 示例扩展 |
| **核心创新** | Fork 隔离执行、条件路径激活 | Discovery URL 下载、Plugin 变换工具 | 渐进披露、模板变量、配置注入 | Crew 级共享、XML 缓存锚点 | XML 注入、路径压缩 | 四种角色分类、allowed-tools 前缀 | Skill + Extension 互补 |


## 10. 对 Argo 的启示

1. **SKILL.md + YAML frontmatter 是唯一标准**——七个项目全部遵循，Argo 应直接采用，不要重新发明格式
2. **渐进式披露是正确设计**：先列名称+描述（Level 1），LLM 决定加载后才给完整内容（Level 2）——节省 prompt token
3. **Skill 是内容，Plugin 是代码**——这两个概念不应混淆。Argo 的 talents 应该是纯内容 Skill；代码扩展应该走另一个机制
4. **XML 包裹 `<skill>` 标签让 Anthropic prompt cache 受益**——这对 Anthropic 用户尤其重要
5. **Fork 隔离模式**是高价值特性：某些 Skill 需要在隔离环境中执行（不能污染主会话）——Claude-Code 的 `context: fork` 是 MVP 阶段可以不做但架构上应预留的设计
6. **条件激活**（如 paths 匹配文件）对 VSCode 场景特别有价值——文件操作时自动激活相关 Skill
7. **多来源发现**：内置 Skill（安装时提供）+ 用户 Skill（~/.argo/skills/）+ 项目 Skill（.argo/skills/）+ 外部路径
8. **MVP 阶段应极简**：只需 SKILL.md 的 name + description + Markdown body，其余 frontmatter 字段按需添加
