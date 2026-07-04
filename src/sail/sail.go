package sail

import (
	"context"
	"fmt"
	"os"

	"argo/src/knot"
)

// Chat 发送 LLM 流式请求，返回事件 channel。
// 查找模型所属 provider → 读 API Key → 路由 adapter → 透传事件流。
func Chat(ctx context.Context, modelName string, req knot.ChatRequest) (<-chan knot.Event, error) {
	// 查找模型所属的 provider 配置
	cfg, err := knot.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("sail: %w", err)
	}
	var pName string
	var pCfg knot.ProviderConfig
	for name, p := range cfg.Sail.Provider {
		if _, ok := p.Models[modelName]; ok {
			pName, pCfg = name, p
			break
		}
	}
	if pName == "" {
		return nil, fmt.Errorf("sail: model %q not in any provider", modelName)
	}

	// 从环境变量读取 API Key
	if len(pCfg.Env) == 0 {
		return nil, fmt.Errorf("sail: provider %q env not set", pName)
	}
	apiKey := os.Getenv(pCfg.Env[0])
	if apiKey == "" {
		return nil, fmt.Errorf("sail: env %s empty", pCfg.Env[0])
	}

	// 按 provider 名称路由到对应 adapter
	if pName == "openai" {
		return OpenAIChat(ctx, apiKey, modelName, req)
	}
	baseURL, _ := pCfg.Options["base_url"].(string)
	if baseURL == "" {
		baseURL = openAIBaseURL
	}
	return CompatibleChat(ctx, apiKey, baseURL, modelName, req)
}

// Window 返回模型的 context window token 上限。
// 桩：返回保守默认值 128000。
func Window(modelName string) knot.TokenLimit {
	return knot.TokenLimit{MaxTokens: 128000}
}
