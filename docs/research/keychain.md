# LLM API 管理（keychain）设计调研

> 调研对象：Claude-Code、OpenCode、Hermes Agent、CrewAI、OpenClaw、Nanoclaw、pi

---

## 1. 全局共识

七个项目在 LLM API 管理上的共识非常强：

- **统一接口屏蔽 Provider 差异**：所有项目都在 LLM API 和 Agent 循环之间加了一层抽象
- **API Key 通过环境变量**：几乎全用环境变量（`ANTHROPIC_API_KEY`、`OPENAI_API_KEY` 等）
- **多 Provider 支持**：Anthropic + OpenAI + Google 是最低配置
- **Transport/Adapter 模式**：每个 API 协议有自己的适配器，Agent 循环不感知协议差异

---

## 2. Claude-Code：单 Provider 深度集成 + Fallback 链

**核心文件**：`src/services/api/claude.ts`、`client.ts`、`src/utils/model/`

### 支持的部署方式

```
getAnthropicClient({ apiKey, model }):
  ├── CLAUDE_CODE_USE_BEDROCK   → AnthropicBedrock SDK
  ├── CLAUDE_CODE_USE_VERTEX    → AnthropicVertex SDK
  ├── CLAUDE_CODE_USE_FOUNDRY   → AnthropicFoundry SDK
  └── 默认                       → Anthropic SDK (第一方)
```

> **局限**：虽然是 Claude Code（名字暗示 Anthropic），但它只支持 Anthropic 模型的不同部署方式（第一方 API、AWS Bedrock、GCP Vertex、Azure Foundry），不支持 OpenAI/Gemini 等非 Anthropic 模型。

### 模型选择优先级

```
getMainLoopModel():
  1. 会话覆盖（/model 命令）— 最高优先级
  2. --model 启动标志
  3. ANTHROPIC_MODEL 环境变量
  4. 用户保存的模型设置
  5. 内置默认：
     - Max/Team Premium → Opus
     - Pro/Team Standard → Sonnet
     - 3P (Bedrock/Vertex/Foundry) → Sonnet
```

### Prompt Caching 策略

```
三层缓存：
├── Ephemeral caching ── 默认，会话期间缓存
├── 1h TTL caching ──── 符合条件的订阅者在对话数据集上
├── Global scope ─────── 跨会话缓存部分 system prompt
└── Cache editing ────── 通过 cache_edits API 主动删除缓存内容
```

### Fallback 链

```
主模型 529/rate_limit → 自动回退到备用模型
  ├── 捕获 FallbackTriggeredError
  ├── 切换到更便宜/更慢的模型
  └── 重试轮次

getRuntimeMainLoopModel():
  - Plan mode + 'opusplan' → 强制 Opus
  - Plan mode + 'haiku'    → 强制 Sonnet
```

### 设计思想

1. **单 Provider 深度集成 > 多 Provider 浅层覆盖**：只支持 Anthropic，但支持它的所有部署方式
2. **运行时模型覆盖**：Plan 模式可切换模型——关键任务用 Opus，普通任务用 Sonnet
3. **Cache editing 是独特能力**：在不需要某段缓存内容时主动删除，而非等 TTL 过期

---

## 3. OpenCode：4 轴分离路由 + 15+ Provider

**核心文件**：`packages/llm/src/route/client.ts`、`packages/llm/src/protocols/`、`providers/`

### 四轴路由模型

```
Route = { Protocol, Endpoint, Auth, Framing }

Protocol  ─ API 协议格式（Anthropic Messages / OpenAI Chat / Gemini ...）
Endpoint  ─ 请求 URL
Auth      ─ 认证策略（ApiKey / OAuth / AWS SDK / ADC ...）
Framing   ─ 流式解析方式（SSE / JSON Lines / NDJSON ...）
```

任意组合 = 新 Provider，无需新增代码。

### 路由编译

```typescript
function compile(request: LLMRequest):
  1. request.model.route → 查找路由
  2. route.body.from(request) → 构建 provider 原生 body
  3. route.body.schema → 验证
  4. transport → fetch with headers
  5. 流式: frames → decode → accumulate → LLMEvent[]
```

### 支持的 Provider

| Provider | Protocol |
|----------|----------|
| Anthropic | Anthropic Messages |
| OpenAI | OpenAI Responses / Chat |
| Google Gemini | Gemini |
| Amazon Bedrock | Bedrock Converse |
| Azure | Azure OpenAI |
| OpenRouter | OpenAI Compatible |
| xAI | OpenAI Responses |
| GitHub Copilot | OpenAI Compatible |
| DeepSeek/Mistral/Groq | OpenAI Compatible |
| OpenCode (自研) | OpenAI Compatible |

### API Key 管理

```typescript
// 多种认证方式
Auth.Service:
  ├── 环境变量（标准）
  ├── OAuth（OpenAI、OpenCode）
  ├── GitHub Copilot token（特殊流程）
  └── opencode.json 配置
// Key 不存储在会话中——请求时解析
```

### 设计思想

1. **4 轴组合 = 理论无限 Provider**：Protocol × Endpoint × Auth × Framing 的组合生成所有 API 变体
2. **路由注册表是全局单例**：所有路由在模块导入时自注册，重复 ID 是 bug
3. **Effect-TS 可测试**：`LLMClient` 是 Effect Service，测试时所有层可替换

---

## 4. Hermes Agent：ProviderProfile 声明式描述 + 7 级解析

**核心文件**：`agent/transports/`、`agent/auxiliary_client.py`、`providers/base.py`

### ProviderProfile 声明式

```python
@dataclass
class ProviderProfile:
    name: str                  # "deepseek", "openrouter", ...
    api_mode: str              # "chat_completions"
    aliases: tuple             # 别名
    env_vars: tuple            # API Key 环境变量名
    base_url: str              # API 端点
    auth_type: str             # api_key|oauth_device_code|copilot|aws_sdk
    fallback_models: tuple     # /model 选择器使用
    default_headers: dict
    # 钩子：
    prepare_messages()
    build_extra_body()
    build_api_kwargs_extras()
    fetch_models()
```

### API Mode 自动检测

```
AIAgent.__init__() 中：
  base_url/provider 名称 → 自动检测 api_mode:
    ├── 包含 "anthropic" → anthropic_messages
    ├── 包含 "bedrock"   → bedrock_converse
    ├── 包含 "codex"     → codex_responses
    └── 默认             → chat_completions
```

### 7 级 API Key 解析链

```
1. 显式 api_key + base_url → 直接创建客户端
2. ProviderProfile.env_vars → 环境变量
3. credential_pool → 凭证池
4. ~/.hermes/.env → 配置文件
5. ~/.claude.json → Claude Code 凭证
6. OAuth token → 令牌流
7. Fallback 链 → 备用 Provider
```

### 辅助模型路由

```
AuxiliaryClient.call_llm() 文本任务解析顺序：
  1. 主 provider + 主模型
  2. OpenRouter (OPENROUTER_API_KEY)
  3. Nous Portal (~/.hermes/auth.json)
  4. 自定义 endpoint (base_url + OPENAI_API_KEY)
  5. 原生 Anthropic
  6. 直接 API-key providers (z.ai, Kimi, MiniMax)
  7. None (失败)
```

### 设计思想

1. **35+ Provider 插件**：每个 `plugins/model-providers/<name>/` 就是一组 ProviderProfile + 钩子
2. **ProviderProfile 消除 20+ 布尔标志**：声明式描述替代命令式 kwargs 构建
3. **探测节流**：所有 Key 冷却时，每 provider 每 30s 最多 1 次探测，避免浪费
4. **辅助模型**用于压缩等低价任务——主模型处理核心对话，廉价模型处理摘要

---

## 5. CrewAI：两级路由工厂 + Native SDK 优先

**核心文件**：`llm.py`、`llms/base_llm.py`、`llms/providers/`

### 工厂路由

```python
LLM(model="gpt-4o")
  │
  ├── 显式 provider 参数? → 直接路由
  ├── 模型名包含 "/"?
  │   ├── 前缀是原生 provider (openai/anthropic/bedrock)?
  │   │   YES → 原生 SDK
  │   │   NO  → 回退 LiteLLM
  └── 无 "/" → 从常量推断 provider
```

### 支持的原生 SDK

OpenAI、Anthropic/Claude、Azure、Google/Gemini、Bedrock

### OpenAI 兼容 Provider

OpenRouter、DeepSeek、Ollama、Hosted vLLM、Cerebras、DashScope

> 通过 `OpenAICompatibleCompletion` 统一处理——因为它们都实现了 OpenAI 的 `/v1/chat/completions` 接口。

### 不同 Agent 不同模型

```python
researcher = Agent(llm=LLM(model="gpt-4o"))           # Agent 1: OpenAI
writer = Agent(llm=LLM(model="anthropic/claude-sonnet"))  # Agent 2: Anthropic
manager_llm = LLM(model="gpt-4o")                     # Manager: OpenAI
```

### 设计思想

1. **原生 SDK 优先，LiteLLM 兜底**：主要 Provider 用官方 SDK（最佳兼容性），长尾模型用 LiteLLM 覆盖
2. **每个 Agent 独立 LLM**：不同角色可用不同模型——研究员用 GPT-4o，写手用 Claude Sonnet
3. **分散式 Key 管理**：每个 LLM 实例携带自己的 Key 或依赖 SDK 环境变量发现——无中心化 Key 存储

---

## 6. OpenClaw：5 级模型解析 + V2 Harness 生命周期

**核心文件**：`src/agents/pi-embedded-runner/model.ts`、`model-fallback.ts`、`auth-profiles/`

### 5 级模型解析

```
resolveModelWithRegistry():
  1. resolveExplicitModelWithRegistry()    显式指定
  2. resolvePluginDynamicModelWithRegistry() 插件动态
  3. preferProviderRuntimeResolvedModel()   择优
  4. resolveConfiguredFallbackModel()       配置回退
  5. resolveBundledStaticCatalogModel()     硬编码兜底
```

### 模型规范化 7 步管线

```
模型对象经过：
1. 成本     → 填充默认成本
2. 输入     → 标准化输入格式
3. 插件兼容 → 应用插件兼容层
4. Provider → 匹配 Provider 能力
5. Transport → 确定 API 传输方式
6. 传输回退 → 主 transport 不可用时回退
7. 旧格式   → 适配旧格式字段
```

### V2 Harness 六阶段

```
prepare → start → send → [handleToolCall 循环] → resolveOutcome → cleanup
```

### Auth Profile 冷却 + 探测节流

```
resolveRunFailoverDecision():
  ├── "skip"           → 持久错误，跳过
  ├── "suspend_lanes"  → 通道挂起直到冷却
  └── "attempt"        → 主候选，每 30s 最多 1 次探测
```

### 设计思想

1. **错误分类是核心能力**：`classifyFailoverReason` 决定了 Rate Limit 换 Key、Auth Error 刷新 Token、Overload 退避——如果分类错误就是在浪费用户时间
2. **探测节流**：大面积 provider 故障时不浪费 API 调用
3. **5 级回退**确保在极端情况下 agent 仍然可用

---

## 7. Nanoclaw：Provider 注册表 + OneCLI 凭证代理

**核心文件**：`container/agent-runner/src/providers/factory.ts`、`provider-registry.ts`

### 自注册 Provider 注册表

```typescript
// providers/index.ts barrel
import './claude.js'   → registerProvider('claude', factory)
import './mock.js'     → registerProvider('mock', factory)
// codex + opencode 通过 skill 安装
```

### 当前支持的 Provider

```
claude    → @anthropic-ai/claude-agent-sdk (默认)
codex     → OpenAI Codex CLI + AppServer
opencode  → OpenCode (OpenRouter, OpenAI, Google, DeepSeek)
mock      → 测试用
```

### 凭证管理

**API Key 不存储在 container.json 或 Nanoclaw 数据库中。**

由 **OneCLI gateway** 管理：
- 安全的凭证存储
- 请求时 OAuth 令牌注入
- 审批门控（通过 OneCLI web UI 配置规则）

### Provider 接口

```typescript
interface AgentProvider {
  supportsNativeSlashCommands: boolean
  query(input: QueryInput): AgentQuery  // 返回支持 push() 的事件流
  isSessionInvalid(err: unknown): boolean
}

interface AgentQuery {
  push(message: string): void     // 向活跃查询注入后续消息
  events: AsyncIterable<ProviderEvent>
  abort(): void
}
```

### 设计思想

1. **Provider 抽象覆盖整个 SDK 生命周期**：`query()` 不暴露单次 API 调用——暴露的是支持同步消息推送的事件流
2. **`isSessionInvalid()` 检测过期的 continuation token**：检测到后从零开始，而不是无限重试
3. **OneCLI 作为凭证代理**：环境变量中没有永久的 API Key——在请求时由网关注入
4. **每个 agent-group 静态选择 provider**：不基于消息内容动态路由

---

## 8. pi：统一 Stream 抽象 + 懒加载 Provider

**核心文件**：`packages/ai/src/stream.ts`、`api-registry.ts`、`models.generated.ts`

### Provider 注册表

```typescript
registerBuiltInApiProviders():
  registerApiProvider({ api: "anthropic-messages", stream, streamSimple })
  registerApiProvider({ api: "openai-completions", stream, streamSimple })
  registerApiProvider({ api: "openai-responses", stream, streamSimple })
  registerApiProvider({ api: "google-generative-ai", ... })
  registerApiProvider({ api: "google-vertex", ... })
  registerApiProvider({ api: "bedrock-converse-stream", ... })
  registerApiProvider({ api: "azure-openai-responses", ... })
  registerApiProvider({ api: "openai-codex-responses", ... })
  registerApiProvider({ api: "mistral-conversations", ... })
```

### 统一接口

```typescript
streamSimple(model, context, options):
  resolveApiProvider(model.api) → provider.streamSimple(model, context, options)

// 每个 provider 懒加载：
createLazyStream(loadModule) → 第一次调用时 import → 转发事件流
// 错误编码为流中的 "error" 事件，从不抛出
```

### API Key 解析

```
1. 环境变量 (env-api-keys.ts)
2. OAuth 令牌 (packages/ai/src/utils/oauth/)
3. 动态运行时解析 (AgentHarness 的 getApiKeyAndHeaders 回调)
4. 隐式凭证 (AWS credential chain, Google ADC)
```

### 自动生成模型表

`models.generated.ts`（~22K 行）——由 `scripts/generate-models.ts` 从外部数据源自动生成，包含每个模型的 ID、名称、API 类型、Provider、成本、上下文窗口、最大 Token、思考级别映射等。

### 设计思想

1. **每个 Provider 实现同一个 StreamFunction 签名**：Agent 循环完全不知 provider 特定格式
2. **懒加载 Provider 模块**：直到第一次使用才导入——减少启动时间和内存
3. **错误永不抛出**：编码为 `AssistantMessageEvent` 流事件，保持"不抛出"合约
4. **models.generated.ts 是数据驱动的**：添加新模型只需更新生成脚本的数据源——而非手动维护

---

## 9. 横向对比总结

| 维度 | Claude-Code | OpenCode | Hermes Agent | CrewAI | OpenClaw | Nanoclaw | pi |
|------|------------|----------|-------------|--------|----------|----------|-----|
| **Provider 数量** | 1 (Anthropic 4 部署) | 15+ | 35+ | 6 原生 + 6+ 兼容 | 35+ | 3 (Claude/Codex/OpenCode) | 9 |
| **抽象模式** | Anthropic SDK 封装 | 4 轴路由模型 | ProviderProfile + Transport | 两级工厂 | 5 级解析 + V2 Harness | 自注册表 + OneCLI | 统一 Stream + 懒加载 |
| **API Key** | env var + OAuth | env var + OAuth + config | 7 级解析链 | env var + 显式参数 | env var + 冷却管理 | OneCLI 凭证代理 | env + OAuth + 动态回调 |
| **Fallback** | 模型回退链 | Effect.retry | ProviderProfile 级回退 | 无 | resolveRunFailoverDecision | 无（静态选择） | 无 |
| **模型选择** | 5 级优先级 | session 级 | Agent 级 | Agent 级 (不同 Agent 不同模型) | Agent 级 | agent-group 静态 | Harness 级 |
| **缓存策略** | 3 层 prompt cache | AI SDK 缓存 | KV cache 前缀匹配 | 无 | 无 | SDK 自管理 | 无 |
| **核心创新** | Cache editing | 4 轴组合路由 | 探测节流、ProviderProfile 声明式 | Native SDK 优先、LiteLLM 兜底 | 错误分类决策树 | OneCLI 无密钥设计、push() 注入 | 懒加载、永不抛出的流事件 |


## 10. 对 Argo 的启示

1. **Transport/Adapter 模式是最佳实践**：每个 API 协议有自己的适配器，Agent 循环只看到统一接口。OpenCode 的 4 轴路由是最干净的抽象
2. **Key 解析应有层级**：Hermes Agent 的 7 级解析（显式参数 → 环境变量 → 凭证池 → 配置文件 → 借用其他工具凭证 → OAuth → Fallback）是最完善的参考
3. **不要自己实现凭证存储**：Nanoclaw 的 OneCLI 代理模式说明密钥管理应该委托给专门的凭证管理器
4. **懒加载 Provider**是正确选择：pi 的 `createLazyStream` 确保不在启动时加载所有 Provider 模块
5. **错误应编码为流事件而非抛异常**：pi 的"不抛出"合约对 Agent 循环的健壮性至关重要
6. **MVP 阶段只需 3 个 Provider**：Anthropic + OpenAI + OpenAI-Compatible（覆盖 90% 模型）
7. **每个 Agent 实例可能用不同模型**：Argo 的 keychain 应该支持 Agent 级别的模型绑定
