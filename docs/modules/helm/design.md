# helm

## 概述

helm 是 Argo 的 Agent 核心循环。基于状态机模型驱动 "LLM 调用 → 工具执行 → 循环" 的 ReAct 链路。LoopState 仅持有控制流字段，对话历史由 Vault 管理。brig.Guard 在工具执行前做权限门控，EventAsk 通过 SSE 与调用方交互（DEC-031、DEC-035）。

## 核心概念

| 概念 | 说明 |
|------|------|
| LoopPhase | 状态机阶段枚举：PhaseLLM / PhaseExecuteTools / PhaseWaitAsk / PhaseDone |
| LoopState | 控制流状态结构体，可 JSON 序列化持久化到 state.json（不含对话历史） |
| Run | 状态机入口函数，串联 LLM ↔ ExecuteTools 直至暂停或结束 |
| runPhaseLLM | LLM 调用 + SSE 消费 + 工具调用收集，完成后立刻 vault.Append |
| runPhaseExecuteTools | 逐条 brig 门控 + 工具执行，每条结果立刻 vault.Append |
| Vault | 对话历史唯一数据源（transcript.jsonl），Phase 间通过 `*[]Message` 指针共享 |

## LoopState 字段

| 字段 | 说明 |
|------|------|
| Phase | 当前阶段 |
| TurnCount | 轮次计数 |
| ModelName | 当前模型 |
| ToolUses | 本轮 LLM 返回的全部工具调用（含执行结果） |
| ToolUseIndex | 当前处理到的工具下标 |
| AskReason | PhaseWaitAsk 时的确认原因 |
| AskID | PhaseWaitAsk 时的 ask 标识 |

## 流程

### Phase 流转

```
PhaseLLM
  │
  ├─ 无工具调用 → PhaseDone
  │
  └─ 有工具调用 → PhaseExecuteTools
                    │
                    ├─ 全部完成 → PhaseLLM
                    │
                    └─ 遇 Ask → PhaseWaitAsk → goroutine 退出
                                                 │
                                         外部回复后 Run() 重入
                                                 │
                                          PhaseExecuteTools（从 ToolUseIndex 续）
```

### Run() 签名

```
Run(ctx, st *LoopState, session *voyage.Session, msgs *[]knot.Message, reply knot.AskResult)
```

- `msgs` 为对话历史指针，goroutine 内 Phase 间共享
- `reply` 为 EventAsk 回复，新消息时传入 `AskDefault`
- 返回 `<-chan knot.Event` + SSE 事件流

### 写入时机

| 消息类型 | 写入点 |
|---------|--------|
| user | handlePrompt 入口立刻 Append |
| assistant | runPhaseLLM 结束后立刻 Append |
| tool | runPhaseExecuteTools 每条执行完立刻 Append |
| ask 回复 | run.go PhaseWaitAsk 入口处理后立刻 Append |

### goroutine 模型

Run() 内部启动一个 goroutine，使用 `emit` 闭包向返回的 channel 发送事件。goroutine 在以下情况退出：
- PhaseWaitAsk（存 LoopState 后退出，等外部回复）
- PhaseDone（emit EventDone 后退出）
- PhaseLLM 遇 sail.Chat 错误（emit EventError 后退出）

## 行为合约

### 上下文裁剪

- 每轮 PhaseLLM 调用前，将消息历史交给 `reef.Compact()` 处理
- 裁剪失败不阻塞，降级使用原始消息

### System prompt

- helm 主动拼装：行为核心 + 环境信息 + 工具列表 + AGENTS.md
- 每次 PhaseLLM 调用时重建（工具列表可能变化）

### 错误处理

- sail.Chat 错误：slog.Error + emit EventError → PhaseDone
- 工具执行失败：tu.Result = error → vault.Append → 继续下一条
- vault 写入失败：slog.Error，不阻塞流程
