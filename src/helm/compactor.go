package helm

import (
	"context"
	"log/slog"

	"argo/src/knot"
	"argo/src/sail"
)

// Compactor 压缩消息历史以适应 token 上限。
type Compactor interface {
	// ShouldCompact 判断是否需要触发 compact，不执行实际压缩。
	ShouldCompact(inputTokens int) bool
	// Compact 执行压缩，返回摘要正文 + 被覆盖的消息区间 [from, to)。
	// 未触发时 from == to == 0。失败时应原样返回空值，不中断 agent loop。
	Compact(ctx context.Context, messages []knot.Message, inputTokens int) (summary string, from int, to int, err error)
}

// CompactorFactory 是 Compactor 的工厂函数。
// provider 为摘要生成所用的模型连接，opts 来自 config.json 的 compactOpts。
type CompactorFactory func(provider sail.Provider, opts map[string]any) (Compactor, error)

var compactors = make(map[string]CompactorFactory)

// RegisterCompactor 注册 Compactor 工厂。建议在 init() 中调用。
func RegisterCompactor(name string, factory CompactorFactory) {
	compactors[name] = factory
}

// ResolveCompactor 按名称解析 Compactor。
// name 为空时默认用 "llm-summary"；解析失败时回退 llm-summary。
func ResolveCompactor(name string, provider sail.Provider, opts map[string]any) (Compactor, error) {
	if name == "" {
		name = "llm-summary"
	}
	factory, ok := compactors[name]
	if !ok {
		slog.Warn("unknown compactor, falling back to llm-summary", "name", name)
		factory = compactors["llm-summary"]
	}
	return factory(provider, opts)
}
