# sail 行为规约

## 概述

sail 负责 LLM API 的配置、接入和调用。它加载 `~/.argo/models.json` 管理多模型配置，通过三个 provider adapter 屏蔽不同 API 的差异，向 helm 暴露统一的 `LLM` 接口（`Chat()` + `Window()`）。

---

## 需求

### 需求：多模型配置

sail SHALL 从 `~/.argo/models.json` 加载模型配置。每个模型条目包含 `provider`（API 类型）、`model`（模型名）、`api_key`（凭证）。`api_key` 支持 `${ENV_VAR}` 语法引用环境变量。

#### 场景：最简配置

- 给定：`models.json` 中 `claude` 条目仅写了 `"claude-sonnet-4-20250514"`（字符串形式）
- 当：sail 加载配置
- 则：sail 自动推断 `provider: "anthropic"`、从 `ANTHROPIC_API_KEY` 环境变量读取 api_key

#### 场景：完整配置（覆盖非标准 base_url）

- 给定：`models.json` 中 deepseek 条目以 object 形式写了 `provider`、`model`、`base_url`、`api_key`
- 当：sail 加载配置
- 则：sail 使用用户提供的完整配置，不做自动推断

#### 场景：首次启动无配置文件

- 给定：`~/.argo/models.json` 不存在
- 当：Argo 首次启动
- 则：sail 扫描环境变量中的已知 API Key，生成默认配置文件

### 需求：三 Provider Adapter

sail SHALL 提供三个 adapter：`anthropic`（Anthropic Messages API）、`openai`（OpenAI Chat Completions API）、`openai-compatible`（任何实现 `/v1/chat/completions` 的 API）。

#### 场景：openai-compatible 覆盖非标准 API

- 给定：用户配置了中国厂商的模型，`provider: "openai-compatible"`，`base_url: "https://api.deepseek.com"`
- 当：helm 调用 `Chat()`
- 则：sail 通过 openai-compatible adapter 发送请求到指定 base_url

### 需求：运行时模型切换

sail SHALL 支持在会话中切换模型。`Chat(modelName, req)` 的第一个参数指定本次调用使用的模型名称。

#### 场景：会话中切换到备用模型

- 给定：helm 当前使用 `claude`，需要切换到 `deepseek`
- 当：helm 调用 `sail.Chat("deepseek", req)`
- 则：sail 使用 deepseek 的配置发送 LLM 请求

### 需求：流式 Chat

sail SHALL 通过 channel 返回流式事件。事件类型包括 `text_delta`（逐 token 文本）、`tool_use`（工具调用）、`done`（完成 + TokenUsage）。

#### 场景：流式推送 LLM 回复

- 给定：LLM 正在生成回复
- 当：LLM 逐 token 返回
- 则：sail 通过 `<-chan Event` 推送每个 token，最后推送 `done` 事件

### 需求：Context Window 查询

sail SHALL 提供 `Window()` 方法，返回当前模型的 context window token 上限。

#### 场景：reef 需要知道压缩阈值

- 给定：当前模型是 claude-sonnet-4，context window 200K
- 当：helm 调用 `sail.Window()`
- 则：返回 `TokenLimit{MaxTokens: 200000}`

### 需求：Token 用量追踪

sail SHALL 从 API 响应中提取 `usage` 字段，归一化为 `TokenUsage` 结构，塞入 `done` 事件。

#### 场景：追踪每轮消耗

- 给定：LLM 返回了 `usage: {input_tokens: 500, output_tokens: 200}`
- 当：sail 推送 `done` 事件
- 则：`done.TokenUsage` 包含 input/output token 数

### 需求：错误分类

sail SHALL 将 API 错误分类为指定类型。分类决定 helm 的后续动作——不给重试不该重试的错误。

#### 场景：API Key 无效

- 给定：API 返回 401
- 当：sail 出错
- 则：错误分类为 `"auth"`，`Retryable: false`。helm 通知用户并等待回应

#### 场景：速率限制

- 给定：API 返回 429（rate limit）
- 当：sail 出错
- 则：错误分类为 `"rate_limit"`，`Retryable: true`

### 需求：模型能力元数据

sail SHALL 维护当前模型的能力元数据，通过 `Window()` 返回 context window 上限。模型表有三层级来源：内置默认值（硬编码 5-6 个主流模型）→ OpenRouter API 自动更新 → 用户手动覆盖（最高优先级）。

---

## 边界情况

- HTTP 层面超时（120s stream staleness）：由 `context.Context` 超时控制。sail 不设整轮截止时间
- OpenRouter 更新失败：使用上次缓存的模型表，记录诊断日志，不影响正常运行
- 配置文件格式错误：返回明确错误，不使用任何默认值（防止静默切换到非预期模型）
- 用户未配置模型表且未配置任何 API Key 环境变量：提示用户至少配置一个模型的 API Key
