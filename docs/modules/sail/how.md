# sail 实现设计

## 概述

sail 实现 helm 定义的 `LLM` 接口，提供三个 provider adapter 屏蔽不同 API 的差异。启动时加载 `~/.argo/models.json`，首次启动自动生成默认配置。

## 内部结构

```
sail/
├── config.go       // LoadConfig() 加载 + 自动生成 models.json
├── client.go       // Client：模型管理 + LLM 接口实现
├── adapters.go     // anthropicAdapter / openaiAdapter / openaiCompatibleAdapter
├── errors.go       // APIError 错误分类
├── metadata.go     // ModelCapability 内置表 + OpenRouter 更新
├── types.go        // ModelConfig, TokenUsage, APIError
└── sail_test.go
```

## 核心类型

```go
// ModelConfig — models.json 中每条模型的配置
type ModelConfig struct {
    Provider string // "anthropic" | "openai" | "openai-compatible"
    Model    string
    BaseURL  string // 可选，openai-compatible 需要
    APIKey   string // 支持 ${ENV_VAR} 语法
}

// TokenUsage — 从 API 响应提取，归一化
type TokenUsage struct {
    InputTokens  int
    OutputTokens int
}

// APIError — 错误分类
type APIError struct {
    StatusCode int
    Code       string // "auth" | "quota" | "rate_limit" | "server_error" | "context_overflow" | "network"
    Retryable  bool
    Message    string
}

// ModelCapability — 模型能力元数据
type ModelCapability struct {
    ContextWindow int  // max input tokens
    MaxOutput     int
    SupportsTools bool
    SupportsImages bool
}
```

## 配置文件

`~/.argo/models.json`：

```json
{
  "default": "claude",
  "models": {
    "claude": {
      "provider": "anthropic",
      "model": "claude-sonnet-4-20250514",
      "api_key": "${ANTHROPIC_API_KEY}"
    },
    "gpt": {
      "provider": "openai",
      "model": "gpt-4o",
      "api_key": "${OPENAI_API_KEY}"
    },
    "deepseek": {
      "provider": "openai-compatible",
      "model": "deepseek-v4-pro",
      "base_url": "https://api.deepseek.com",
      "api_key": "${DEEPSEEK_API_KEY}"
    }
  }
}
```

## 函数签名

```go
// Client — sail 入口，实现 helm 的 LLM 接口
type Client struct { ... }
func NewClient(cfgPath string) (*Client, error)

func (c *Client) Chat(ctx context.Context, modelName string, req ChatRequest) (<-chan Event, error)
func (c *Client) Window(modelName string) TokenLimit

// LoadConfig — 加载 models.json，不存在则自动生成
func LoadConfig(path string) (*ModelsConfig, error)

// GenerateDefaultConfig — 扫描环境变量，生成默认配置
func GenerateDefaultConfig() *ModelsConfig

// classifyError — 错误分类
func classifyError(statusCode int, body []byte) *APIError

// ModelCapability lookups
func lookupCapability(model string) (ModelCapability, bool)
func (c *Client) RefreshCapabilities(ctx context.Context) error // 从 OpenRouter 更新
```

## 数据流

```
启动时:
  LoadConfig("~/.argo/models.json")
    ├── 文件存在 → 加载
    ├── 文件不存在 → GenerateDefaultConfig() → 扫描环境变量 → 生成默认配置
    └── RefreshCapabilities() → 从 OpenRouter 拉取模型元数据（后台，不阻塞）

运行时（Chat）:
  helm → sail.Chat(ctx, modelName, req)
    │
    ├── 1. 查找模型配置
    │     models.json[modelName] → ModelConfig{Provider, Model, BaseURL, APIKey}
    │     解析 ${VAR} → 环境变量
    │
    ├── 2. 选择 adapter
    │     provider=="anthropic"         → anthropicAdapter
    │     provider=="openai"            → openaiAdapter
    │     provider=="openai-compatible" → openaiCompatibleAdapter(baseURL)
    │
    ├── 3. 构造请求 + 发送
    │     adapter.Chat(ctx, model, messages, tools)
    │       ├── 成功 → 流式消费 HTTP body → 推送 Event{text_delta}
    │       ├── 有 tool_calls → 推送 Event{tool_use}
    │       ├── 完成 → 推送 Event{done, TokenUsage}
    │       └── 失败 → classifyError() → 返回 *APIError
    │
    └── 4. 返回 <-chan Event
```

## 模型元数据三级来源

```
优先级（高→低）：
  1. 用户手动覆盖（models.json 中的 max_input_tokens 字段）
  2. OpenRouter API（RefreshCapabilities 拉取的最新数据）
  3. 内置默认表（硬编码 5-6 个主流模型）

如果三级都没有相应模型的元数据 → 使用保守默认值（Window=128000）
```
