# crew 实现设计

## 概述

crew 实现 helm 定义的 `Skills` 接口。一门技能就是一个 SKILL.md 文件。crew 只做三件事——发现、读取、按需供给。

## 内部结构

```
crew/
├── scanner.go      // 扫描 ~/.argo/skills/ + 项目 .argo/skills/
├── loader.go       // 解析 SKILL.md frontmatter + 读取正文
├── registry.go     // Skills 接口实现
├── types.go        // SkillInfo, SkillEntry
└── crew_test.go
```

## 核心类型

```go
// SkillInfo — L1 元数据（List 返回）
type SkillInfo struct {
    Name        string
    Description string
    Path        string // SKILL.md 文件路径
}

// SkillEntry — L2 指令（Instructions 返回）
type SkillEntry struct {
    SkillInfo
    Content string // SKILL.md 正文（不含 frontmatter）
}

// Crew — Skills 接口实现
type Crew struct { ... }
```

## 函数签名

```go
type Crew struct { ... }
func NewCrew(paths []string) (*Crew, error)

// List — 返回所有可用技能的 L1 元数据
func (c *Crew) List(ctx context.Context) []SkillInfo

// Instructions — 加载指定技能的 L2 指令
func (c *Crew) Instructions(ctx context.Context, name string) (string, error)

// loadSkill — 解析 SKILL.md 的 YAML frontmatter + Markdown 正文
func loadSkill(path string) (*SkillEntry, error)

// scanDir — 递归扫描目录下的 SKILL.md 文件
func scanDir(root string) ([]string, error)
```

## SKILL.md 格式

```markdown
---
name: learn
description: 引导用户完成技术概念的结构化学习流程
---

# 正文

技能的具体指令和约束...
```

## 数据流

```
启动时:
  Crew.scanDir("~/.argo/skills/")
    ├── 找到 learn/SKILL.md → 解析 frontmatter → 存入 entries[name]
    ├── 找到 select/SKILL.md → 同上
    └── 递归扫描子目录

helm 构建 system prompt:
  Skills.List()
    → [{name:"learn", description:"..."}, {name:"select", description:"..."}]
    → helm 嵌入 system prompt 的 <available_skills> 块

LLM 决定激活某个技能:
  Skills.Instructions("learn")
    → 读取 learn/SKILL.md 正文
    → 返回完整 Markdown 内容
    → helm 注入 system prompt

LLM 需要技能的资源文件:
  Read 工具 → learn/references/guide.md（标准 deck 工具，不在 crew 范围内）
```
