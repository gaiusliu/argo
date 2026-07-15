package deck

// EvidenceSlot 标记证据槽位的类型，用于 Phase 2 的 Observation 结构
type EvidenceSlot string

const (
	SlotAnswers       EvidenceSlot = "answers"         // 回答原问题的证据
	SlotCounter       EvidenceSlot = "counter_evidence" // 推翻当前假设的证据（强制保留）
	SlotUnexpected    EvidenceSlot = "unexpected"       // 未问到的异常信息（强制保留）
	SlotDeferredLeads EvidenceSlot = "deferred_leads"   // 可能有用但不应立刻展开
)

// ToolIntent 工具调用前模型声明的意图（Phase 2 核心理念）
type ToolIntent struct {
	Hypothesis string // 当前假设
	Question   string // 这次调用要回答的问题
	Need       string // 需要什么证据才算够
	StopWhen   string // 什么情况下应该停止调查
}

// Observation 工具返回后系统提取的证据包（Phase 2）
type Observation struct {
	Answers       string `json:"answers"`         // 回答原问题的证据
	Counter       string `json:"counter_evidence"` // 推翻当前假设的证据
	Unexpected    string `json:"unexpected"`       // 异常信息
	DeferredLeads string `json:"deferred_leads"`   // 延迟展开的方向
	ArtifactID    string `json:"artifact_id"`      // 原文重取入口
}
