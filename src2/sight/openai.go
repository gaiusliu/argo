package sight

import (
	"context"
	"net/http"

	"argo/src2/pact"
)

// openAI 实现 Sight，字段仅含长生命周期配置，每次 Take 内部构建请求。
type openAI struct {
	model   string
	key     string
	baseURL string
	hands   []pact.Hand
	client  *http.Client
}

func newOpenAI(model, key, baseURL string, hands []pact.Hand, client *http.Client) *openAI {
	return &openAI{
		model:   model,
		key:     key,
		baseURL: baseURL,
		hands:   hands,
		client:  client,
	}
}

func (o *openAI) Name() string { return o.model }

// Take 将 pact 消息+工具转为 API 请求，通过 HTTP 流式调用并推送 pact.Event。
func (o *openAI) Take(ctx context.Context, msgs []pact.Message) <-chan pact.Event {
	req := o.buildRequest(msgs)
	return o.doChat(ctx, req)
}

// buildRequest 将 pact.Message / pact.Hand 转为内部 API 请求结构。
func (o *openAI) buildRequest(msgs []pact.Message) apiReq {
	req := apiReq{
		Model:    o.model,
		Messages: make([]apiMsg, 0, len(msgs)),
		Hands:    make([]apiTool, 0, len(o.hands)),
		Stream:   true,
	}
	for _, m := range msgs {
		req.Messages = append(req.Messages, apiMsg{
			Role:    m.Role,
			Content: m.Content,
			OmenID:  m.OmenID,
		})
	}
	for _, t := range o.hands {
		req.Hands = append(req.Hands, apiTool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}
	return req
}

// doChat 为 stub 实现，后续从 src/sight/ 搬运完整 HTTP+SSE 流解析逻辑。
func (o *openAI) doChat(ctx context.Context, req apiReq) <-chan pact.Event {
	out := make(chan pact.Event, 1)
	go func() {
		defer close(out)
		// TODO：替换为真实 HTTP 请求 + SSE 流读取
		for {
			select {
			case <-ctx.Done():
				out <- pact.Event{Type: pact.EventTypeError, Err: ctx.Err()}
				return
			default:
				out <- pact.Event{Type: pact.EventTypeTextDelta, Delta: "（stub：sight.openAI.doChat 待实现）"}
				out <- pact.Event{Type: pact.EventTypeDone}
				return
			}
		}
	}()
	return out
}
