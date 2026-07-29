// Package pact 定义 Argo 各模块间共享的公共数据类型和配置协议。
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
}

// ── 流式事件 ──

// EventKind 流式事件类型。
type EventKind string

const (
	// KindStart 流开始
	KindStart EventKind = "start"
	// KindTextStart 文本段开始
	KindTextStart EventKind = "textStart"
	// KindTextDelta 文本增量
	KindTextDelta EventKind = "textDelta"
	// KindTextDone 文本段结束
	KindTextDone EventKind = "textDone"
	// KindThinkingStart 思考段开始
	KindThinkingStart EventKind = "thinkingStart"
	// KindThinkingDelta 思考增量
	KindThinkingDelta EventKind = "thinkingDelta"
	// KindThinkingDone 思考段结束
	KindThinkingDone EventKind = "thinkingDone"
	// KindToolUseStart 工具调用开始
	KindToolUseStart EventKind = "toolUseStart"
	// KindToolUseDelta 工具调用增量
	KindToolUseDelta EventKind = "toolUseDelta"
	// KindToolUseDone 工具调用结束
	KindToolUseDone EventKind = "toolUseDone"
	// KindDone 整个响应结束
	KindDone EventKind = "done"
	// KindError 错误
	KindError EventKind = "error"
	// KindAsk 需要用户确认
	KindAsk EventKind = "ask"
)

// Event 流式事件，sight.Take() 通过 channel 推送。
type Event struct {
	// Kind 事件类型
	Kind EventKind
	// Delta textDelta / thinkingDelta 的增量文本
	Delta string
	// Omen toolUse* / ask 时携带
	Omen Omen
	// Usage KindDone 时携带的 token 消耗统计
	Usage TokenUsage
	// Err KindError 时携带的错误
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

// ── 工具描述符 ──

// Hand 工具元数据，供 LLM function calling 使用。由 deck 包提供，sight 包消费。
type Hand struct {
	// Name 工具名
	Name string
	// Description 工具描述
	Description string
	// Parameters JSON Schema 参数定义
	Parameters map[string]any
}

// ── 配置协议 ──

// AgentCfg 请求级配置，每次 handlePrompt 时热重载。
type AgentCfg struct {
	Sight SightCfg `json:"sight"`
	Press PressCfg `json:"press"`
	Clip  ClipCfg  `json:"clip"`
}

// SightCfg LLM 调用相关配置。
type SightCfg struct {
	// DefaultModel 默认模型名
	DefaultModel string `json:"default_model,omitempty"`
	// Providers provider 名 → 配置的映射
	Providers map[string]ProviderCfg `json:"providers"`
}

// ProviderCfg 单个 LLM provider 配置。
type ProviderCfg struct {
	// Protocol 协议类型，取值见 Protocol* 常量，空字符串默认为 openai-completions
	Protocol string `json:"protocol,omitempty"`
	// APIKey API 密钥
	APIKey string `json:"api_key"`
	// BaseURL API 基础地址
	BaseURL string `json:"base_url,omitempty"`
	// Models 该 provider 支持的模型集合
	Models map[string]bool `json:"models"`
}

// Provider 协议类型常量
const (
	// ProtocolOpenAI OpenAI Chat Completions 协议
	ProtocolOpenAI = "openai-completions"
)

// PressCfg 上下文压缩配置。
type PressCfg struct {
	// Strategy 压缩策略名，默认 "llm-summary"
	Strategy string `json:"strategy,omitempty"`
	// Threshold 触发压缩的上下文窗口百分比（1-100），默认 80
	Threshold int `json:"threshold,omitempty"`
}

// ClipCfg 工具输出截断配置。
type ClipCfg struct {
	// Strategy 截断策略名，默认 "head-tail"
	Strategy string `json:"strategy,omitempty"`
}

// ServerCfg 进程级配置，启动时加载一次，运行期不变。
type ServerCfg struct {
	// Addr 监听地址，如 ":8080"
	Addr string `json:"addr"`
	// Log 日志配置
	Log LogCfg `json:"log"`
}

// LogCfg 日志配置。
type LogCfg struct {
	// Dir 日志目录
	Dir string `json:"dir,omitempty"`
	// Level 日志级别：debug / info / warn / error
	Level string `json:"level,omitempty"`
	// MaxSize 单文件大小上限（MB）
	MaxSize int `json:"max_size,omitempty"`
	// MaxAge 文件保留天数
	MaxAge int `json:"max_age,omitempty"`
}
