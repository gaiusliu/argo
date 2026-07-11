# server

## 概述

server 是 Argo 的无状态 HTTP 后端。每次请求自行从磁盘加载会话状态（Vault + HelmState），处理完成后持久化。通过 SSE 流式返回事件给前端（CLI / TUI）。

server 依赖 voyage、helm、knot，使用 chi 路由框架。

## 核心概念

| 概念 | 说明 |
|------|------|
| Server | 无状态 HTTP server 结构体，持有 argoDir + chi router |
| handlePrompt | 统一入口 handler，根据 ToolUseVerdicts 分支处理新消息和 Ask 回复 |
| streamAndSave | SSE 流式转发事件（持久化由 helm.checkpoint 负责） |
| HelmState | 控制流状态结构体，由 helm 包管理，通过 state.json 持久化到 session 目录 |
| EventJSON / ToolUseJSON | knot.Event / knot.ToolUse 的 HTTP API 序列化视图 |

## 路由

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/session/new` | 创建新会话 → `{ sessionID }` |
| POST | `/session/prompt` | 统一入口：ToolUseVerdicts 为空=新消息，非空=Ask 回复 → SSE 流 |
| POST | `/session/resume` | 恢复已有会话 → `{ session }` |
| GET | `/session/list` | 列出所有会话 → `{ sessions }` |
| POST | `/session/delete` | 删除会话 → `{}` |

## 流程

### handlePrompt 统一入口

```
新消息路径（ToolUseVerdicts 为空）:
  voyage.ResumeVoyage(sessionID) → session
  session.Load() → msgs
  st = helm.NewHelmState(model, msgs)
  st.Messages += user message
  helm.Run(ctx, st, session) → events
  streamAndSave(w, events)

Ask 回复路径（ToolUseVerdicts 非空）:
  voyage.ResumeVoyage(sessionID) → session
  session.Load() → msgs
  st = helm.NewHelmState(model, msgs)
  st.Load(sessionID)              ← 从 state.json 恢复 Phase/ToolUses/Saved
  将 ToolUseVerdicts 写入 st.ToolUses（匹配 ID）
  helm.Run(ctx, st, session) → events
  streamAndSave(w, events)
```

### streamAndSave

```
for ev := range events:
  writeSSE(w, ev)           ← Event → EventJSON → SSE 格式
  flusher.Flush()
```

持久化由 helm 内部的 `checkpoint()` 在 Asking / Done 阶段完成，streamAndSave 只负责 SSE 转发。

## 无状态设计

server 不维护任何会话级内存状态：

- 每次 HTTP 请求独立：Vault.Load → 构造 HelmState → Run → 流式转发
- EventAsk 出现时 goroutine 退出，HelmState 存 state.json，SSE 关闭
- 客户端发起新请求（带 ToolUseVerdicts）时从 state.json 恢复，Phase 自动变为 ExecTools 继续执行

## 关键文件

| 文件 | 职责 |
|------|------|
| `server.go` | Server 结构体 + Config + New() 路由注册 + Session CRUD handlers |
| `prompt.go` | handlePrompt 统一入口 + streamAndSave |
| `sse.go` | writeSSE — Event → ToEventJSON → SSE 格式化 |
| `event_json.go` | EventJSON / ToolUseJSON / UsageJSON 序列化视图 + ToEventJSON / toolUsesToJSON |
| `types.go` | 请求体 / 响应体 JSON 类型（NewPromptRequest 等） |
