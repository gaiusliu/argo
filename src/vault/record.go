package vault

import "time"

// RecordType transcript record 类型标识。
type RecordType string

// transcript record 类型常量。
const (
	RecordUser RecordType = "user"
	RecordLLM  RecordType = "llm"
	RecordTool RecordType = "tool"
)

// ToolVerdict 工具调用的审批结果。
type ToolVerdict string

const (
	VerdictAutoAllow   ToolVerdict = "auto_allow"   // 静态规则直接放行
	VerdictAutoDeny    ToolVerdict = "auto_deny"    // 静态规则直接拒绝
	VerdictCachedAllow ToolVerdict = "cached_allow" // 命中用户之前的审批记录自动放行
	VerdictAskAllow    ToolVerdict = "ask_allow"    // 用户当场确认同意
	VerdictAskDeny     ToolVerdict = "ask_deny"     // 用户当场拒绝
)

// Record transcript 中的一条记录。各类型共用同一结构，JSON tag + omitempty 控制序列化字段。
type Record struct {
	// 公共字段
	Type      RecordType `json:"type"`
	Seq       int        `json:"seq"`
	Timestamp time.Time  `json:"timestamp"`

	// user
	Content string `json:"content,omitempty"` // 用户输入原文

	// llm
	Model        string           `json:"model,omitempty"`         // 模型名
	ToolCalls    []ToolCallRecord `json:"tool_calls,omitempty"`    // 工具调用请求
	InputTokens  int              `json:"input_tokens,omitempty"`  // 输入 token 数，预留
	OutputTokens int              `json:"output_tokens,omitempty"` // 输出 token 数，预留

	// tool
	Verdict    ToolVerdict `json:"verdict,omitempty"`      // 审批结果
	ToolCallID string      `json:"tool_call_id,omitempty"` // 关联 llm.tool_calls
	Name       string      `json:"name,omitempty"`         // 工具名
	Status     string      `json:"status,omitempty"`       // success / error
	DurMs      int         `json:"dur_ms,omitempty"`       // 执行耗时（毫秒），预留
}

// ToolCallRecord LLM 工具调用请求。
type ToolCallRecord struct {
	ToolCallID string         `json:"tool_call_id"`
	Name       string         `json:"name"`
	Args       map[string]any `json:"args"`
}
