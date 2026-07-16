package helm

import (
	"context"

	"argo/src/knot"
)

// Compactor 压缩消息历史以适应 token 上限。
type Compactor interface {
	// ShouldCompact 判断是否需要触发 compact，不执行实际压缩。
	ShouldCompact(inputTokens int) bool
	// Compact 执行压缩，返回摘要正文 + 被覆盖的消息区间 [from, to)。
	// 未触发时 from == to == 0。失败时应原样返回空值，不中断 agent loop。
	Compact(ctx context.Context, messages []knot.Message, inputTokens int) (summary string, from int, to int, err error)
}
