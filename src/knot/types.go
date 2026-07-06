package knot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// EventType 流式事件类型标识
type EventType string

// Event 类型常量
const (
	EventStart         EventType = "start"
	EventTextStart     EventType = "textStart"
	EventTextDelta     EventType = "textDelta"
	EventTextDone      EventType = "textDone"
	EventThinkingStart EventType = "thinkingStart"
	EventThinkingDelta EventType = "thinkingDelta"
	EventThinkingDone  EventType = "thinkingDone"
	EventToolUseStart  EventType = "toolUseStart"
	EventToolUseDelta  EventType = "toolUseDelta"
	EventToolUseDone   EventType = "toolUseDone"
	EventDone          EventType = "done"
	EventError         EventType = "error"
)

// MessageRole 消息角色类型
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleTool      MessageRole = "tool"
	MessageRoleSystem    MessageRole = "system"
)

// Message 单条对话消息
type Message struct {
	// Role 取值为 MessageRoleSystem | MessageRoleUser | MessageRoleAssistant | MessageRoleTool
	Role    MessageRole
	Content string
}

// ChatRequest LLM 调用请求
type ChatRequest struct {
	Messages []Message
	Tools    []Tool
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
	Type    EventType
	Delta   string // textDelta 或 thinkingDelta，由 Type 区分
	ToolUse ToolUse
	Usage   TokenUsage // EventDone 时携带的消耗统计
	Err     error
}

// ToolUse 工具调用，Result.Status == StatusDefault 表示待执行
type ToolUse struct {
	ID         string
	Name       string
	Parameters map[string]any
	Result     ToolResult
}

// ToolSource 工具来源
type ToolSource int

const (
	SourceBuiltin ToolSource = iota
	SourceCLI
)

// Tool 工具接口，由 deck 包实现
type Tool interface {
	Name() string
	Description() string
	Source() ToolSource
	// ParametersSchema 返回工具参数的 JSON Schema，供 LLM function calling 使用。
	ParametersSchema() map[string]any
	Execute(ctx context.Context, params map[string]any) (ToolResult, error)
}

// ResultStatus 工具执行结果的状态码
type ResultStatus int

const (
	StatusDefault ResultStatus = iota
	StatusSuccess
	StatusError
)

// ToolResult 工具执行返回结果
type ToolResult struct {
	Output   string
	Status   ResultStatus
	Metadata map[string]any
}

// AskUser 用户确认回调：RunLoop 判定工具调用为 Ask 时调用。
// tu 为待确认的工具调用，reason 为需要确认的原因。
// 返回 true 表示用户同意执行，调用方追加动态 Allow 规则后继续。
type AskUser func(tu ToolUse, reason string) (bool, error)

// 工具参数键常量——deck、brig、sail 共用的规范字段名。
// 新增工具或参数时在此处追加，不各处硬编码字符串字面量。
const (
	ParamCmd        = "cmd"         // bash：shell 命令字符串
	ParamPath       = "path"        // read/write/edit/grep/glob：文件路径
	ParamURL        = "url"         // web_fetch：请求 URL
	ParamContent    = "content"     // write：写入内容
	ParamOldStr     = "old_string"  // edit：待替换的原始文本
	ParamNewStr     = "new_string"  // edit：替换后的新文本
	ParamPattern    = "pattern"     // grep/glob：匹配模式
	ParamQuery      = "query"       // web_search：搜索词
	ParamArgs       = "args"        // CLI 工具：传给外部二进制的参数
	ParamLimit      = "limit"       // read/grep/glob：结果上限
	ParamOffset     = "offset"      // read：起始行号
	ParamReplaceAll = "replace_all" // edit：是否替换全部匹配
	ParamName       = "name"        // lookup：要查询的工具名
)

// ConfigPath 返回全局配置文件路径：$ARGO_HOME/config.json 或 ~/.argo/config.json
func ConfigPath() (string, error) {
	if home := os.Getenv("ARGO_HOME"); home != "" {
		return filepath.Join(home, "config.json"), nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config path: %w", err)
	}
	return filepath.Join(dir, ".argo", "config.json"), nil
}

// Config 全局配置，聚合各模块配置
type Config struct {
	WorkingDir string     `json:"-"` // 启动时 os.Getwd()，不序列化
	Deck       DeckConfig `json:"deck"`
	Sail       SailConfig `json:"sail"`
}

// DeckConfig deck 模块配置
type DeckConfig struct {
	CLI map[string]CLIToolEntry `json:"cli"`
}

// CLIToolEntry 单个 CLI 工具的配置条目
type CLIToolEntry struct {
	Description string `json:"description"`
}

// SailConfig sail 模块配置
type SailConfig struct {
	Model    string                    `json:"model,omitempty"`
	Provider map[string]ProviderConfig `json:"provider"`
}

// ProviderConfig provider 级别配置
type ProviderConfig struct {
	Name    string                 `json:"name,omitempty"`
	Env     []string               `json:"env,omitempty"`
	Options map[string]any         `json:"options,omitempty"`
	Models  map[string]ModelConfig `json:"models"`
}

// ModelConfig 单个模型配置
type ModelConfig struct {
	Name      string         `json:"name,omitempty"`
	Reasoning bool           `json:"reasoning,omitempty"`
	Limit     *ModelLimit    `json:"limit,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
}

// ModelLimit token 限制，nil 使用 API 默认值
type ModelLimit struct {
	Context *int `json:"context,omitempty"`
	Input   *int `json:"input,omitempty"`
	Output  *int `json:"output,omitempty"`
}
