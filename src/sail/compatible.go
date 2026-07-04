package sail

import (
	"context"

	"argo/src/knot"
)

// CompatibleChat 以用户指定的 baseURL 为端点的流式 LLM 调用，覆盖 DeepSeek / Kimi / MiniMax / 豆包
func CompatibleChat(ctx context.Context, apiKey, baseURL, modelName string, req knot.ChatRequest) (<-chan knot.Event, error) {
	return doChat(ctx, apiKey, baseURL, modelName, req)
}
