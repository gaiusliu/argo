# Architecture Evolution: Layering, Domain Model, and Event-Driven

> 本文档记录 Argo 项目在分层、领域模型和事件驱动三个方向上的长期演进思路。
> 不包含当下必须实施的改造，而是作为架构演进的参考蓝图。

## 背景

Argo 当前处于 MVP 阶段，核心流程已经跑通：

- CLI 通过 HTTP + SSE 与 server 通信
- `helm` 状态机驱动 LLM 调用 → 工具执行 → 用户审批的循环
- `voyage` / `vault` 负责会话持久化
- `deck` / `brig` / `sail` 分别负责工具、门控、模型调用

当前架构的职责切分已经比较清晰，但存在以下可演进空间：

1. 层与层之间通过 concrete 实现直接耦合
2. 领域概念散落在 `knot.Message`、`HelmState`、`ToolUse` 等结构中
3. `helm` 通过 channel 直接与 server 耦合，Ask 流程需要断连重建

本文档提出一个渐进式演进方向：

```
依赖注入 → 分层清晰 → 领域模型显式化 → 事件总线解耦
```

## 1. 分层架构

### 1.1 当前分层

```
┌─────────────────────────────────────┐
│  Transport   cli / server           │
├─────────────────────────────────────┤
│  Orchestration  helm                │
├─────────────────────────────────────┤
│  Capability     deck / brig / sail  │
├─────────────────────────────────────┤
│  Persistence    voyage / vault      │
├─────────────────────────────────────┤
│  Shared Kernel  knot                │
└─────────────────────────────────────┘
```

当前问题：层与层之间不是通过接口调用，而是直接 concrete 耦合。

### 1.2 目标分层

```
┌─────────────────────────────────────────┐
│  Transport Layer                        │
│  cli / server / future desktop plugin   │
│  职责：协议适配、请求解码、响应编码、SSE  │
├─────────────────────────────────────────┤
│  Application Layer                      │
│  AgentService                           │
│  职责：用例编排、事务边界、checkpoint 时机 │
├─────────────────────────────────────────┤
│  Domain Layer                           │
│  Session / Thread / Tool / Guard        │
│  职责：核心业务规则、状态机、审批逻辑      │
├─────────────────────────────────────────┤
│  Infrastructure Layer                   │
│  FileVault / OpenAIProvider / HTTPClient│
│  职责：具体技术实现                       │
└─────────────────────────────────────────┘
```

### 1.3 分层规则

| 规则 | 含义 |
|------|------|
| 上层可调下层 | Server 可以调用 AgentService |
| 下层不知道上层 | Domain 不依赖 HTTP / SSE |
| 同层不直接依赖 | Domain 不直接 new Infrastructure |
| 依赖通过接口 | `model.Provider` 而不是 `*sail.SailOpenAI` |

### 1.4 对 Argo 的适用性

当前 Argo 规模较小，全面分层会带来一定 overhead。但 **依赖注入改造天然是分层的基础**。完成 DI 后，分层会自然出现，不需要额外大动作。

## 2. 领域模型

### 2.1 当前问题

领域概念散落在多个结构中：

- 对话历史：`[]knot.Message` 由 `helm` 直接操作
- 会话状态：`HelmState` 混合了控制流字段和持久化游标（`Saved`）
- 工具调用：`knot.ToolUse` 同时表示 LLM 请求、执行结果、审批结果
- 审批：`brig.Gate` 藏在 `voyage.Session` 内部，规则写死

这些设计在 MVP 阶段够用，但会让后续扩展变困难。

### 2.2 目标领域模型

建议将核心领域对象显式化：

```
┌─────────────────────────────────────────┐
│              Session                    │
│  - ID                                   │
│  - WorkingDir                           │
│  - Thread（对话线程）                    │
│  - Guard（审批策略）                     │
│  - CheckpointPolicy（持久化策略）        │
├─────────────────────────────────────────┤
│              Thread                     │
│  - Messages []Message                   │
│  - AppendUser / AppendAssistant / Tool  │
│  - ToChatRequest()                      │
├─────────────────────────────────────────┤
│              Message                    │
│  - Role / Content / ToolCalls           │
├─────────────────────────────────────────┤
│              ToolUse                    │
│  - ID / Name / Parameters               │
│  - Verdict（审批结果）                   │
│  - Result（执行结果）                    │
├─────────────────────────────────────────┤
│              Guard                      │
│  - Evaluate(ToolUse) Verdict            │
│  - Approve(ToolUse)                     │
└─────────────────────────────────────────┘
```

### 2.3 关键改进点

#### Tale 取代裸 `[]Message`（高优先级 — 配合 compact / token 计数）

当前 `HelmState` 直接持有 `[]Message` 和 `Saved` 游标。未来引入 `reef.Compact()`（消息裁剪）和 token 计数后，会暴露一个一致性问题：

```
Compact 前: Messages = [msg0, msg1, msg2, msg3], Saved = 2
Compact 删掉 msg0, msg1 →
Compact 后: Messages = [msg2, msg3],       Saved = ???  // 游标失效
```

`Messages` 和 `Saved` 必须同时修改，但当前它们分散在 `PhaseRunnerModel`、`checkpoint` 等多处操作，一致性无法保证。

**方案：** 将消息列表和持久化游标封装为 `Tale` 类型，放入 `HelmState`：

```go
type HelmState struct {
    Phase    int
    Model    string
    ToolUses []knot.ToolUse
    Tale     *Tale             // 替代 Messages + Saved
}
```

`Tale` 核心方法：

```go
type Tale struct {
    messages     []Message
    systemPrompt string
    saved        int            // 持久化游标，外部不可见
}

// 追加消息
func (t *Tale) AppendUser(content string)
func (t *Tale) AppendAssistant(content string, toolCalls []ToolCall)
func (t *Tale) AppendTool(callID string, content string)

// 构造 LLM 请求（自动注入 system prompt、触发 compact、裁剪旧工具结果）
func (t *Tale) BuildRequest(tools []Tool, limit int) ChatRequest

// Checkpoint 用：返回未持久化的差量
func (t *Tale) Unsaved() []Message
func (t *Tale) MarkSaved()

// 内部：token 预算驱动
func (t *Tale) compact(limit int)
```

生命周期：

```
创建 Session：vault.Load() → messages → Tale{messages, saved=len(messages)}
运行中：     tale.AppendXxx() → tale.BuildRequest()（自动 compact）
Checkpoint： delta := tale.Unsaved() → vault.Append(delta) → tale.MarkSaved()
Compact 时： tale.compact(limit) 内部同时调整 messages 和 saved，外部无感
```

与 `reach` / token 计数的协作：

- `BuildRequest()` 内部调用 `reef.Compact()` 裁剪消息
- 裁剪时对旧工具结果做摘要化（超过 N 轮的工具结果截断），与 `deck.truncation`（安全上限）分层处理
- Token 计数结果缓存在 Tale 内部，消息变化时自动失效

#### ToolUse 拆分职责

当前 `ToolUse` 同时携带：

- LLM 发起的调用请求
- 门控审批结果
- 执行结果

未来可考虑拆分为：

```go
type ToolCall struct {        // LLM 发起
    ID, Name, Parameters
}

type ToolExecution struct {   // 执行上下文
    Call    ToolCall
    Verdict Verdict
    Result  ToolResult
}
```

#### Guard 作为一等领域对象

当前 `Guard` 被 `voyage.Session` 持有。未来可让 `Guard` 独立存在，由 `helm` 在执行阶段显式调用：

```go
verdict := guard.Evaluate(toolCall)
```

### 2.4 对 Argo 的适用性

完整领域模型重构成本较高，不建议在 MVP 阶段全面推进。但可以在遇到以下情况时逐步引入：

- `HelmState` 字段继续膨胀
- 需要支持多会话并发编辑
- 需要支持不同模式的对话线程（如 plan / act 分离）

## 3. 事件驱动

### 3.1 当前问题

helm 通过 `chan knot.Event` 直接把事件推给 server：

```
helm.Run() ──chan Event──► server ──SSE──► cli
```

问题：

1. `helm` 必须与 server 绑定在同一个进程内，无法独立演进
2. Ask 流程需要断连重建 SSE，客户端逻辑复杂
3. 未来若支持多客户端订阅同一 session，channel 模式难以扩展

### 3.2 目标事件驱动架构

引入内部事件总线：

```
┌─────────┐    ┌─────────────┐    ┌─────────┐
│  helm   │───►│  Event Bus  │◄───│  cli    │
└─────────┘    └──────┬──────┘    └─────────┘
                      │
               ┌──────┴──────┐
               │  SSE Adapter │
               └─────────────┘
```

### 3.3 事件类型设计

```go
type Event interface {
    eventMarker()
}

type TextDeltaEvent struct { Delta string }
type ThinkingDeltaEvent struct { Delta string }
type ToolUseStartEvent struct { ToolUse ToolUse }
type ToolUseDoneEvent struct { ToolUse ToolUse }
type AskUserEvent struct { ToolUses []ToolUse }
type DoneEvent struct { Usage TokenUsage }
type ErrorEvent struct { Err error }
```

### 3.4 Ask 流程改进

当前 Ask 流程：

```
helm ──EventAsk──► server ──SSE──► cli
helm goroutine 退出
cli 重新 POST /session/prompt
helm 重新加载 state，继续执行
```

事件驱动后：

```
helm ──AskUserEvent──► Event Bus ──► cli
cli ──AskReplyEvent──► Event Bus ──► helm
helm 继续执行，无需断连
```

### 3.5 事件总线的实现选择

| 方案 | 适用场景 | 复杂度 |
|------|----------|--------|
| 内存 channel + 订阅表 | 单机、进程内 | 低 |
| Redis Pub/Sub | 多实例 server | 中 |
| NATS / Kafka | 大规模分布式 | 高 |

Argo 当前适合 **内存事件总线**，后续若需要多实例再迁移。

### 3.6 对 Argo 的适用性

事件总线能解决 Ask 断连问题，但会引入额外复杂度。建议在以下条件触发时再考虑：

- Ask 断连导致状态同步 bug
- 需要支持 WebSocket 或桌面端接入
- 需要多个组件同时响应同一事件

## 4. 渐进演进路线

建议分三个阶段推进，避免一次性大重构。

### 第一阶段：依赖注入（当前已共识）

范围：

- `helm.PhaseRunnerModel` 注入 `model.Provider` + `tool.Registry`
- `helm.PhaseRunnerExecTools` 注入 `tool.Registry`
- `voyage.Session` 注入 `vault.Vault` + `brig.Guard`
- 移除 `knot.GetConfig()` 全局单例
- `sail` 注入 `*http.Client`
- `brig.FileGuard` 注入 `RuleLoader`

收益：

- 组件可独立测试
- 替换实现无需改被调用方
- 为分层打好基础

### 第二阶段：轻量级 Application Layer

在完成 DI 后，将 `server/prompt.go` 中的业务流抽成 `AgentService`：

```go
type AgentService interface {
    Run(ctx context.Context, sessionID, message string) (<-chan knot.Event, error)
    ReplyAsk(ctx context.Context, sessionID string, verdicts []ToolUse) (<-chan knot.Event, error)
}
```

此时 server handler 只剩：

```go
events, err := s.agent.Run(ctx, req.SessionID, req.Message)
streamSSE(w, events)
```

收益：

- HTTP 协议与业务流程解耦
- server handler 可独立测试
- 为事件总线做准备

### 第三阶段：事件总线 + 领域模型显式化

当项目规模或需求触发时：

1. 引入内存事件总线
2. 将 `Thread`、`Guard`、`CheckpointPolicy` 等领域对象显式化
3. 将 Ask 流程改为事件驱动，不再断连重建
4. 考虑多客户端订阅同一 session

收益：

- 支持更灵活的客户端接入
- 核心流程不再依赖 HTTP 协议细节
- 更容易扩展和测试

## 5. 何时不该推进

以下情况建议暂停架构演进：

- 核心功能尚未稳定，频繁修改业务逻辑
- 团队对当前代码还没有足够理解
- 没有多客户端或分布式的实际需求
- 测试覆盖不足，重构风险高

## 6. 总结

| 方向 | 核心价值 | 当前优先级 | 触发条件 |
|------|----------|------------|----------|
| 依赖注入 | 解耦、可测试 | 高 | 已达成共识，立即实施 |
| Tale 封装 | Compact/token 计数的一致性安全 | 高 | 实施 compact / token 计数时同步推进 |
| 分层清晰 | 职责边界明确 | 中 | DI 完成后自然形成 |
| ToolUse 拆分 | 职责分离 | 中 | ToolUse 字段继续膨胀时 |
| 事件驱动 | 流程解耦、多客户端 | 低 | Ask 体验问题或多客户端需求 |

一句话：

> **先做依赖注入建立分层，再视情况补领域模型和事件总线，不必一步到位。**
