package knot

import "argo/src/deck"

// Event 类型常量
const (
	EventStart         = "start"
	EventTextStart     = "textStart"
	EventTextDelta     = "textDelta"
	EventTextDone      = "textDone"
	EventThinkingStart = "thinkingStart"
	EventThinkingDelta = "thinkingDelta"
	EventThinkingDone  = "thinkingDone"
	EventToolUseStart  = "toolUseStart"
	EventToolUseDelta  = "toolUseDelta"
	EventToolUseDone   = "toolUseDone"
	EventDone          = "done"
	EventError         = "error"

	MessageRoleUser      = "user"
	MessageRoleAssistant = "assistant"
	MessageRoleTool      = "tool"
)

// Message 单条对话消息
type Message struct {
	// Role 取值为 MessageRoleUser | MessageRoleAssistant | MessageRoleTool
	Role    string
	Content string
}

// ChatRequest LLM 调用请求
type ChatRequest struct {
	SystemPrompt string
	Messages     []Message
	Tools        []deck.Tool
}

// TokenLimit 模型 context window 上限
type TokenLimit struct {
	MaxTokens int
}

// TokenUsage 单次调用的 token 消耗
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
}

// Event 流式事件，通过 channel 推送给调用方
type Event struct {
	// Type 标识事件类型，取值为各 Event* 常量
	Type    string
	Delta   string // textDelta 或 thinkingDelta，由 Type 区分
	ToolUse *ToolUse
	Done    *DonePayload
	Err     error
}

// ToolUse 工具调用，Result=nil 表示待执行
type ToolUse struct {
	ID         string
	Name       string
	Parameters map[string]any
	Result     *deck.ToolResult
}

// DonePayload 完成时的最终结果
type DonePayload struct {
	Text       string
	TokenUsage TokenUsage
}
