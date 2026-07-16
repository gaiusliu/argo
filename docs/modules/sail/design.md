# sail

## 概述

sail 是 Argo 的 LLM 接入层。通过 `Provider` 接口向上层暴露统一调用入口，内部封装配置解析、API Key 读取、HTTP 请求、SSE 流解析和错误分类。一份 OpenAI Chat Completions SSE 解析实现覆盖所有兼容供应商，区别仅在 baseURL 和 API Key。

sail 依赖 `knot`，不依赖 helm/deck/brig/voyage。

## 核心概念

| 概念 | 说明 |
|------|------|
| `Provider` | 统一 LLM 调用接口：`Name() / Model() / Chat(ctx, messages, tools) <-chan knot.Event` |
| `NewProvider` | 工厂函数：model name → 查配置 → 读 env API Key → 返回 `ProviderOpenAI`。model 为空时使用配置默认值 |
| `ProviderOpenAI` | Provider 的唯一实现。持有 model/provider/apiKey/baseURL/messages/tools/HTTPClient，`Chat()` 方法执行 `buildRequestBody` + `doChat` |
| `NewProviderOpenAI` | 显式构造器（供 helm compact_llm_summary 等场景使用），不依赖 `knot.GetConfig()` |
| `doChat` | HTTP POST + 3 次重试 + 429 Retry-After 优先 / 指数退避 1s/2s/4s。200 → `parseSSE` goroutine；非 200 → `classifyError` 判定是否可重试 |
| `parseSSE` | `bufio.Scanner` 逐行读取 `text/event-stream`。`context.AfterFunc(ctx, body.Close)` 实现安全退出，零 goroutine 泄漏 |
| `streamState` | 流式事件状态机，跟踪 text/thinking/tool_calls 三块的活跃状态，决定每个 chunk 应推的 EventType |
| `handleChunk` | 解析 SSE chunk JSON → 按 reasoning → content → tool_calls 顺序判状态 → 返回事件列表 |
| `flush` | `[DONE]` 或 EOF 时：关闭所有活跃 block（textDone/thinkingDone）→ `json.Unmarshal` 累积的 arguments 为扁平 Parameters → 发出 `EventToolUseDelta` |
| `buildRequestBody` | 将 `knot.Message` 列表 + `knot.Tool` 列表转为 OpenAI JSON 请求体 |
| `Window` | 三级查找模型 context window：用户配置 > `models.dev` 缓存 > 保守默认 128000 |
| `APIError` | 结构化错误：Code / Retryable / RetryAfterSeconds / Message |
| `classifyError` | HTTP 状态码 + 响应体 + 响应头 → `APIError`，429 内部区分 billing 耗尽和频率限制 |

## 行为合约

### Chat — 流式 LLM 调用

- **输入**：`ctx context.Context`、`messages []knot.Message`、`tools []knot.Tool`
- **输出**：`<-chan knot.Event` 事件流（从不返回 error——配置错误在 `NewProvider` 阶段已处理）
- **内部流程**：`buildRequestBody()` → `doChat(ctx)` → 返回 event channel

### doChat — HTTP 层 + 重试

- HTTP 头：`Authorization: Bearer {apiKey}` + `Content-Type: application/json`
- 重试策略：最多 3 次。429 优先用 `Retry-After` 秒数，否则指数退避 1s/2s/4s。网络错误可重试；非可重试错误（auth/quota/context_overflow）直接返回
- HTTP Client：通过 `ProviderOpenAI.HTTPClient` 注入，nil 时回退 `http.DefaultClient`
- 超时：由 helm 通过 `context.WithTimeout` 覆盖整个 LLM 调用 + SSE 消费周期

### parseSSE — SSE 解析 + 安全退出

`context.AfterFunc(ctx, body.Close)` 注册回调——ctx 取消时关闭 body 中断阻塞的 scanner。回调在取消发出方 goroutine 同步执行，无需额外 goroutine 等待。`defer stop()` 确保 scanner 先结束时摘除回调，避免双 Close。

### streamState — 状态转换规则

```
reasoning → text 或 reasoning → tool_calls 时，自动先关 reasoning block（EventThinkingDone）

flush 时（[DONE] / EOF）：
  textActive → EventTextDone（附带本次 API token usage）
  thinkingActive → EventThinkingDone
  toolCalls → json.Unmarshal arguments → EventToolUseDelta（完整 Parameters）
```

### 错误分类

| HTTP 状态码 | APIError.Code | Retryable |
|------------|---------------|-----------|
| 200 | — | — |
| 401 | auth | false |
| 402 / 429 (billing) | quota | false |
| 429 (rate_limit) | rate_limit | true |
| 400 (context length) | context_overflow | false |
| 5xx | server_error | true |
| 网络错误 | network | true |

### NewProvider 路由规则

`api == "openai-completions"` 或空 → `ProviderOpenAI`。baseURL 默认 `https://api.openai.com/v1/chat/completions`，`provider != "openai"` 时从 `Options["base_url"]` 读取自定义地址。

### 依赖边界

sail 只依赖 `knot`（类型、配置、事件定义），不依赖 helm/deck/brig/voyage。`Provider` 接口由 helm 消费。
