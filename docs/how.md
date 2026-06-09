# Argo 技术架构

## 概述

Argo 是一个用 Go 实现的极简 AI Agent 框架。六个模块 (deck/helm/reef/vault/sail/crew) 通过接口注入协作，同一份核心代码编译为 CLI 和 Gateway 两种运行模式。

## 技术栈

- **语言**：Go
- **六个模块**：deck (工具)、helm (循环)、reef (压缩)、vault (存储)、sail (模型)、crew (技能)
- **双入口**：`argo run` (CLI) / `argo serve` (Gateway)，共享同一核心代码
- **状态持有**：调用方持有会话状态，helm 是纯函数

## 模块拆分

| 模块 | 职责 | 技术形态 | 设计状态 |
|------|------|---------|---------|
| **deck** | 工具注册、发现、调用。统一内部工具/外部CLI/MCP三种来源 | Go 自由函数 + 接口 | 详细设计完成 |
| **helm** | Agent 主循环。用户输入→LLM→工具→结果→循环 | Go 自由函数 + channel 流式输出 | 详细设计完成 |
| **reef** | 会话上下文压缩。token超限时裁剪旧消息、生成LLM摘要 | Go 代码固化流程骨架，SKILL.md 定义摘要模板 | 详细设计完成 |
| **vault** | 对话记录持久化(transcript) + 跨会话长期记忆 | Go 文件/存储操作 | 详细设计完成 |
| **sail** | 多模型API管理。屏蔽Anthropic/OpenAI/DeepSeek等差异 | Go 接口 + adapter模式 | 待设计 |
| **crew** | SKILL.md技能加载与调度。渐进式披露注入system prompt | Go 文件扫描 + SKILL.md解析 | 详细设计完成 |

## 跨模块接口契约

```go
// 以下接口由 helm（消费者）定义，各模块提供实现

// Tools — deck 模块实现
type Tools interface {
    List() []Tool
    Lookup(ctx context.Context, name string) (Tool, bool)
    ExecuteBatch(ctx context.Context, calls []ToolCall) []ToolResult
}

// Compactor — reef 模块实现
type Compactor interface {
    Compact(ctx context.Context, messages []Message, window TokenLimit) (CompactResult, error)
}

// LLM — sail 模块实现
type LLM interface {
    Chat(ctx context.Context, req ChatRequest) (<-chan Event, error)
    Window() TokenLimit
}

// Memory — vault 模块实现
type Memory interface {
    Append(ctx context.Context, messages []Message) error           // 写入对话记录
    Recall(ctx context.Context, query string) (*MemoryEntry, error) // 检索长期记忆
    Store(ctx context.Context, content string) error                // 写入长期记忆
}

// Skills — crew 模块实现
type Skills interface {
    List(ctx context.Context) []SkillInfo
    Instructions(ctx context.Context, name string) string
}
```

## 数据流

### 启动流程

```
argo run / argo serve
  │
  ├── deck  注册 → import tools/builtin 触发 init() 自注册
  │               → 读 ~/.argo/tools.json → 注册外部 CLI 工具
  │               → spawn MCP 子进程 → 注册 MCP 工具
  ├── vault  初始化 → 创建/打开 session 目录
  ├── reef   就绪 → 等待 helm 调用
  ├── crew 扫描 → 加载 ~/.argo/skills/ 下的 SKILL.md
  ├── sail   就绪 → 等待 helm 调用
  │
  └── helm   就绪 → 等待用户输入
```

### 运行时流程（一回合）

```
用户输入
  │
  ▼
helm.runLoop()
  │
  ├── 1. reef.Compact(messages, window)    // 裁剪消息历史（未超阈值则原样返回）
  ├── 2. deck.List()                       // 获取可用工具列表
  ├── 3. buildSystemPrompt(                // 拼装 system prompt
  │        crew.ActiveInstructions()     //   ← crew: 当前 Skill 指令
  │        vault.Recall()                   //   ← vault: 用户偏好/长期记忆
  │        tools                            //   ← deck: 工具描述
  │    )
  ├── 4. sail.Chat(system, messages, tools) // LLM 调用 → <-chan Event（流式转发）
  │      ├── Event.text_delta  → 调用方渲染逐 token
  │      ├── Event.tool_use    → LLM 要调工具
  │      └── Event.done        → 最终回复 + token 用量
  │
  ├── 5. deck.Lookup(name) → Tool.Execute(params)             // 执行工具（如 LLM 有 tool_use）
  │      └── Event.tool_use (Result 已填充) → 工具执行完成
  │
  ├── 6. vault.Append(newMessages)          // 写入对话记录
  │
  └── 7. 回到步骤 1（如 LLM 还有 tool_use）或结束
```

## 配置文件与目录结构

```
~/.argo/
├── tools.json                 // deck: 外部 CLI + MCP 工具配置
├── sessions/
│   └── {session_id}/
│       └── transcript.jsonl   // vault: 对话记录
├── skills/                    // crew: SKILL.md 文件
│   ├── reef-compact/
│   │   └── SKILL.md           // reef: 压缩摘要模板
│   └── learn/
│       └── SKILL.md           // 业务 Skill（由 Vug/叩问等上层定义）
└── memories/                  // vault: 长期记忆存储（待设计）

代码仓库内：
argo/
├── cmd/
│   ├── run/                   // CLI 入口
│   └── serve/                 // Gateway 入口
├── deck/                      // 工具管理模块
├── helm/                      // Agent 循环模块
├── reef/                      // 上下文压缩模块
├── vault/                     // 存储模块（待设计）
├── sail/                      // 模型管理模块（待设计）
├── crew/                   // 技能管理模块（待设计）
└── docs/                      // 设计文档
```
