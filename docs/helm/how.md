# helm 实现设计

## 概述

helm 是一个无状态的自由函数 `runLoop`，不绑定方法，所有外部依赖通过 `HelmDeps` 接口注入。通过 channel 流式推送事件，调用方（CLI/Gateway）持有会话状态并负责持久化。

## 内部结构

```
helm/
├── loop.go       // runLoop() 核心循环
├── prompt.go     // buildSystemPrompt() system prompt 组装
├── stream.go     // event channel + Event 类型定义
├── deps.go       // HelmDeps 接口
└── types.go      // TurnState, ToolCall
```

## 核心类型

```go
// HelmDeps — helm 消费的外部接口
type HelmDeps struct {
    Tools     Tools
    Compactor Compactor
    Memory    Memory
    LLM       LLM
    Skills    Skills
}

// 以下接口由 helm（消费者）定义，各模块包提供实现

type Tools interface {
    List(ctx context.Context) []ToolDef
    ExecuteBatch(ctx context.Context, calls []ToolCall) []ToolResult
    Lookup(ctx context.Context, name string) (ToolDef, bool)
}

type Compactor interface {
    Compact(ctx context.Context, messages []Message, window TokenLimit) ([]Message, error)
}

type Memory interface {
    Recall(ctx context.Context, query string) (*MemoryEntry, error)
    Store(ctx context.Context, content string) error
}

type LLM interface {
    Chat(ctx context.Context, req ChatRequest) (<-chan Event, error)
    Window() TokenLimit
}

type Skills interface {
    List(ctx context.Context) []SkillInfo
    Instructions(ctx context.Context, name string) string
}

// TurnState — 调用方持有，每轮传入传出
type TurnState struct {
    Messages    []Message
    TurnCount   int
    SystemPrompt string
}

// Event — 流式事件
type Event struct {
    Type      string       // "text_delta" | "tool_use" | "done" | "error"
    TextDelta string       // Type="text_delta" 时
    ToolUse   *ToolUse     // Type="tool_use" 时，Result 非 nil 表示已执行完成
    Done      *DonePayload // Type="done" 时
    Error     error        // Type="error" 时
}

type DonePayload struct {
    Text       string
    TokenUsage TokenUsage
}

// ToolUse — LLM 发起调用时 Result=nil，deck 执行完成后更新同一条目填入 Result
type ToolUse struct {
    ID         string
    Name       string
    Parameters map[string]any
    Result     *ToolResult
}

type TokenLimit struct {
    MaxTokens int
}
```

## 函数签名

```go
// runLoop — 核心循环。返回 event channel，调用方 for range 消费
// state 由调用方初始化并传入（包含首次 user message），循环结束后 state 被更新为最终状态
// 每轮结束后调用 deps.Vault 写入新增消息（格式见 ../vault/transcript.md）
func runLoop(ctx context.Context, deps HelmDeps, state *TurnState) (<-chan Event, error)

// buildSystemPrompt — helm 主动向各模块索取信息，拼装 system prompt
// 每轮 LLM 调用前执行
func buildSystemPrompt(ctx context.Context, deps HelmDeps, deckTools []ToolDef) string
```

## 核心循环伪代码

```go
func runLoop(ctx context.Context, deps HelmDeps, state *TurnState) (<-chan Event, error) {
    out := make(chan Event, 16)

    go func() {
        defer close(out)

        // 会话级缓存：system prompt + tools 只构建一次，跨轮次复用
        tools := deps.Tools.List(ctx)
        state.SystemPrompt = buildSystemPrompt(ctx, deps, tools)

        for {
            state.TurnCount++

            // 1. 先裁剪消息历史，确定剩余 token 预算
            msgs, err := deps.Reef.Compact(ctx, state.Messages, deps.Sail.Window())
            if err != nil { /* 裁剪失败不阻塞，用原始消息 */ }
            state.Messages = msgs

            // 2. LLM 调用
            events, err := deps.Sail.Chat(ctx, ModelRequest{
                System:  state.SystemPrompt,
                Messages: state.Messages,
                Tools:    tools,
            })
            if err != nil {
                out <- Event{Type: "error", Error: err}
                return
            }

            // 5. 流式消费 LLM 回复，收集 tool_use
            var toolUses []ToolUse
            var finalText string
            for evt := range events {
                out <- evt                     // 转发 text_delta / done
                if evt.Type == "tool_use" {
                    toolUses = append(toolUses, *evt.ToolUse)
                }
                if evt.Type == "done" {
                    finalText = evt.Done.Text
                }
            }

            // 6. 无工具调用 → 结束
            if len(toolUses) == 0 {
                out <- Event{Type: "done", Done: &DonePayload{Text: finalText}}
                deps.Vault.Store(ctx, finalText)
                return
            }

            // 7. 执行工具，原地填充 Result
            for i := range toolUses {
                def, _ := deps.Tools.Lookup(ctx, toolUses[i].Name)
                result := deps.Tools.Execute(ctx, def, toolUses[i].Parameters)
                toolUses[i].Result = &result
                out <- Event{Type: "tool_use", ToolUse: &toolUses[i]} // 同一 ID，Result 已填
            }

            // 8. 注入结果到消息历史
            state.Messages = append(state.Messages, assistantMsg(toolUses))
            for _, tu := range toolUses {
                state.Messages = append(state.Messages, toolResultMsg(tu))
            }
        }
    }()

    return out, nil
}
```

## 数据流

```
调用方 (CLI/Gateway)
  │
  │ state = { Messages: [userMsg], TurnCount: 0 }
  ▼
runLoop(ctx, deps, &state)
  │
  ├─ 会话级缓存（循环前执行一次，跨轮次复用）:
  │     ├─ deps.Tools.List()        → tools
  │     └─ buildSystemPrompt(): talents + vault + tools → system prompt
  ├─ deps.Reef.Compact()     → 裁剪消息历史
  ├─ deps.Sail.Chat()    → <-chan Event（流式转发给调用方）
  │     ├─ [每轮] Event.text_delta → 调用方展示逐 token
  │     ├─ [有工具] Event.tool_use → 调用方展示工具调用及结果
  │     └─ Event.done         → 最终文本 + token 用量
  ├─ deps.Tools.ExecuteBatch() → 工具执行
  ├─ deps.Vault.Store()      → fire-and-forget 写入记忆
  │
  ▼
state.Messages 更新（追加 assistant + tool_results）
循环 → 或无工具调用则退出
```
