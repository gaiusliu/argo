# sail

## 概述

sail 是 LLM 接入层，负责将"模型名"解析为可用的连接参数并执行流式 Chat 调用。对外暴露 `Provider` 接口，隐藏配置解析、API Key 读取、协议路由等细节。消费者只需持有 `Provider` 即可调用 LLM，不关心供应商差异。

## 核心概念

| 概念 | 说明 |
|------|------|
| `Provider` | 一个可用的 LLM 连接，暴露 Name / Model / Chat 三个方法 |
| `NewProvider` | 工厂函数，从 `knot.Config` 按模型名解析出 Provider 实现 |
| `ProviderOpenAI` | OpenAI API + 所有 OpenAI 兼容 API 的 Provider 实现 |
| `ProviderConfig.API` | 供应商协议字段（`"openai-completions"` 或空默认），`NewProvider` 据此路由 |
| `HTTPClient` | 可注入的 `*http.Client`，nil 时回退 `http.DefaultClient` |

## 行为合约

### 模型解析

`NewProvider(model)` 的行为保证：

- `model` 为空 → 使用配置中的默认模型
- 遍历配置中所有 provider，找到 model 所属的供应商
- 从对应环境变量读取 API Key
- OpenAI 原生 provider 使用固定 BaseURL；其余 compatible provider 从 `Options["base_url"]` 读取，回退到 OpenAI 地址
- 不支持的 API 协议返回 error

### Chat 调用

`Provider.Chat(ctx, messages, tools)` 的行为保证：

- 按 OpenAI `/v1/chat/completions` 协议构建请求体
- 支持流式 SSE 解析
- 网络错误自动重试（最多 3 次，指数退避：1s/2s/4s）
- 429 状态码优先使用 `Retry-After` 头指定的退避时间
- 非 200 状态码按错误分类（ratelimit / auth / bill / safety / server_error / network）
- context 取消时立即终止

### HTTP Client 注入

- 测试或自定义场景可通过 `NewProviderOpenAI(..., httpClient)` 注入自定义 `*http.Client`
- 生产代码默认 nil，自动使用 `http.DefaultClient`
