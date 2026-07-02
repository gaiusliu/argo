# sail 技术决策记录

---

## DEC-S01：三 Provider Adapter 覆盖主流市场

**日期**：2026-06-05
**状态**：已决定

**决策**：sail 提供三个 adapter——`anthropic`（Anthropic Messages API）、`openai`（OpenAI Chat Completions API）、`openai-compatible`（任何实现 `/v1/chat/completions` 的 API）。

**原因**：三个 adapter 覆盖 95%+ 的 LLM 市场。openai-compatible adapter 通过 `base_url` 接入 DeepSeek、通义千问、GLM、Kimi、MiniMax、Ollama 等所有兼容 API。Gemini 原生协议后续按需加。

**拒绝**：OpenCode 的 4 轴路由（Protocol × Endpoint × Auth × Framing）——抽象层次太高，MVP 不值得。每一家都实现同一个 `Chat()` + `Window()` 接口即可，不需要组合式的路由引擎。

---

## DEC-S02：HTTP 层面超时，不设整轮截止时间

**日期**：2026-06-05
**状态**：已决定

**决策**：sail 只在 HTTP 层面设超时（通过 `context.Context` 传导），不设整轮对话的截止时间。

**原因**：

1. 所有调研项目都不设整轮截止——复杂任务可能一轮跑很久，中途砍掉会丢失进度
2. 用户权限确认可能长时间阻塞——Agent 不该替用户做权限决定
3. LLM API 层面的超时只解决"服务端多久不回复算挂"（network staleness），不是"这轮干了多久"

**实现**：`Chat(ctx, modelName, req)` 的第一个参数是 `context.Context`，由 Helm 设置 HTTP 超时。sail 内部 goroutine 监听 `ctx.Done()` 并在 ctx 取消时退出，不泄漏 goroutine。

---

## DEC-S03：错误分类

**日期**：2026-06-05
**状态**：已决定

**决策**：sail 将 API 错误分成 6 类，通过 `APIError.Code` 标识。分类决定 helm 的后续动作。

| Code | HTTP | Retryable | 动作 |
|------|:---:|:---:|------|
| `"auth"` | 401/403 | 否 | 通知用户，等待回应 |
| `"quota"` | 402/429（billing） | 否 | 通知用户，等待回应 |
| `"rate_limit"` | 429 | 是 | 等 retry-after + 重试（≤2 次） |
| `"server_error"` | 5xx | 是 | 等 1s + 重试（≤2 次） |
| `"context_overflow"` | 413 | 否 | 传播给 helm → reef 裁剪 |
| `"network"` | DNS/连接拒绝/TLS/EOF | 是 | 等 1s + 重试（≤2 次） |

**参考**：Claude-Code 的 `classifyFailoverReason()` 和 Hermes Agent 的 402 缓存。

**拒绝**：笼统返回 "API call failed"——helm 无法做出正确的重试决策，可能把 401 当网络瞬断重试 3 次。

---

## DEC-S04：三级模型能力元数据

**日期**：2026-06-05
**状态**：已决定

**决策**：模型能力元数据（context window、max output、是否支持 tools/images）通过三层优先级获取：用户手动覆盖 > OpenRouter API 自动更新 > 内置默认值。

**原因**：

1. 内置默认值覆盖 Anthropic/OpenAI/DeepSeek 最常用的 5-6 个模型，足够大部分用户
2. OpenRouter 的免费 `/models` 端点覆盖 200+ 模型，新模型上市无需 Argo 发版
3. 用户可以在 `models.json` 中覆盖任意字段

**拒绝**：
- 纯 API 自描述（Anthropic/OpenAI 的 /models 端点）——额外网络调用，各家格式不统一
- 纯用户配置声明——用户需要知道模型的 context window 等参数

---

## DEC-S05：API Key 仅从环境变量 + 配置文件

**日期**：2026-06-05
**状态**：已决定

**决策**：API Key 从环境变量读取（优先）或 `models.json` 的 `api_key` 字段（支持 `${VAR}` 语法）。不做多 Key 轮转、不做加密存储、不做 OAuth。

**原因**：MVP 的 Key 管理只需覆盖最基础的使用场景。多 Key 轮转（OpenClaw 的 auth profiles）、加密存储（Claude-Code 的 Keychain）、OAuth（OpenCode 的 Auth pipeline）是成熟产品的优化方向。

**拒绝**：多 Key 轮转（OpenClaw）——引入冷却探测、fallback 链、profile 管理等复杂度，MVP 阶段 Key 只有一个。

---

## DEC-S06：首次启动自动生成默认配置

**日期**：2026-06-05
**状态**：已决定

**决策**：Argo 首次启动时，sail 扫描环境变量中已知的 API Key，自动生成 `~/.argo/models.json`。用户不需要手动执行 init 命令。`argo init` 仅用于交互式引导用户配置 LLM 接入（选 provider、填 API Key）。

**参考**：Trae Agent 和 CodeBuddy 的环境变量引用语法 `${VAR}`。

**拒绝**：`argo init` 作为必须执行的初始化步骤——增加用户的使用门槛。
