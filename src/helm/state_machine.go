package helm

import "argo/src/knot"

// LoopPhase RunLoop 状态机的当前阶段。
type LoopPhase int

const (
	PhaseIdle LoopPhase = iota
	PhaseLLM
	PhaseExecuteTools
	PhaseWaitAsk
	PhaseDone
)

// LoopState RunLoop 状态机的控制流状态，可序列化持久化到 state.json。
// 对话历史由 Vault 管理（transcript.jsonl），LoopState 不持有。
type LoopState struct {
	// Phase 当前所处的阶段。
	Phase LoopPhase

	// TurnCount 当前轮次编号。
	TurnCount int

	// ModelName 当前使用的模型名。
	ModelName string

	// ToolUses 本轮 LLM 返回的全部工具调用。
	// 已执行的条目 Result 已填充，未执行的 Result 为空。
	ToolUses []knot.ToolUse

	// ToolUseIndex 当前正在处理的工具调用下标（ToolUses 的索引）。
	ToolUseIndex int

	// AskReason 当前事件等待确认的原因（PhaseWaitAsk 时有效）。
	AskReason string

	// AskID 当前等待确认的事件标识（PhaseWaitAsk 时有效）。
	AskID string
}

// NewPromptState 构造新消息入口的 LoopState。
// 对话历史由调用方通过 Vault.Load() 获取，不在 LoopState 中持有。
func NewPromptState(modelName string) *LoopState {
	return &LoopState{
		ModelName: modelName,
		Phase:     PhaseLLM,
	}
}
