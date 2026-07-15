package deck

import (
	"log/slog"

	"argo/src/knot"
)

// Truncator 对单次工具结果做后处理（截断、证据提取等）。
// Truncate 失败时应原样返回 result，不中断工具执行流。
type Truncator interface {
	Truncate(result knot.ToolResult) knot.ToolResult
}

// TruncatorFactory 是 Truncator 的工厂函数，opts 来自 config.json 的 truncationOpts
type TruncatorFactory func(opts map[string]any) (Truncator, error)

var truncators = make(map[string]TruncatorFactory)

// RegisterTruncator 注册 Truncator 工厂。建议在 init() 中调用。
func RegisterTruncator(name string, factory TruncatorFactory) {
	truncators[name] = factory
}

// ResolveTruncator 按名称解析 Truncator。
// name 为空时默认用 "head-tail"；解析失败时回退 head-tail。
func ResolveTruncator(name string, opts map[string]any) (Truncator, error) {
	if name == "" {
		name = "head-tail"
	}
	factory, ok := truncators[name]
	if !ok {
		slog.Warn("unknown truncator, falling back to head-tail", "name", name)
		factory = truncators["head-tail"]
	}
	return factory(opts)
}
