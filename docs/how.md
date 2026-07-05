# Argo 技术架构

## 概述

Argo 是一个用 Go 实现的极简 AI Agent 框架。七个模块 (deck/helm/reef/vault/sail/crew/brig) 通过包级单例调用协作，同一份核心代码编译为 CLI 和 Gateway 两种运行模式。共享类型集中在 knot 包。

## 技术栈

- **语言**：Go
- **七个模块 + 一个类型包**：deck (工具)、helm (循环)、reef (压缩)、vault (存储)、sail (模型)、crew (技能)、brig (权限门控)、knot (共享类型)
- **双入口**：`argo run` (CLI) / `argo serve` (Gateway)，共享同一核心代码
- **状态持有**：调用方持有会话状态，helm 是纯函数

## 模块拆分

| 模块 | 职责 | 技术形态 |
|------|------|---------|
| **deck** | 工具注册、发现、调用。统一内置工具和CLI外部工具两种来源 | Go 接口 + 注册表 |
| **helm** | Agent 主循环。用户输入→LLM→工具→结果→循环 | Go 自由函数 + channel 流式输出 |
| **reef** | 会话上下文压缩。token超限时裁剪旧消息、生成LLM摘要 | Go 代码固化流程骨架，SKILL.md 定义摘要模板 |
| **vault** | 对话记录持久化(transcript) + 跨会话长期记忆 | Go 文件/存储操作 |
| **sail** | 多模型API管理。屏蔽Anthropic/OpenAI/DeepSeek等差异 | Go 接口 + adapter模式 |
| **crew** | SKILL.md技能加载与调度。渐进式披露注入system prompt | Go 文件扫描 + SKILL.md解析 |
| **brig** | 工具调用权限门控。Allow/Ask/Deny 三态裁决，规则即记忆 | Go 规则引擎 + 静态规则库 |
| **knot** | 跨模块共享类型：Message/Event/ToolUse/Config/ConfirmFunc | Go 接口 + 结构体 |

## 跨模块接口契约

各模块通过包级公开函数暴露能力，由 helm 直接调用：

- **deck**：`List()` / `Lookup(name)` / `Tool.Execute()` — 工具注册、发现、执行
- **reef**：`Compact(ctx, messages, tokenLimit)` — 上下文裁剪，失败降级
- **vault**：`Recall()` / `Store()` / `Append()` — 长期记忆存取（待建）
- **sail**：`Chat(ctx, modelName, req)` / `Window(modelName)` / `DefaultModel()` — LLM 调用 + 模型配置
- **crew**：`List()` / `Instructions(name)` — 技能加载（待建）
- **brig**：`Evaluate(tu)` / `Append(tool, pattern, action, source)` / `Snapshot()` / `Restore()` — 权限门控

## 数据流

### 启动流程

```
argo run / argo serve
  │
  ├── deck  注册 → 内置工具显式注册（builtin 优先）
  │               → 读 ~/.argo/config.json → 校验并注册外部 CLI 工具
  ├── vault  初始化 → 创建/打开 session 目录
  ├── reef   就绪 → 等待 helm 调用
  ├── crew 扫描 → 加载 ~/.argo/skills/ 下的 SKILL.md
  ├── sail   就绪 → 等待 helm 调用
  ├── brig   就绪 → NewEngine() 加载静态规则
  │
  └── helm   就绪 → 等待用户输入
```

### 运行时流程（一回合）

```
用户输入
  │
  ▼
helm.RunLoop()
  │
  │   会话级缓存（循环前只执行一次）：
  │   ├── deck.List() + buildSystemPrompt()
  │   └── 跨轮次复用
  │
  ├── 1. reef.Compact()        // 裁剪消息历史（失败降级用原始消息）
  ├── 2. sail.Chat()           // LLM 调用 → <-chan Event（流式转发）
  │      ├── textStart / textDelta / textDone  → 累计 + 转发
  │      ├── toolUseStart                      → 收集待执行
  │      └── done / error                      → 最终回复或错误
  │
  ├── 3. 无 tool_use → 退出
  │
  ├── 4. deck.Lookup() → brig.Evaluate() → Tool.Execute()
  │      // brig 判定 Allow/Ask/Deny，Ask 时 confirm 征求用户
  │
  ├── 5. 结果注入 state.Messages          // assistant + tool 消息
  │
  └── 6. 继续循环或退出
```

## 配置文件与目录结构

```
~/.argo/
├── config.json                // 全局配置（工具 + 模型 provider）
├── profile.md                 // vault: 用户画像/偏好（LLM 维护，5K 预算）
├── AGENTS.md                  // vault: 硬性约束（用户管理，LLM 只读）
├── sessions/
│   └── {session_id}/
│       └── transcript.jsonl   // vault: 对话记录
├── skills/                    // crew: SKILL.md 文件
│   ├── reef-compact/
│   │   └── SKILL.md           // reef: 压缩摘要模板
│   └── learn/
│       └── SKILL.md           // 业务 Skill（由 Vug/叩问等上层定义）

代码仓库内：
argo/
├── cmd/
│   ├── run/                   // CLI 入口
│   └── serve/                 // Gateway 入口
├── src/
│   ├── knot/                  // 跨模块共享类型
│   ├── deck/                  // 工具管理模块
│   ├── helm/                  // Agent 循环模块
│   ├── reef/                  // 上下文压缩模块
│   ├── vault/                 // 存储模块
│   ├── sail/                  // 模型管理模块
│   ├── brig/                  // 权限门控模块
│   └── crew/                  // 技能管理模块
└── docs/                      // 设计文档
```
