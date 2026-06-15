package deck

import (
	"context"
	"encoding/json"
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
