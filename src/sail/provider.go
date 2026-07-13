package sail

import (
	"context"
	"fmt"
	"os"

	"argo/src/knot"
)

// Provider 一个可用的 LLM 连接——封装供应商、模型、鉴权和调用。
type Provider interface {
	Name() string  // 供应商名（openai / deepseek / …）
	Model() string // 解析后的模型名（空入参已兜底为默认值）
	Chat(ctx context.Context, messages []knot.Message, tools []knot.Tool) <-chan knot.Event
}

// NewProvider 从配置解析 model → provider → apiKey → baseURL。
// model 为空时使用配置中的默认模型。
func NewProvider(model string) (Provider, error) {
	cfg, err := knot.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("sail provider: %w", err)
	}

	// 模型名为空时使用配置默认值
	if model == "" {
		model = cfg.Sail.Model
	}

	// 遍历 provider，找到 model 所属的供应商
	var pName string
	var pCfg knot.ProviderConfig
	for name, p := range cfg.Sail.Provider {
		if _, ok := p.Models[model]; ok {
			pName, pCfg = name, p
			break
		}
	}
	if pName == "" {
		return nil, fmt.Errorf("sail provider: model %q not in any provider", model)
	}

	// 从环境变量读取 API Key
	if len(pCfg.Env) == 0 {
		return nil, fmt.Errorf("sail provider: provider %q env not set", pName)
	}
	apiKey := os.Getenv(pCfg.Env[0])
	if apiKey == "" {
		return nil, fmt.Errorf("sail provider: env %s empty", pCfg.Env[0])
	}

	// 按 api 协议路由到对应实现
	switch pCfg.API {
	case "", "openai-completions":
		baseURL := openAIBaseURL
		if pName != "openai" {
			if u, ok := pCfg.Options["base_url"].(string); ok && u != "" {
				baseURL = u
			}
		}
		return &ProviderOpenAI{
			Provider: pName,
			model:    model,
			APIKey:   apiKey,
			BaseURL:  baseURL,
		}, nil
	default:
		return nil, fmt.Errorf("sail provider: unsupported api %q for provider %q", pCfg.API, pName)
	}
}
