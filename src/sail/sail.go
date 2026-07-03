package sail

import (
	"context"

	"argo/src/knot"
)

// Chat 发送 LLM 请求，返回流式事件 channel。
// 桩：直接发送 done 事件后关闭 channel。
func Chat(ctx context.Context, modelName string, req knot.ChatRequest) (<-chan knot.Event, error) {
	out := make(chan knot.Event, 1)

	go func() {
		defer close(out)
		out <- knot.Event{
			Type: knot.EventDone,
			Done: &knot.DonePayload{Text: "sail stub: 模型就绪"},
		}
	}()

	return out, nil
}

// Window 返回模型的 context window token 上限。
// 桩：返回保守默认值 128000。
func Window(modelName string) knot.TokenLimit {
	return knot.TokenLimit{MaxTokens: 128000}
}
