# IDEA-001：架构长期演进 — 分层、领域模型、事件驱动

**父索引**：[../ideas.md](../ideas.md)
**日期**：2026-07-16

## 背景

Argo 当前处于 MVP 阶段，核心流程已跑通：CLI → HTTP/SSE → server → helm 状态机 → LLM/工具/审批。当前架构的职责切分已比较清晰，但存在可演进空间：

1. 层与层之间通过 concrete 实现直接耦合
2. 领域概念散落在 `knot.Message`、`HelmState`、`ToolUse` 等结构中
3. `helm` 通过 channel 直接与 server 耦合，Ask 流程需要断连重建

建议分阶段渐进演进：

```
依赖注入 → 分层清晰 → 领域模型显式化 → 事件总线解耦
```

> **注意**：依赖注入（DI）改造已部分完成，见迭代 [010](../iterations/010-di-refactor/main.md) / [011](../iterations/011-sail-provider/main.md) 及 [dependency-injection-plan.md](../dependency-injection-plan.md)。下文仅保留 DI 在架构演进中的定位，不展开细节。

---

## 方案设想

### 1. 分层架构

**目标分层**：

```
┌─────────────────────────────────────────┐
│  Transport Layer                        │
│  cli / server / future desktop plugin   │
├─────────────────────────────────────────┤
│  Application Layer                      │
│  AgentService                           │
├─────────────────────────────────────────┤
│  Domain Layer                           │
│  Session / Thread / Tool / Guard        │
├─────────────────────────────────────────┤
│  Infrastructure Layer                   │
│  FileVault / OpenAIProvider / HTTPClient│
└─────────────────────────────────────────┘
```

分层规则：上层可调下层、下层不知道上层、同层不直接依赖、依赖通过接口。

当前 Argo 规模较小，全面分层有 overhead。但 DI 改造天然是分层的基础——完成 DI 后分层自然浮现。

### 2. 领域模型显式化

#### 2.1 Tale 封装（高优先级 — 配合 compact / token 计数）

当前 `HelmState` 直接持有 `[]Message` 和 `Saved` 游标。引入 `reef.Compact()`（消息裁剪）后会暴露一致性问题：Compact 修改了 Messages 但 Saved 游标失效。

**方案**：将消息列表和持久化游标封装为 `Tale` 类型：

```go
type Tale struct {
    messages     []Message
    systemPrompt string
    saved        int            // 持久化游标，外部不可见
}

func (t *Tale) AppendUser(content string)
func (t *Tale) AppendAssistant(content string, toolCalls []ToolCall)
func (t *Tale) AppendTool(callID string, content string)
func (t *Tale) BuildRequest(tools []Tool, limit int) ChatRequest
func (t *Tale) Unsaved() []Message
func (t *Tale) MarkSaved()
```

`BuildRequest()` 内部自动触发 compact + token 计数，一致性由 Tale 保证，外部无感。

#### 2.2 ToolUse 拆分

当前 `ToolUse` 同时携带 LLM 发起的调用请求、门控审批结果、执行结果。未来可拆分为 `ToolCall`（LLM 发起）+ `ToolExecution`（执行上下文）。

#### 2.3 Guard 作为一等领域对象

当前 `Guard` 被 `voyage.Session` 持有。未来可让 `Guard` 独立存在，由 `helm` 显式调用 `guard.Evaluate(toolCall)`。

### 3. 事件驱动

引入内部事件总线，解决 Ask 断连重建问题：

```
helm ──AskUserEvent──► Event Bus ──► cli
cli ──AskReplyEvent──► Event Bus ──► helm
helm 继续执行，无需断连
```

当前适合**内存事件总线**（单机、进程内），后续按需迁移到 Redis/NATS。

---

## 决策条件

### 分层推进

| 阶段 | 内容 | 决策条件 |
|------|------|---------|
| 第一阶段 | 依赖注入 | ✅ 已执行（迭代 010/011） |
| 第二阶段 | Application Layer（AgentService） | `server/prompt.go` 进一步膨胀，或需要支持新传输层 |
| 第三阶段 | 事件总线 + 领域模型 | Ask 断连导致 bug / 需要 WebSocket 或桌面端接入 / 需要多会话并发 |

### 领域模型

- **Tale 封装**：实施 compact / token 计数时同步推进，否则一致性无法保证
- **ToolUse 拆分**：`ToolUse` 字段继续膨胀时
- **Guard 独立化**：需要支持多会话并发编辑或不同模式的对话线程时

### 事件驱动

- Ask 断连导致状态同步 bug
- 需要支持 WebSocket 或桌面端接入
- 需要多个组件同时响应同一事件

### 暂停条件

以下情况建议暂停所有架构演进：
- 核心功能尚未稳定
- 团队对当前代码还没有足够理解
- 没有多客户端或分布式的实际需求
- 测试覆盖不足，重构风险高

---

## 排序总览

| 方向 | 核心价值 | 当前优先级 | 决策条件 |
|------|----------|------------|----------|
| 依赖注入 | 解耦、可测试 | ✅ 已完成 | 迭代 010/011 |
| Tale 封装 | Compact/token 计数一致性安全 | 高 | 实施 compact / token 计数时同步推进 |
| 分层清晰 | 职责边界明确 | 中 | DI 完成后自然形成 |
| ToolUse 拆分 | 职责分离 | 中 | ToolUse 字段继续膨胀时 |
| 事件驱动 | 流程解耦、多客户端 | 低 | Ask 体验问题或多客户端需求 |
