# 008：Argo 前后端分离架构

**日期**：2026-07-07 ~
**状态**：🚧 进行中
**涉及模块**：helm、knot、vault、brig、voyage（新增）、server（新增）、cli

## 目标

将 Argo 从单体架构（CLI 直接调 RunLoop、vault、brig）改造为前后端分离架构——前端（CLI / TUI）只负责用户交互和事件渲染，后端（server 进程）承载 RunLoop、vault 写入、brig 门控、LLM 调用和工具执行。前后端通过 HTTP + SSE 流式事件协议通信。

## 范围

### 包含

- vault 包重构：Vault 接口精简为纯存储（绑定 session 目录），Session CRUD 迁出
- voyage 模块（新增）：Session 结构体（聚合 Vault + brig.Engine + session ID），Session CRUD
- RunLoop 参数设计：`TurnState`（对话状态）+ `voyage.Session`（会话级服务）
- vault 写入点：RunLoop 内部调用 `session.Append(records)`
- CLI 改造：从函数调用变为 HTTP + SSE 客户端
- permission 交互：EventAsk 通过 HTTP 桥接用户确认

### 不含

- TUI 实现（当前 CLI 保持行模式）
- 多 session 并发（MVP 单 session）
- 认证 / TLS
- WebSocket 替代 SSE
- session fork / continue / resume（后续迭代）

## 前后端交互协议

### 事件模型

沿用 `knot.Event`，新增 `EventMessageStart` / `EventMessageEnd` 标记消息边界：

```
EventStart          — RunLoop 启动
EventMessageStart   — 一条消息开始（user / assistant / tool）
EventTextDelta      — 文本增量
EventTextDone       — 文本完成
EventThinkingStart  — 思考开始
EventThinkingDelta  — 思考增量
EventThinkingDone   — 思考完成
EventToolUseDelta   — 工具调用（获批后才发出）
EventToolUseDone    — 工具执行完成
EventAsk            — 权限确认请求（带 permissionID）
EventError          — 错误
EventMessageEnd     — 一条消息结束
EventDone           — RunLoop 结束（带 Usage）
```

### REST 端点

```
POST /session/create  { cwd }
                      → { sessionID }

POST /session/resume  { sessionID }
                      → { session: SessionInfo }

POST /session/prompt  { sessionID, message }
                      → SSE stream<Event>

POST /session/delete  { sessionID }
                      → {}

POST /ask/reply       { sessionID, askID, allow }
                      → {}

GET  /session/list
                      → { sessions: [SessionInfo] }
```

### SSE 流格式

```
data: {"type":"EventMessageStart","message":{"role":"user",...}}

data: {"type":"EventStart"}

data: {"type":"EventTextDelta","delta":"Hello"}

data: {"type":"EventToolUseDelta","tool_use":{"name":"read",...}}

data: {"type":"EventAsk","permission_id":"perm_xxx","tool_use":{...},"reason":"..."}
                                                          ← 服务端阻塞等待
POST /permission/reply  { sessionID, permissionID: "perm_xxx", allow: true }
                                                          → 服务端继续执行

data: {"type":"EventToolUseDone","tool_use":{...}}

data: {"type":"EventDone","usage":{...}}

data: {"type":"EventMessageEnd","message":{"role":"assistant",...}}
```

### permission 交互流（基于状态机检查点）

```
  Frontend                              Backend
     │                                     │
     │  POST /session/prompt               │
     │  ─────────────────────────────────▶  │  ResumeSession → NewPromptState → Run()
     │  ◀── SSE data: EventAsk ──────────  │  遇 Ask → 存 LoopState(PhaseWaitAsk) → goroutine 退出
     │                                     │
     │  askUser() → y/n                    │
     │                                     │
     │  POST /ask/reply                    │
     │  ─────────────────────────────────▶  │  loadLoopState → NewReplyState → Run()
     │  ◀── SSE data: EventToolUseDone ──  │  继续执行工具
     │  ◀── SSE data: EventDone ────────  │  PhaseDone
```

## 模块关系

```
vault ─┐
        ├── voyage ── helm
brig ──┘

vault: Append / Load                   ← 纯存储，绑定 session 目录
brig:  Guard 接口（FileGuard 实现）       ← 工具权限门控 + 用户审批持久化
voyage: Session（嵌入 SessionInfo + Vault + Guard），CRUD  ← 会话聚合层
helm:  LoopState 状态机 + Run() 入口     ← ReAct 循环，支持暂停/恢复
```

### API 类型

```
knot (内部类型)
  ↑
server/event_json.go (JSON 序列化层)
  ↑
SSE → 前端
```

## 核心 API

### helm.Run（`src/helm/run.go`）

```go
func Run(ctx context.Context, st *LoopState, session *voyage.Session) (<-chan knot.Event, error)
```

单入口，覆盖 PhaseLLM → PhaseExecuteTools → PhaseWaitAsk / PhaseDone 全流程。

### LoopState（`src/helm/state_machine.go`）

```go
type LoopState struct {
    Phase        LoopPhase   // 当前阶段
    Messages     []knot.Message  // 完整对话历史
    TurnCount    int
    ModelName    string
    ToolUses     []knot.ToolUse  // 本轮工具调用
    ToolUseIndex int
    AskReason    string      // PhaseWaitAsk 时有效
    AskID        string
    AskReply     knot.AskResult
    // ...
}

func NewPromptState(messages []knot.Message, modelName, userMsg string) *LoopState
func NewReplyState(prev *LoopState, reply knot.AskResult) *LoopState
```

`LoopState` 可 JSON 序列化，PhaseWaitAsk 时持久化到 session 目录。

### brig.Guard（`src/brig/brig.go`）

```go
type Guard interface {
    Evaluate(tu knot.ToolUse) (Verdict, string)
    Approve(tu knot.ToolUse)
    IsCachedApproval(tu knot.ToolUse) bool
    Snapshot() []ApprovalEntry
    Restore([]ApprovalEntry)
}
```

### voyage.Session（`src/voyage/voyage.go`）

```go
type Session struct {
    SessionInfo           // 嵌入，字段提升（ID, WorkingDir, CreatedAt 等）
    Vault  vault.Vault
    Guard  brig.Guard
}
func CreateSession(baseDir, cwd string) (*Session, error)
func ResumeSession(baseDir, id string) (*Session, error)  // 自动恢复 approvals
func ListSessions(baseDir string) ([]SessionInfo, error)
func DeleteSession(baseDir, id string) error
func RenameSession(baseDir, id, name string) error
func SaveApprovals(s *Session) error
```

## Server 启动流程

```
1. knot.LoadConfig()
2. wake.NewLogWriter(cfg.Log)
3. deck.RegisterTools()
4. http.ListenAndServe(:4090, chiRouter)
```

每个 `/session/prompt` 请求：

```
1. voyage.ResumeSession(baseDir, sessionID)  — 从磁盘加载，恢复 Guard 审批记录
2. session.Vault.Load()                      — 读对话历史
3. helm.NewPromptState(msgs, modelName, msg)  — 构造入口状态
4. helm.Run(ctx, st, session)                — 启动状态机，返回 event channel
5. streamAndSave(w, ..., events, st, session) — SSE 转发 + 通道关闭后持久化 LoopState + approvals
```

每个 `/ask/reply` 请求：

```
1. loadLoopState(baseDir, sessionID)         — 加载暂停的 LoopState
2. voyage.ResumeSession(baseDir, sessionID)
3. helm.NewReplyState(st, reply)             — 注入用户回复
4. helm.Run(ctx, st, session)
5. streamAndSave(...)
```

## 整体开发状态

| 功能 | 状态 | 涉及文件 |
|------|------|---------|
| 协议设计 + 参数决策 + 模块拆分 | ✅ 完成 | — |
| vault 接口精简 + 错误处理规范 | ✅ 完成 | `src/vault/vault.go`, `src/vault/file_vault.go` |
| voyage 模块（Session + CRUD） | ✅ 完成 | `src/voyage/voyage.go` |
| Message → Record 转换 | ✅ 完成 | `src/vault/convert.go` |
| TurnState → LoopState 状态机 | ✅ 完成 | `src/helm/state_machine.go`, `src/helm/run.go`, `src/helm/phase_*.go` |
| brig Guard 接口化 + 审批持久化 | ✅ 完成 | `src/brig/brig.go` |
| server 模块（无状态 HTTP 后端） | ✅ 完成 | `src/server/` |
| knot.Event 清理（AskReply 删除） | ✅ 完成 | `src/knot/types.go` |
| sail 超时从 doChat 提升到 runPhaseLLM | ✅ 完成 | `src/sail/adapter.go`, `src/helm/phase_llm.go` |
| CLI HTTP 客户端改造 | ✅ 完成 | `src/cli/`（client.go, crud.go, sse.go, convert.go, server.go, ui.go, session_pick.go, ask.go, main.go） |
| 模块设计文档更新 | ✅ 完成 | helm、server、vault、voyage、brig、knot、sail、deck 8 个 design.md |

## 关键决策

| 日期 | 决策 | 原因 |
|------|------|------|
| 2026-07-08 | RunLoop → 状态机 (LoopState + Phase) | 消除 goroutine 阻塞，支持暂停/恢复/持久化，server 无状态化 |
| 2026-07-08 | Phase 函数用 emit 回调而非 channel | 语义清晰——emit 方向明确"产出"，不暴露 channel 生命周期 |
| 2026-07-08 | PhaseWaitAsk 时 LoopState → 磁盘 JSON | server 无状态：每次请求 Resume → Run → 存盘，不需要 sessions map |
| 2026-07-08 | brig.Engine → Guard 接口 + FileGuard 实现 | 可测试、可替换；voyage 是唯一构造注入点 |
| 2026-07-08 | 审批记录持久化到 approvals.json | 会话重启后 userApprovals 不丢失，server 无状态的前提条件 |
| 2026-07-08 | knot.Event → eventJSON 序列化层分离 | channel、error 等类型不可 JSON 序列化，API 契约需要独立类型 |
| 2026-07-08 | Session 嵌入 SessionInfo 而非持有 ID 字段 | 减少重复字段，Session 自带完整元数据 |
| 2026-07-08 | Delete/Rename 从 Session 方法 → 独立函数 | server 无状态后不需要先 Resume 再操作，直接操作磁盘 |
| 2026-07-08 | sail 超时从 doChat 提升到 runPhaseLLM | SSE goroutine 的 context 生命周期与 HTTP transport 冲突；提升后 defer cancel 在 for range 消费完 events 后触发 |
| 2026-07-08 | 底层函数返原错，调用方决定语义 | readRecords/loadApprovals 不处理 os.IsNotExist，由 Load/ResumeSession 各自解释 |

## 需更新的模块设计文档

| 模块 | 文件 | 变更点 |
|------|------|--------|
| helm | [docs/modules/helm/design.md](docs/modules/helm/design.md) | 状态机替代 RunLoop：LoopState、Phase 流转、emit 模式、Run() 签名 |
| brig | [docs/modules/brig/design.md](docs/modules/brig/design.md) | Guard 接口、FileGuard 改名、Snapshot/Restore、ApprovalEntry |
| sail | [docs/modules/sail/design.md](docs/modules/sail/design.md) | ChatDefaultTimeout 导出、超时位置变更 |
| knot | [docs/modules/knot/design.md](docs/modules/knot/design.md) | Event.AskReply 删除、事件模型说明 |
| voyage | 新建 `docs/modules/voyage/design.md` | 新增模块 |
| server | 新建 `docs/modules/server/design.md` | 新增模块 |
| vault | [docs/modules/vault/design.md](docs/modules/vault/design.md) | readRecords 错误处理规范（可选，较小改动） |

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-07-12 | 模块设计文档全部更新完成（8 个 design.md） |
| 2026-07-08 | vault 拆分为纯存储 + voyage 模块（Session 聚合 Vault + Engine）；TurnState 精简为纯对话状态 |
| 2026-07-08 | 状态机重构完成：helm RunLoop → LoopState + Phase；brig Guard 接口化 + 审批持久化；server 无状态化 |
| 2026-07-08 | CLI HTTP 客户端改造完成：8 个新文件，复用 server 包导出类型；自动启动/连接 argo-server；SSE 流解析 + EventAsk 桥接 |
| 2026-07-07 | 迭代开始；协议设计 + 参数决策完成 |

## 小 TODO

- [x] ~~跨平台 server 二进制名适配~~（`src/cli/server.go`）：默认 `argo-server`，仅在同目录存在 `.exe` 时使用，已兼容全平台。
- [x] ~~Vault 成为对话历史与状态的唯一数据源~~：已在 009-vault-single-source 中完成——Messages `json:"-"`、Saved 游标增量写入、handlePrompt 按分支读 transcript。
