// Package sight 定义 LLM 连接抽象，负责发送请求并返回流式事件。
package sight

import (
	"context"
	"net/http"

	"argo/src2/deck"
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
func New(cfg SightCfg, model string, hands []deck.HandMeta, client *http.Client) (Sight, error) {
	for _, pc := range cfg.Providers {
		if pc.Models[model] {
			switch pc.Protocol {
			case ProtocolOpenAI:
				return newOpenAI(model, pc.APIKey, pc.BaseURL, hands, client), nil
			default:
				return nil, &APIError{Code: "unsupported_protocol", Message: "unknown protocol: " + pc.Protocol}
			}
		}
	}
	return nil, &APIError{Code: "model_not_found", Message: "model " + model + " not configured"}
}

// Provider 协议类型常量
const (
	// ProtocolOpenAI OpenAI Chat Completions 协议
	ProtocolOpenAI = "openai-completions"
)

// SightCfg LLM 调用相关配置。
type SightCfg struct {
	// DefaultModel 默认模型名
	DefaultModel string `json:"default_model,omitempty"`
	// Providers provider 名 → 配置的映射
	Providers map[string]ProviderCfg `json:"providers"`
}

// ProviderCfg 单个 LLM provider 配置。
type ProviderCfg struct {
	// Protocol 协议类型，取值见 Protocol* 常量，空字符串默认为 openai-completions
	Protocol string `json:"protocol,omitempty"`
	// APIKey API 密钥
	APIKey string `json:"api_key"`
	// BaseURL API 基础地址
	BaseURL string `json:"base_url,omitempty"`
	// Models 该 provider 支持的模型集合
	Models map[string]bool `json:"models"`
}
