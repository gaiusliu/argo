# 003-sail-mvp

**日期**：2026-07-04 ~
**状态**：🚧 进行中
**涉及模块**：sail、knot

## 目标

将 sail 从桩实现推进为完整 MVP，接入真实 LLM API。支持两个 provider adapter，覆盖 ChatGPT、DeepSeek、MiniMax、Kimi、豆包五个厂家。使 helm 的 RunLoop 能够通过 `<-chan knot.Event` 消费真实的模型流式响应。

## 关键流程

### Chat 流程

```
helm.Chat(ctx, modelName, req)
│
├─ 1. 查找模型配置
│   models.json[modelName] → ModelConfig
│   ├─ 命中 → 解析 api_key 中的 ${ENV_VAR} → 得到真实 Key
│   └─ 未命中 → 返回错误，不发起请求
│
├─ 2. 选择 Adapter（按 provider 字段分发）
│   ├─ "openai"              → openaiAdapter      (ChatGPT)
│   └─ "openai-compatible"   → compatibleAdapter   (DeepSeek / MiniMax / Kimi / 豆包)
│
├─ 3. Adapter 内部
│   ├─ 将 knot.ChatRequest 转换为各 API 原生格式
│   ├─ HTTP POST 流式请求 ───────────────► LLM API
│   │                                       │
│   │  ◄── SSE/流式响应 ─────────────────────┘
│   │
│   ├─ 逐 token 推送 knot.Event(textDelta)
│   ├─ 遇 tool_calls 推送 knot.Event(toolUseStart/Delta/Done)
│   ├─ 成功结束 → 提取 usage，推送 knot.Event(done)
│   └─ 失败 → classifyError() → 决定重试或返回错误
│
└─ 4. 返回 <-chan knot.Event 给 helm
```

### Window 流程

`Window()` 返回当前模型一次能处理的最大 token 数（context window 上限）。它被 helm 在每轮 LLM 调用前传给 reef——reef 靠这个阈值判断消息历史是否逼近上限、是否需要触发压缩。如果 Window 返回不准确的值（比如桩里硬编码的 128K），reef 要么过早压缩（浪费上下文），要么压缩太晚（API 413 拒绝请求）。

取值来源按优先级排列——用户显式配置 > 内置厂家默认表 > 保守回退值：

```
sail.Window(modelName)
│
├─ 1. 查用户覆盖 → models.json 中的 max_input_tokens 字段
├─ 2. 查内置默认表 → 硬编码 5 个厂家的 context window
└─ 3. 回退保守值 → 128000
```

## 功能列表

| # | 功能 | MVP 为何要做 |
|---|------|-------------|
| 1 | **核心类型定义** | APIError、ModelCapability 是所有功能的基础类型，不先定下来 adapter 和 Client 无法开工 |
| 2 | **配置加载完善** | 在现有 LoadConfig 基础上补 GenerateDefaultConfig——扫描环境变量自动生成 `models.json`，否则每个新用户都要手写配置文件 |
| 3 | **Provider Adapter ×2** | 核心价值所在。openai adapter 覆盖 ChatGPT，openai-compatible adapter 通过 `base_url` 覆盖 DeepSeek、MiniMax、Kimi、豆包四个国产厂商 |
| 4 | **Client 实现** | `Chat()` + `Window()` 是 helm 直接依赖的唯一入口，替换当前桩实现，使整个 Agent 循环跑通 |
| 5 | **错误分类 + 重试** | helm 需要知道哪些错误可以自动重试（429/5xx/网络瞬断）、哪些必须通知用户等待回应（401/402）。不分类会导致把 API Key 无效当成网络瞬断重试 |
| 6 | **模型元数据内置表** | `Window()` 需要返回真实 context window 供 reef 判断何时压缩。MVP 内置 5 个厂家常用模型的值 |

### 支持的模型厂家

| 厂家 | Adapter | 接入方式 |
|------|---------|---------|
| ChatGPT | openai | OpenAI Chat Completions API |
| DeepSeek | openai-compatible | `base_url` → `https://api.deepseek.com` |
| MiniMax | openai-compatible | `base_url` → MiniMax 端点 |
| Kimi | openai-compatible | `base_url` → Moonshot 端点 |
| 豆包 | openai-compatible | `base_url` → 火山引擎端点 |

## 公共设计约束

- 两个 adapter 统一内部接口，对外只暴露 `<-chan knot.Event`，helm 不感知 adapter 差异
- API Key 的 `${ENV_VAR}` 解析在 Client 层统一完成，adapter 只收明文字符串
- 重试在 `doChat` adapter 层：`classifyError` 返回 `Retryable` 标记由 `doChat` 内部消费，3 次指数退避。5xx 不触发换模型——换模型需上层征求用户同意
- 流式事件类型必须对齐 `knot.Event` 已有的 12 种类型
- 不设整轮截止时间，HTTP 超时通过 `context.Context` 传导（DEC-S02）

## 错误场景

| 场景 | 分类 | MVP 为何考虑 |
|------|------|-------------|
| API Key 无效 (401/403) | `auth` | 最常见的新手配置错误，必须有明确提示 |
| 余额不足 (402/429 billing) | `quota` | 用户第一个遇到的钱相关问题，需告知而非静默失败 |
| 速率限制 (429) | `rate_limit` | 免费 API 高频调用必然触发，应自动等 retry-after 重试 |
| 服务端错误 (5xx) | `server_error` | 厂商临时故障，应自动重试而非直接报错给用户 |
| Context 溢出 (413) | `context_overflow` | 长对话必然遇到，需传播给 reef 触发压缩 |
| 网络错误 (DNS/TLS/EOF) | `network` | 断网或代理故障，应自动重试 |

## 整体开发状态

| 功能 | 状态 | 子文档 | 关联测试 |
|------|------|--------|---------|
| 核心类型定义 | ✅ | — | — |
| 配置加载完善 | ✅ | — | — |
| Provider Adapter ×2 | ✅ | — | — |
| Client 实现 | ✅ | — | — |
| 错误分类 + 重试 | ✅ | errors.go | — |
| 模型元数据内置表 | ✅ | metadata.go | — |

## 关键决策

- [DEC-S01~S06](../../modules/sail/decisions.md)：Adapter 拆分、超时策略、错误分类、元数据来源、Key 管理、自动配置生成
