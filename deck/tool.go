package deck

import "context"

// Tool 是工具的统一抽象。CLI / MCP / builtin 三种来源分别实现此接口。
type Tool interface {
	Name() string
	Description() string
	Source() ToolSource
	Execute(ctx context.Context, params map[string]any) (ToolResult, error)
	Concurrency() ConcurrencyMode
}

// tools 是进程内唯一的工具注册表。
var tools = make(map[string]Tool)

// toolsOrder 记录注册顺序，保证 List 按插入顺序返回。
var toolsOrder []string

// NewBuiltinTool 创建内置工具实例。
func NewBuiltinTool(name, description string, handler func(ctx context.Context, params map[string]any) (ToolResult, error), concurrency ConcurrencyMode) *builtinTool {
	return &builtinTool{
		name:        name,
		description: description,
		handler:     handler,
		concurrency: concurrency,
	}
}
