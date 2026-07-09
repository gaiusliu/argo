package helm

import (
	"argo/src/knot"
	"argo/src/vault"
)

// Checkpoint RunLoop 挂起时的完整状态快照。
// 用于暂停/恢复流程：RunLoop 遇到 EventAsk 后写检查点并退出，
// 外部回复后加载检查点继续执行。
type Checkpoint struct {
	// Messages 完整对话历史，已包含本轮的 assistant message 及其 tool_calls 元数据。
	// 不包含工具执行结果（tool role messages 在执行循环结束后才注入）。
	Messages []knot.Message

	// TurnCount 当前轮次编号。
	TurnCount int

	// ModelName 当前使用的模型名。
	ModelName string

	// MsgCount 本轮开始前的消息数量，用于恢复后计算 vault 增量。
	MsgCount int

	// ToolUses 本轮 LLM 返回的全部工具调用。
	// 已执行的条目 Result 已填充，当前待审批条目 Result 为空。
	ToolUses []knot.ToolUse

	// ToolUseIndex 当前卡在第几个工具调用（ToolUses 的下标）。
	ToolUseIndex int

	// AskReason brig.Evaluate 返回的审批原因。
	AskReason string

	// ToolMeta 已执行工具的结果元数据，key 为 tool_use ID。
	// 用于恢复后调用 appendRecords 写入 vault。
	ToolMeta map[string]vault.ToolRecordMeta
}
