# sail

## 概述

sail 是 Argo 的 LLM 接入层——加载配置、路由 provider、发起流式 API 调用，通过 `<-chan knot.Event` 向 helm 推送统一格式的流式事件。sail 内部用一份 OpenAI Chat Completions SSE 解析实现覆盖 ChatGPT、DeepSeek、Kimi、MiniMax、豆包五个厂家，区别仅在 baseURL。

## 核心概念

| 概念 | 说明 |
|------|------|
| `Chat()` | 流式调用入口：查配置 → 读 API Key → 路由 adapter → 返回事件 channel |
| `OpenAIChat` / `CompatibleChat` | 两个薄入口函数，分别内置 `api.openai.com` 和用户配置的 baseURL |
| `doChat` | HTTP POST + 启动 goroutine 解析 SSE，立即返回 channel |
| `parseSSE` | 逐行解析 SSE 流，`context.AfterFunc` 响应 ctx 取消，零 goroutine 泄漏 |
| `streamState` | 流式事件状态机，跟踪 text/thinking/tool_use 三块的首个 chunk 以决定 push start 还是 delta |
| `streamState.handleChunk` | 解析 chunk JSON → 根据当前状态判断 event type → 返回事件列表 |
| `streamState.flush` | 流结束时关闭所有活跃 block：textDone → thinkingDone → toolUseDone |
| `knot.Event` | 流式事件：textStart/Delta/Done、thinkingStart/Delta/Done、toolUseStart/Delta/Done、error |

## 行为合约

### Chat——流式 LLM 调用入口

对外暴露的单入口函数。定位模型所属 provider → 从环境变量读 API Key → 按 provider 类型路由到 `OpenAIChat` 或 `CompatibleChat`。

- **输入**：`ctx context.Context`、`modelName string`、`req knot.ChatRequest`
- **输出**：`<-chan knot.Event` 事件流 + `error`（仅配置/路由阶段失败时非 nil）
- **路由规则**：`provider == "openai"` → `OpenAIChat`；其他 → `CompatibleChat`（baseURL 取自 `Options["base_url"]`，为空时回退 `https://api.openai.com`）
- **API Key**：优先读 `ProviderConfig.Env[0]` 对应的环境变量，为空则报错

### doChat——HTTP 层

构造 OpenAI Chat Completions 请求体 → POST 到 `{baseURL}/v1/chat/completions` → 非 200 返回单事件 error channel → 200 启动 goroutine 进入 `parseSSE`。

- **HTTP 头**：`Authorization: Bearer {apiKey}` + `Content-Type: application/json`
- **超时**：通过 `ctx` 注入 `http.NewRequestWithContext`，不设整轮截止时间
- **非 200**：立即 `close(out)`，返回只含一个 `EventError` 的 channel

### parseSSE——SSE 解析 + 安全退出

`bufio.Scanner` 逐行读取 `text/event-stream` 响应体。`context.AfterFunc(ctx, body.Close)` 注册 ctx 取消回调——ctx 取消时关闭 body 中断阻塞的 scanner，回调在取消发出方 goroutine 同步执行，无需额外 goroutine 等待。`defer stop()` 确保 scanner 先结束时摘除回调，避免双 Close 和泄漏。

scanner.Err 和 ctx.Err 各自独立判断，有错即推 EventError。

### streamState——流式事件状态机

增量解析 OpenAI SSE chunk，根据当前状态决定每个 chunk 应推哪种 EventType：

```
chunk 到达
├─ content 非空
│   ├─ textActive? 否 → EventTextStart，置 textActive=true
│   └─ textActive? 是 → EventTextDelta
│   └─ 此前 thinkingActive? → 先推 EventThinkingDone
│
├─ reasoning 非空（兼容 reasoning_content / reasoning_text）
│   ├─ thinkingActive? 否 → EventThinkingStart，置 thinkingActive=true
│   └─ thinkingActive? 是 → EventThinkingDelta
│
├─ tool_calls 非空
│   ├─ thinkingActive? → 先推 EventThinkingDone
│   ├─ toolCall[index] 未记录? → EventToolUseStart，记录 index→id
│   └─ toolCall[index] 已记录? → EventToolUseDelta
│
└─ flush（[DONE] 或 EOF）
    ├─ textActive → EventTextDone
    ├─ thinkingActive → EventThinkingDone
    └─ 遍历 toolCalls → EventToolUseDone
```

**状态转换**：reasoning → text 或 reasoning → tool_calls 时，自动先关 reasoning block（EventThinkingDone）。参考 kilocode 的 `transform()` 方法中 `isActiveReasoning` 到 `isActiveText` 的自动切换模式。
