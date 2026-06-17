package deck

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolSource 表示工具的注册来源。
type ToolSource string

const (
	ToolSourceBuiltin ToolSource = "builtin"
	ToolSourceCLI     ToolSource = "cli"
	ToolSourceMCP     ToolSource = "mcp"
)

// ConcurrencyMode 表示工具的执行模式：并发或串行。
type ConcurrencyMode string

const (
	Concurrent ConcurrencyMode = "concurrent"
	Sequential ConcurrencyMode = "sequential"
)

// MaxOutputChars 单工具输出的默认字符数上限。
const MaxOutputChars = 50000

// TruncationMinSavings 截断最小节省量。只有当原文超出上限至少这么多字节时才触发截断，
// 避免"截断标记 + head + tail 比原文还长"的情况。
const TruncationMinSavings = 200

// ToolResult 表示一次工具调用的结果。
type ToolResult struct {
	Output   string
	Status   string // success | error | timeout
	Metadata map[string]any
}

// ToolCall 表示一次待执行的工具调用请求。
type ToolCall struct {
	Name   string
	Params map[string]any
}

// ToolsConfig 对应 ~/.argo/tools.json 的配置结构。
type ToolsConfig struct {
	CLI []CLIToolEntry `json:"cli"`
	MCP []MCPToolEntry `json:"mcp"`
}

// CLIToolEntry 对应 CLI 工具条目。
type CLIToolEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// MCPToolEntry 对应 MCP 工具条目。
type MCPToolEntry struct {
	Name    string   `json:"name"`
	Command []string `json:"command"`
}

// builtinTool 通过 handler 函数指针注册的内置工具。
type builtinTool struct {
	name        string
	description string
	handler     func(ctx context.Context, params map[string]any) (ToolResult, error)
	concurrency ConcurrencyMode
}

// cliTool 将外部 CLI 命令适配为 Tool 接口。
type cliTool struct {
	name        string
	description string
	binary      string
	concurrency ConcurrencyMode
}

// mcpTool 将 MCP 协议工具适配为 Tool 接口，其实现与 MCPManager 绑定。
// 两者因循环引用合并为一个 task，MCPManager 在 mcp.go 中实现。
type mcpTool struct {
	description string
	serverName  string
	toolName    string
	inputSchema json.RawMessage
	manager     *MCPManager
	concurrency ConcurrencyMode
}

// TruncationStrategy 表示截断策略枚举。
type TruncationStrategy int

const (
	TruncationHead         TruncationStrategy = iota // 保留前部
	TruncationTail                                   // 保留尾部
	TruncationHeadAndTail                            // 保留头尾
)

// String 返回截断策略的字符串表示，用于 metadata 写入与 JSON 序列化。
func (s TruncationStrategy) String() string {
	switch s {
	case TruncationHead:
		return "head"
	case TruncationTail:
		return "tail"
	case TruncationHeadAndTail:
		return "HeadAndTail"
	default:
		return "unknown"
	}
}

// TruncationConfig 定义截断行为的可配置参数。
// Ratio 和 TailRatio 为 1~80 的整数百分点（含义"保留 N% 字符"），详见 DEC-010。
type TruncationConfig struct {
	Strategy  TruncationStrategy `json:"strategy"`
	Ratio     int                `json:"ratio"`
	TailRatio int                `json:"tail_ratio"`
}

// Validate 校验 TruncationConfig 的合法性。
// Ratio 必须 > 0 且 ≤ 80（百分点整数）；HeadAndTail 策略下 TailRatio 同样校验。
func (c TruncationConfig) Validate() error {
	if c.Ratio <= 0 || c.Ratio > 80 {
		return fmt.Errorf("ratio must be in (0, 80] (percent), got %d", c.Ratio)
	}
	if c.Strategy == TruncationHeadAndTail {
		if c.TailRatio <= 0 || c.TailRatio > 80 {
			return fmt.Errorf("tail_ratio must be in (0, 80] (percent), got %d", c.TailRatio)
		}
	}
	return nil
}
