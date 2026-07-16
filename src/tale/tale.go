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

// BuildRequest 构造 LLM 请求消息列表。
// 按时间顺序应用 compact 标记（C1→C2→...），然后前置 system prompt。
// systemPrompt 为空时返回压缩视图不含 system prompt（供 Compactor 使用）。
func (t *Tale) BuildRequest(systemPrompt string) []knot.Message {
	compacted := t.applyCompactView()
	if systemPrompt == "" {
		return compacted
	}
	sysMsg := knot.Message{Role: knot.MessageRoleSystem, Content: systemPrompt}
	return append([]knot.Message{sysMsg}, compacted...)
}

// Unsaved 返回尚未持久化的消息差量，供 checkpoint 增量写入。
func (t *Tale) Unsaved() []knot.Message {
	return t.messages[t.saved:]
}

// AppendMessages 直接追加消息切片，不改变持久化游标。
// 用于从 state.json 恢复尚未落盘的 PendingMessages。
func (t *Tale) AppendMessages(msgs []knot.Message) {
	t.messages = append(t.messages, msgs...)
}

// MarkSaved 将当前全部消息标记为已持久化。
func (t *Tale) MarkSaved() {
	t.saved = len(t.messages)
}

// MarkCompact 记录 compact 标记。不删除消息，BuildRequest 时自动应用压缩视图。
func (t *Tale) MarkCompact(from, to int, summary string) {
	t.messages = append(t.messages, knot.Message{
		Role:        knot.MessageRoleCompact,
		Content:     summary,
		CompactFrom: from,
		CompactTo:   to,
	})
}

// applyCompactView 按时间顺序应用 compact 标记，返回压缩后的消息列表。
// 每个 compact 的 from/to 对应当前中间结果的索引。
func (t *Tale) applyCompactView() []knot.Message {
	// 收集非 compact 消息作为起点
	var result []knot.Message
	for _, m := range t.messages {
		if m.Role != knot.MessageRoleCompact {
			result = append(result, m)
		}
	}
	// 按时间顺序（t.messages 中的顺序）依次应用 compact
	for _, m := range t.messages {
		if m.Role != knot.MessageRoleCompact {
			continue
		}
		if m.CompactFrom < 0 || m.CompactTo > len(result) || m.CompactFrom >= m.CompactTo {
			continue
		}
		summaryMsg := knot.Message{Role: knot.MessageRoleSystem, Content: m.Content}
		result = append(
			append(result[:m.CompactFrom], summaryMsg),
			result[m.CompactTo:]...,
		)
	}
	return result
}

// AppendTool 追加一条 role=tool 的消息到对话历史末尾，关联指定的 tool_call。
func (t *Tale) AppendTool(callID string, content string) {
	t.messages = append(t.messages, knot.Message{
		Role:       knot.MessageRoleTool,
		ToolCallID: callID,
		Content:    content,
	})
}
