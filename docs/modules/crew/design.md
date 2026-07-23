# crew

## 概述

crew 是技能（Skill）管理模块。一门技能就是一个目录下的 SKILL.md 文件。crew 只做三件事——发现、读取、按需供给，不做生命周期管理、安全扫描或条件激活。

crew 在架构中处于内容供给层：helm 从 crew 获取 L1 技能列表注入 system prompt，deck 通过 crew 提供 `skill` 工具供 LLM 加载 L2 全文，server 通过 crew 实现 `/skill-name` slash command。

## 核心概念

| 概念 | 说明 |
|------|------|
| SkillInfo | 技能元数据：Name、Description、Path（SKILL.md 路径） |
| Skills 接口 | `List() []SkillInfo` + `Instructions(name string) (string, error)` |
| Crew | Skills 接口实现，不缓存——每次访问从磁盘重新扫描 |
| 渐进式披露 | L1 元数据永驻 system prompt，L2 全文通过 skill 工具或 /skill-name 按需加载，L3 资源文件通过现有 read 工具访问 |
| 目录结构 | `SKILL.md` 所在目录 = 一个技能，目录名为 name fallback |
| 优先级 | 项目级 `.argo/skills/` 覆盖用户级 `~/.argo/skills/` 同名技能 |
| 双激活路径 | skill 工具（LLM 自主） + /skill-name（用户显式），互补 |

## 行为合约

### 技能发现

crew 在 `Init()` 时记录用户和项目技能目录路径。每次 `List()` 和 `Instructions()` 调用都从磁盘重新扫描，不维护内存缓存。

- 递归扫描 `~/.argo/skills/`（用户级）和 `.argo/skills/`（项目级）
- 跳过以 `.` 开头的隐藏目录和 `node_modules`
- 目录不存在时不报错，返回空列表
- 刚写入磁盘的技能立即对 `List()` 和 `Instructions()` 可见，无需重启或手动 reload

### 技能元数据（L1）

`List()` 返回所有已发现技能的 name 和 description，按名称字母排序。

- 每轮对话通过 `buildSystemPrompt` 自动注入 `<available_skills>` XML 块到 system prompt
- 没有技能时 `<available_skills>` 块不出现

### 技能全文加载（L2）

`Instructions(name)` 返回 SKILL.md 去除 YAML frontmatter 后的完整 Markdown 正文。

- 技能不存在时返回 error
- 可通过 skill 工具（LLM 调用）或 /skill-name slash command（用户显式）触发
- /skill-name 检测在 server 层 `resolveSlashCommand()` 中，所有客户端通用；技能不存在时消息原样透传给 LLM

### SKILL.md 格式解析

解析 YAML frontmatter（`---` 分隔），提取 `name` 和 `description` 字段。

- `name` 缺失时用父目录名作为 fallback
- `description` 允许为空
- frontmatter 不存在时该文件被跳过，不视为有效技能
