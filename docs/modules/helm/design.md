# helm

## 概述

helm 是 Argo 的 Agent 核心循环。基于状态机模型驱动 "LLM 调用 → 工具执行 → 循环" 的 ReAct 链路。HelmState 仅持有控制流字段，对话历史由 Vault（transcript.jsonl）管理。brig.Guard 在工具执行前做权限门控，EventAsk 通过 SSE 与调用方交互（DEC-031、DEC-035）。

## 核心概念

| 概念 | 说明 |
|------|------|
| Phase | 状态机阶段常量：HelmPhaseModel / HelmPhaseExecTools / HelmPhaseAsking / HelmPhaseDone |
| HelmState | 控制流状态结构体，可 JSON 序列化持久化到 state.json（Messages 字段不持久化） |
| Run | 状态机入口函数，goroutine 内循环执行各 Phase 直至 Asking 或 Done 退出 |
| PhaseRunner | 阶段执行器接口，4 个实现：PhaseRunnerModel / PhaseRunnerExecTools / PhaseRunnerAsking / PhaseRunnerDone |
| newRunner | 工厂函数，按 Phase 值返回对应的 PhaseRunner 实现 |
| checkpoint | 断点持久化函数，增量写入 transcript + 审批记录，在 Asking / Done 阶段调用 |

## HelmState 字段

| 字段 | 说明 |
|------|------|
| Phase | 当前阶段（HelmPhaseModel=1, ExecTools=2, Asking=3, Done=4） |
| Model | 当前使用的模型名 |
| ToolUses | 当前轮次 LLM 返回的工具调用列表（含执行结果和 verdict） |
| Saved | 已持久化到 transcript.jsonl 的消息游标：Messages[:Saved] 已在 transcript，Messages[Saved:] 待写入 |
| Messages | 完整对话历史，`json:"-"` 不持久化到 state.json（transcript.jsonl 是唯一持久化来源） |

## 流程

### Phase 流转

```
HelmPhaseModel
  │
  ├─ 无工具调用 → HelmPhaseDone
  │
  └─ 有工具调用 → HelmPhaseExecTools
                    │
                    ├─ 全部执行完成 → HelmPhaseModel（循环）
                    │
                    └─ 遇 Ask → HelmPhaseAsking → goroutine 退出
                                                     │
                                             外部填充 ToolUseVerdicts 后 Run() 重入
                                                     │
                                             HelmPhaseExecTools（继续执行）
```

### Run() 签名

```
Run(ctx context.Context, st *HelmState, session voyage.Voyage) <-chan knot.Event
```

- 内部启动 goroutine，通过 `for` 循环依次执行各 Phase
- 返回带缓冲的 `<-chan knot.Event`（`make(chan knot.Event, 16)`），调用方通过 SSE 转发
- Asking 或 Done 阶段 goroutine 退出，等待外部 HTTP 请求再次驱动

### PhaseRunner 接口

```
type PhaseRunner interface {
    Run(ctx context.Context, st *HelmState, emit func(knot.Event), session voyage.Voyage)
}
```

| 实现 | Phase | 职责 |
|------|-------|------|
| PhaseRunnerModel | HelmPhaseModel | 构建 system prompt + 调用 LLM + 收集 tool_calls，流转到 ExecTools 或 Done |
| PhaseRunnerExecTools | HelmPhaseExecTools | 逐条 brig 门控 + 工具执行，遇 Ask 流转到 Asking |
| PhaseRunnerAsking | HelmPhaseAsking | checkpoint 落盘 + Save HelmState → goroutine 退出等待外部回复 |
| PhaseRunnerDone | HelmPhaseDone | checkpoint 落盘 → goroutine 退出 |

### checkpoint 机制

checkpoint() 在 Asking 和 Done 两个终点阶段被调用，执行两次持久化：

1. **增量写入 transcript**：`vault.MessagesToRecords(Messages[Saved:], …)` → `session.Append()`
2. **审批记录**：`voyage.SaveApprovals(session)` → `approvals.json`

写入失败不阻塞流程，仅 slog.Error 记录。

### 写入时机

| 消息类型 | 写入点 |
|---------|--------|
| user | handlePrompt 入口追加到 st.Messages，checkpoint 时增量写入 |
| assistant | PhaseRunnerModel.Run 结束后追加到 st.Messages |
| tool | PhaseRunnerExecTools.Run 每条执行完追加到 st.Messages |
| 全部持久化 | checkpoint() 统一增量写入（Asking / Done 阶段） |

### goroutine 模型

Run() 内部启动一个 goroutine，使用 `emit` 闭包向返回的 channel 发送事件。goroutine 在以下情况退出：

- HelmPhaseAsking（checkpoint + Save HelmState 后退出，等外部回复）
- HelmPhaseDone（checkpoint 后退出）
- PhaseRunnerModel.Run 遇 LLM 错误（emit EventError 后设 Phase=Done，下一轮退出）

## 行为合约

### 上下文裁剪

- 每轮 PhaseRunnerModel.Run 调用前，将消息历史交给 `reef.Compact()` 处理
- 裁剪失败不阻塞，降级使用原始消息

### System prompt

- helm 主动拼装：行为核心 + 环境信息（OS/日期/配置目录） + 工具列表 + AGENTS.md
- 每次 PhaseRunnerModel.Run 调用时通过 `buildSystemPrompt()` 重建（工具列表可能变化）

### 错误处理

- LLM 调用错误：slog.Error + emit EventError → Phase = Done
- 工具执行失败：tu.Result = StatusError → 继续下一条
- Vault 写入失败：slog.Error，不阻塞流程

## 关键文件

| 文件 | 职责 |
|------|------|
| `run.go` | Run() 入口，goroutine + 事件 channel（内联 `make(chan knot.Event, 16)`） + Phase 循环 |
| `helm_state.go` | HelmState 定义 + NewHelmState + Save/Load 持久化 + state.json 路径 |
| `phase_runner.go` | PhaseRunner 接口 + newRunner 工厂函数 |
| `phase_model.go` | PhaseRunnerModel：LLM 调用 + checkpoint |
| `phase_exec_tools.go` | PhaseRunnerExecTools：brig 门控 + 工具执行 |
| `phase_asking.go` | PhaseRunnerAsking：断点持久化 + 等待外部回复 |
| `phase_done.go` | PhaseRunnerDone：最终 checkpoint |
| `prompt.go` | buildSystemPrompt：拼装 system prompt |
