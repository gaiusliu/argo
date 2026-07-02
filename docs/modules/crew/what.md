# crew 行为规约

## 概述

crew 实现 helm 定义的 Skills 接口。一门技能就是一个 SKILL.md 文件。crew 只做三件事——发现、读取、按需供给。技能内容按三级渐进式披露加载。

| 级 | 加载时机 | 内容 | 方法 |
|------|------|------|------|
| L1 元数据 | 启动时扫描 | 仅 name + description | Skills.List() |
| L2 指令 | LLM 决定激活时 | 完整 SKILL.md 正文 | Skills.Instructions(name) |
| L3 资源 | LLM 需要更多细节时 | skill 目录下 references/ 等文件 | Read 工具按需 |

## 需求

### 需求：技能发现

crew SHALL 在启动时递归扫描 `~/.argo/skills/` 目录，解析每个 SKILL.md 的 YAML frontmatter，提取 name 和 description。

#### 场景：启动时扫描技能目录

- 给定：`~/.argo/skills/learn/SKILL.md` 存在，frontmatter 声明 name=learn, description="引导用户完成技术概念的结构化学习流程"
- 当：Argo 启动
- 则：Skills.List() 返回 [{name:"learn", description:"引导用户完成技术概念的结构化学习流程"}]

#### 场景：重复技能名

- 给定：`~/.argo/skills/custom/SKILL.md` 和项目 `.argo/skills/custom/SKILL.md` 都存在，且 frontmatter 中 name 相同
- 当：Argo 启动
- 则：项目级优先于用户级。两个技能都加载，项目级的覆盖用户级的

### 需求：按需加载技能指令

crew SHALL 在 LLM 决定激活某个技能后，返回该技能的完整 SKILL.md 正文（不含 YAML frontmatter）。

#### 场景：LLM 激活技能

- 给定：LLM 判断当前任务需要 learn Skill
- 当：helm 调用 Skills.Instructions("learn")
- 则：返回 learn SKILL.md 的完整正文，注入 system prompt

#### 场景：请求不存在的技能

- 给定：LLM 请求 Skills.Instructions("nonexistent")
- 当：名为 "nonexistent" 的技能在注册表中不存在
- 则：返回错误，helm 告知 LLM 该技能不存在

### 需求：技能列表注入 system prompt

crew SHALL 提供可用技能的名称和描述列表，供 helm 嵌入 system prompt 的 `<available_skills>` 块。

#### 场景：LLM 在对话中看到可用技能

- 给定：注册表中有 learn 和 select 两个技能
- 当：helm 调用 Skills.List()
- 则：返回两个 SkillInfo，helm 将其格式化为 XML 或 Markdown 列表注入 system prompt

## 边界情况

- SKILL.md 不存在 frontmatter：视为无效技能，跳过加载，记日志
- SKILL.md frontmatter 缺少 name：视为无效技能，跳过加载
- SKILL.md frontmatter 缺少 description：允许加载，description 为空字符串
- 技能目录下无 SKILL.md 但包含 .md 文件：不视为技能，跳过
