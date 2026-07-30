// Package pact 定义 Argo 各模块间共享的公共数据类型和协议。
// 所有新包只 import pact，不互相依赖。
package pact

// ── 核心类型 ──

// Omen 统一表示工具调用的完整生命周期，吸收旧 knot.ToolCall + ToolUse + ToolResult。
type Omen struct {
	// ID 工具调用唯一标识
	ID string
	// Name 工具名
	Name string
	// Arguments JSON 序列化的参数（LLM 原始输出）
	Arguments string
	// Result 工具输出文本
	Result string
	// Status 执行状态，取值见 StatusPending / StatusSuccess / StatusError 常量
	Status int
	// Verdict 审核结果，取值见 VerdictAllow / VerdictDeny 常量
	Verdict int
	// VerdictReason 审核/驳回理由
	VerdictReason string
}

// Omen 状态常量
const (
	// StatusPending 待执行
	StatusPending = iota + 1
	// StatusSuccess 成功
	StatusSuccess
	// StatusError 失败
	StatusError
)

// Omen 审核结果常量
const (
	// VerdictAllow 放行
	VerdictAllow = iota + 1
	// VerdictDeny 拒绝
	VerdictDeny
	// VerdictAsk 等待用户审批
	VerdictAsk
)

// Message 角色常量
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
	RoleSystem    = "system"
	RoleCompact   = "compact"
)

// Message 单条对话消息。
type Message struct {
	// Role 消息角色，取值见 RoleUser / RoleAssistant / RoleTool / RoleSystem / RoleCompact 常量
	Role string
	// Content 消息正文
	Content string
	// OmenID role=tool 时关联对应的 Omen
	OmenID string
	// Omens role=assistant 时记录本轮发起的工具调用
	Omens []Omen
	// Model role=assistant 时记录使用的模型名
	Model string
	// Timestamp 消息时间戳（ISO 8601）
	Timestamp string
}

// ── 流式事件 ──

// 流式事件类型常量，Event.Type 取值。
const (
	// EventTypeStart 流开始
	EventTypeStart = "start"
	// EventTypeTextStart 文本段开始
	EventTypeTextStart = "textStart"
	// EventTypeTextDelta 文本增量
	EventTypeTextDelta = "textDelta"
	// EventTypeTextDone 文本段结束
	EventTypeTextDone = "textDone"
	// EventTypeThinkingStart 思考段开始
	EventTypeThinkingStart = "thinkingStart"
	// EventTypeThinkingDelta 思考增量
	EventTypeThinkingDelta = "thinkingDelta"
	// EventTypeThinkingDone 思考段结束
	EventTypeThinkingDone = "thinkingDone"
	// EventTypeToolUseStart 工具调用开始
	EventTypeToolUseStart = "toolUseStart"
	// EventTypeToolUseDelta 工具调用增量
	EventTypeToolUseDelta = "toolUseDelta"
	// EventTypeToolUseDone 工具调用结束
	EventTypeToolUseDone = "toolUseDone"
	// EventTypeDone 整个响应结束
	EventTypeDone = "done"
	// EventTypeError 错误
	EventTypeError = "error"
	// EventTypeAsk 需要用户确认
	EventTypeAsk = "ask"
)

// Event 流式事件，sight.Take() 通过 channel 推送。
type Event struct {
	// Type 事件类型，取值见 EventTypeStart / EventTypeTextStart / EventTypeTextDelta / EventTypeTextDone / EventTypeThinkingStart / EventTypeThinkingDelta / EventTypeThinkingDone / EventTypeToolUseStart / EventTypeToolUseDelta / EventTypeToolUseDone / EventTypeDone / EventTypeError / EventTypeAsk 常量
	Type string
	// Delta textDelta / thinkingDelta 的增量文本
	Delta string
	// Omens toolUse* / ask 时携带的工具调用
	Omens []Omen
	// Usage EventTypeDone 时携带的 token 消耗统计
	Usage TokenUsage
	// Err EventTypeError 时携带的错误
	Err error
}

// ── Token 用量 ──

// TokenUsage 单次 LLM 调用的 token 消耗。
type TokenUsage struct {
	// InputTokens 输入 token 数
	InputTokens int
	// OutputTokens 输出 token 数
	OutputTokens int
}
