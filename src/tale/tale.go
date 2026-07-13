// Package tale 封装对话历史及其持久化游标，保证消息列表与 Saved 游标的一致性。
// 未来 Compact（消息裁剪）和 token 计数也将收敛于此，外部通过方法操作，无须感知内部游标。
package tale

import "argo/src/knot"

// Tale 对话历史封装。
// 内部持有完整的 Messages 链和持久化游标 saved，
// Compact 裁剪时同时调整二者，外部无感。
type Tale struct {
	messages     []knot.Message
	systemPrompt string
	saved        int // 已持久化的消息条数，外部不可见
}

// New 用已有的消息列表构造 Tale。
// saved 初始等于 len(messages)，表示全部消息已持久化（从 vault 加载的场景）。
func New(messages []knot.Message, systemPrompt string) *Tale {
	return &Tale{
		messages:     messages,
		systemPrompt: systemPrompt,
		saved:        len(messages),
	}
}

// Saved 返回当前持久化游标，供 helm.State.Save 序列化到 state.json。
func (t *Tale) Saved() int { return t.saved }

// SetSaved 从 state.json 反序列化后恢复游标，供 helm.State.Load 调用。
func (t *Tale) SetSaved(n int) { t.saved = n }

// AppendUser 追加一条 role=user 的消息到对话历史末尾。
func (t *Tale) AppendUser(content string) {
	t.messages = append(t.messages, knot.Message{Role: knot.MessageRoleUser, Content: content})
}

// AppendAssistant 追加一条 role=assistant 的消息到对话历史末尾。
// toolCalls 为 nil 时表示纯文本回复。
func (t *Tale) AppendAssistant(content string, toolCalls []knot.ToolCall) {
	t.messages = append(t.messages, knot.Message{
		Role:      knot.MessageRoleAssistant,
		Content:   content,
		ToolCalls: toolCalls,
	})
}

// BuildRequest 构造 LLM 请求消息列表：system prompt + 完整对话历史。
func (t *Tale) BuildRequest(systemPrompt string) []knot.Message {
	sysMsg := knot.Message{Role: knot.MessageRoleSystem, Content: systemPrompt}
	return append([]knot.Message{sysMsg}, t.messages...)
}

// Unsaved 返回尚未持久化的消息差量，供 checkpoint 增量写入。
func (t *Tale) Unsaved() []knot.Message {
	return t.messages[t.saved:]
}

// MarkSaved 将当前全部消息标记为已持久化。
func (t *Tale) MarkSaved() {
	t.saved = len(t.messages)
}

// AppendTool 追加一条 role=tool 的消息到对话历史末尾，关联指定的 tool_call。
func (t *Tale) AppendTool(callID string, content string) {
	t.messages = append(t.messages, knot.Message{
		Role:       knot.MessageRoleTool,
		ToolCallID: callID,
		Content:    content,
	})
}
