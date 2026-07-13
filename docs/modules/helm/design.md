# helm

## 概述

helm 是 Argo 的 Agent 核心循环，基于状态机驱动 "LLM 调用 → 工具执行 → 循环" 的 ReAct 链路。控制流状态和对话历史全部归属 Voyage（[DEC-038](/docs/decisions.md#dec-038voyage-单一聚合体去接口--session-改名--helmstate-合并--tale-归属)）。

## 核心概念

| 概念 | 说明 |
|------|------|
| Phase | 状态机阶段常量（定义在 voyage 包）：`PhaseModel` / `PhaseExecTools` / `PhaseAsking` / `PhaseDone` |
| Run | 状态机入口：`func Run(ctx, v) <-chan knot.Event`。内部 goroutine 循环执行各 Phase |
| PhaseRunner | 阶段执行器接口：`Run(ctx, v, emit) bool`，返回 `true`=goroutine 退出 |
| newRunner | 工厂函数，按 Phase 值返回对应的 PhaseRunner |
| checkpoint | Done 阶段的断点持久化：增量写 vault + 审批记录。Asking 阶段**不调** checkpoint |

## Phase 流转

```
PhaseModel
  │
  ├─ 无工具调用 → PhaseDone
  │
  └─ 有工具调用 → PhaseExecTools
                    │
                    ├─ 全部执行完成 → PhaseModel（循环）
                    │
                    └─ 遇 Ask → PhaseAsking → goroutine 退出
                                                     │
                                             外部填充 ToolUseVerdicts 后 Run() 重入
                                                     │
                                             PhaseExecTools（继续执行）
```

## Run() 签名

```go
func Run(ctx context.Context, v *voyage.Voyage) <-chan knot.Event
```

- 内部启动 goroutine，`for` 循环依次执行各 Phase
- 每轮循环前检查 `ctx.Done()`——中断（Ctrl+C）时直接退出，不调 checkpoint
- 返回 `make(chan knot.Event, 16)`，调用方通过 SSE 转发

## PhaseRunner 接口

```go
type PhaseRunner interface {
    Run(ctx context.Context, v *voyage.Voyage, emit func(knot.Event)) bool
}
```

| 实现 | Phase | 职责 |
|------|-------|------|
| PhaseRunnerModel | PhaseModel | 调用 LLM，收集 tool_calls，追加 assistant 到 Tale |
| PhaseRunnerExecTools | PhaseExecTools | brig 门控 + 工具执行 + 追加 tool 结果到 Tale |
| PhaseRunnerAsking | PhaseAsking | 设置 PendingMessages → SaveState → goroutine 退出 |
| PhaseRunnerDone | PhaseDone | checkpoint（写 vault + 审批） → goroutine 退出 |

## Asking 延迟持久化

Asking 阶段**不调 checkpoint**（不写 vault）。本轮尚未落盘的 user + assistant(tool_calls) 以 `PendingMessages` 存入 state.json。PhaseDone 时一次性写完整对话。

```
PhaseAsking:
  v.PendingMessages = v.Tale().Unsaved()   ← user + assistant(tool_calls)
  v.Phase = PhaseExecTools
  v.SaveState()                            ← state.json 含 PendingMessages

中断 / /interrupt:
  DeleteState(id) → state.json 删 → Resume 回到断点前

Ask 回复:
  LoadState → PendingMessages 恢复进 Tale → exec tools → Model → Done
  checkpoint → 完整对话写入 vault
```

参考 [DEC-040](/docs/decisions.md#dec-040asking-延迟持久化phaseasking-不写-vaultpendingmessages-暂存-statejson)。

## 中断机制

- 服务端 `run.go`：每轮循环前 `select { case <-ctx.Done(): return }`
- 客户端 Ctrl+C → `signal.Notify` → `cancel()` → ctx.Done → goroutine 退出
- 中断时不调 checkpoint，内存态丢弃，下次 Resume 回到上一断点

参考 [DEC-039](/docs/decisions.md#dec-039signal-中断ctrlc-截获触发-agent-loop-终止)。

## 关键文件

| 文件 | 职责 |
|------|------|
| `run.go` | Run() 入口，goroutine + 事件 channel + Phase 循环 + ctx.Done 检查 |
| `phase_runner.go` | PhaseRunner 接口 + newRunner 工厂 |
| `phase_model.go` | PhaseRunnerModel：LLM 调用 + Tale.AppendAssistant |
| `phase_exec_tools.go` | PhaseRunnerExecTools：brig 门控 + 工具执行 |
| `phase_asking.go` | PhaseRunnerAsking：PendingMessages + SaveState |
| `phase_done.go` | PhaseRunnerDone：checkpoint 落盘 |
| `prompt.go` | buildSystemPrompt：拼装 system prompt |
