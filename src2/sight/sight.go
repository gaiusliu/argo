// Package sight 定义 LLM 连接抽象，负责发送请求并返回流式事件。
package sight

import (
	"context"
	"net/http"

	"argo/src2/pact"
)

// Sight LLM 连接，单次构造后一组 hands 不变，每次 Take 为无状态调用。
type Sight interface {
	// Name 返回解析后的模型名。
	Name() string
	// Take 发送请求，返回流式事件 channel。channel 在所有事件发送完毕后关闭。
	Take(ctx context.Context, msgs []pact.Message) <-chan pact.Event
}

// New 从配置 + 模型名 + 工具描述符 + 共享 HTTP 客户端构造 Sight。
func New(cfg pact.SightCfg, model string, hands []pact.Hand, client *http.Client) (Sight, error) {
	for _, pc := range cfg.Providers {
		if pc.Models[model] {
			switch pc.Protocol {
			case pact.ProtocolOpenAI:
				return newOpenAI(model, pc.APIKey, pc.BaseURL, hands, client), nil
			default:
				return nil, &APIError{Code: "unsupported_protocol", Message: "unknown protocol: " + pc.Protocol}
			}
		}
	}
	return nil, &APIError{Code: "model_not_found", Message: "model " + model + " not configured"}
}
