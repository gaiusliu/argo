# server

## 概述

server 是 Argo 的无状态 HTTP 后端。每次请求自行从磁盘加载会话状态（Vault + LoopState），处理完成后持久化。通过 SSE 流式返回事件给前端（CLI / TUI）。

server 依赖 voyage、helm、knot，使用 chi 路由框架。

## 核心概念

| 概念 | 说明 |
|------|------|
| Server | 无状态 HTTP server 结构体，持有 baseDir + chi router |
| handlePrompt | 统一入口 handler，根据 AskID 字段分支处理新消息和 Ask 回复 |
| streamAndSave | SSE 流式转发 + 通道关闭后持久化 LoopState 和审批记录 |
| state.json | LoopState 控制流字段的 JSON 持久化文件，存于 session 目录 |
| EventJSON | knot.Event 的 HTTP API 序列化视图，排除 channel/error 等不可序列化字段 |

## 路由

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/session/create` | 创建新会话 → `{ sessionID }` |
| POST | `/session/prompt` | 统一入口：AskID 为空=新消息，非空=Ask 回复 → SSE 流 |
| POST | `/session/resume` | 恢复已有会话 → `{ session }` |
| GET | `/session/list` | 列出所有会话 → `{ sessions }` |
| POST | `/session/delete` | 删除会话 → `{}` |

## 流程

### handlePrompt 统一入口

```
新消息路径（AskID 为空）:
  Vault.Load() → msgs
  msgs += user
  session.Append(user record)
  st = NewPromptState(modelName)
  Run(ctx, st, session, &msgs, reply=AskDefault)

Ask 回复路径（AskID 非空）:
  Vault.Load() → msgs
  loadLoopState() → st
  校验 Phase==WaitAsk, AskID 匹配
  设置 reply = AskAllowed / AskDefault
  Run(ctx, st, session, &msgs, reply)
```

### streamAndSave

```
for ev := range events:
  writeSSE(w, ev)           ← Event → EventJSON → SSE 格式
  flusher.Flush()

通道关闭后:
  if Phase == WaitAsk:
    saveLoopState()          ← 序列化控制流到 state.json
  voyage.SaveApprovals()     ← 审批记录到 approvals.json
```

## 无状态设计

server 不维护任何会话级内存状态：
- 每次 HTTP 请求独立：Vault.Load → 构造 LoopState → Run → 流式转发 → 持久化
- EventAsk 出现时 goroutine 退出，LoopState 存 state.json，SSE 关闭
- 客户端发起新请求（带 AskID）时从 state.json 恢复继续

## 关键文件

| 文件 | 职责 |
|------|------|
| `server.go` | Server 结构体 + chi 路由注册 + Session CRUD handlers |
| `prompt.go` | handlePrompt 统一入口 + streamAndSave |
| `sse.go` | writeSSE — Event → EventJSON → SSE 格式化 |
| `event_json.go` | EventJSON / ToolUseJSON / UsageJSON 序列化视图 |
| `state.go` | saveLoopState / loadLoopState 磁盘存取 |
| `types.go` | 请求体 / 响应体 JSON 类型 |
