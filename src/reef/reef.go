package reef

import (
	"context"

	"argo/src/knot"
)

// Compact 裁剪消息历史以适配 token 上限。
// 桩：原样返回。
func Compact(ctx context.Context, messages []knot.Message, tokenLimit knot.TokenLimit) ([]knot.Message, error) {
	return messages, nil
}
