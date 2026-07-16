package vault

import (
	"time"
)

// transcript record 类型常量。
const (
	RecordUser    string = "user"
	RecordModel   string = "model"
	RecordTool    string = "tool"
	RecordCompact string = "compact"
)

// ToolVerdict 工具调用的审批结果。

// Record transcript 中的一条记录。各类型共用同一结构，JSON tag + omitempty 控制序列化字段。
type Record struct {
	// 公共字段
	Type      string    `json:"type"`
	Seq       int       `json:"seq"`
	Timestamp time.Time `json:"timestamp"`

	// user
	Content string `json:"content,omitempty"` // 用户输入原文

	// model
	Model     string           `json:"model,omitempty"`      // 模型名
	ToolCalls []ToolCallRecord `json:"tool_calls,omitempty"` // 工具调用请求

	// tool
	Verdict    int    `json:"verdict,omitempty"`      // 审批结果
	ToolCallID string `json:"tool_call_id,omitempty"` // 关联 model.tool_calls
	Name       string `json:"name,omitempty"`         // 工具名
	Status     string `json:"status,omitempty"`       // success / error

	// compact
	CompactFrom    int    `json:"compact_from,omitempty"`    // 被压缩区间起始索引
	CompactTo      int    `json:"compact_to,omitempty"`      // 被压缩区间结束索引
	CompactSummary string `json:"compact_summary,omitempty"` // LLM 摘要正文
}

// ToolCallRecord LLM 工具调用请求。
type ToolCallRecord struct {
	ToolCallID string         `json:"tool_call_id"`
	Name       string         `json:"name"`
	Args       map[string]any `json:"args"`
}
