# server

## 概述

server 是 Argo 的无状态 HTTP 后端。每次请求自行从磁盘加载 Voyage，处理完成后持久化。通过 SSE 流式返回事件给前端（CLI / TUI）。使用 chi 路由框架。

## 路由

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/session/new` | 创建新会话 → `{ sessionID }` |
| POST | `/session/prompt` | 统一入口：ToolUseVerdicts 为空=新消息，非空=Ask 回复 → SSE 流 |
| POST | `/session/resume` | 恢复会话 → `{ session, messages }`（含对话历史） |
| GET | `/session/list` | 列出所有会话 → `{ sessions }` |
| POST | `/session/delete` | 删除会话 → `{}` |
| POST | `/session/interrupt` | 终止 Asking → 删 state.json → `{ status: "ok" }` |

## handlePrompt 流程

```
新消息路径（ToolUseVerdicts 为空）:
  voyage.Resume(sessionID) → v
  v.Phase = PhaseModel + v.Model = req.Model + v.Tale().AppendUser(msg)
  helm.Run(ctx, v) → events
  streamAndSave(w, events)

Ask 回复路径（ToolUseVerdicts 非空）:
  voyage.Resume(sessionID) → v
  v.LoadState()                           ← 恢复 Phase/ToolUses/PendingMessages
  将 verdicts 写入 v.ToolUses
  helm.Run(ctx, v) → events
  streamAndSave(w, events)
```

## ResumeSession 响应

`POST /session/resume` 返回 `ResumeSessionResponse`：

```go
type ResumeSessionResponse struct {
    SessionInfo SessionInfoJSON `json:"session"`
    Messages    []knot.Message  `json:"messages"`   // 完整对话历史
}
```

Messages 从 vault 加载，CLI 恢复会话时用于展示历史。

## 中断端点

`POST /session/interrupt` 接收 `{ sessionID }`，调用 `voyage.DeleteState(id)` 删除 state.json。返回 `{ status: "ok" }`。文件不存在也不报错（覆盖写模式）。

## 无状态设计

server 不维护任何会话级内存状态：

- 每次 HTTP 请求独立：Vault.Load → Resume Voyage → Run → SSE 转发
- EventAsk 出现时 goroutine 退出，Voyage.SaveState 存 state.json
- 客户端通过 Ctrl+C 触发中断（ctx.Done → goroutine 退出）
- Asking 阶段终止通过 `/interrupt` 端点删 state.json

参考 [DEC-035](/docs/decisions.md#dec-035server-无状态状态机暂停通过磁盘序列化--goroutine-退出) 和 [DEC-039](/docs/decisions.md#dec-039signal-中断ctrlc-截获触发-agent-loop-终止)。

## 关键文件

| 文件 | 职责 |
|------|------|
| `server.go` | Server 结构体 + Config + New() 路由注册 + Session CRUD + Interrupt handler |
| `prompt.go` | handlePrompt 统一入口 + streamAndSave |
| `sse.go` | writeSSE — Event → ToEventJSON → SSE 格式化 |
| `event_json.go` | EventJSON / ToolUseJSON / UsageJSON 序列化视图 |
| `types.go` | 请求体 / 响应体 JSON 类型 |
