package sail

import (
	"context"

	"argo/src/knot"
)

const openAIBaseURL = "https://api.openai.com/v1/chat/completions"

// OpenAIChat 以 api.openai.com 为端点的流式 LLM 调用
func OpenAIChat(ctx context.Context, apiKey, modelName string, req knot.ChatRequest) (<-chan knot.Event, error) {
	return doChat(ctx, apiKey, openAIBaseURL, modelName, req)
}
