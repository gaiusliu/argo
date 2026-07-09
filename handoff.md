# Handoff — 状态机重构 + Server 模块 + Guard 接口化

## 本次会话成果

### 1. helm 状态机（核心重构）

RunLoop 从 "goroutine + channel 阻塞等待 Ask" 改为纯状态机模型。

**文件：**

| 文件 | 内容 |
|------|------|
| [src/helm/state_machine.go](src/helm/state_machine.go) | `LoopPhase` 枚举 + `LoopState` 结构体 + `NewPromptState` / `NewReplyState` 构造函数 |
| [src/helm/phase_llm.go](src/helm/phase_llm.go) | `runPhaseLLM(ctx, st, emit)` — LLM 调用 + SSE 流式消费 |
| [src/helm/phase_execute_tools.go](src/helm/phase_execute_tools.go) | `runPhaseExecuteTools(ctx, st, session, emit) bool` — 门控 + 工具执行 + vault 写入 |
| [src/helm/run.go](src/helm/run.go) | `Run(ctx, st, session)` — 状态机入口，串联 Phas		eLLM ↔ PhaseExecuteTools |

`LoopState` 可 JSON 序列化，`PhaseWaitAsk` 时存磁盘，回复后加载继续。

**Phase 流转：**
```
PhaseLLM → (无工具) PhaseDone
         → (有工具) PhaseExecuteTools
                       → (全部完成) PhaseLLM
                       → (遇 Ask) PhaseWaitAsk → 外部回复 → PhaseExecuteTools
```

### 2. brig 模块 — Guard 接口化

- `Engine` → `FileGuard`，新增 `Guard` interface
- `Session.Engine` → `Session.Guard`，类型从 `*brig.FileGuard` 改为 `brig.Guard`
- `Guard` 接口新增 `Snapshot() []ApprovalEntry` / `Restore([]ApprovalEntry)`
- 审批记录持久化到 `approvals.json`，`ResumeSession` 时自动恢复

**文件：** [src/brig/brig.go](src/brig/brig.go)

### 3. voyage 模块 — 接口优化

- `Resume(baseDir, id)` — 去掉 `cwd` 参数，内部读 `session.json` 取 `WorkingDir`
- `Delete` / `Rename` → `DeleteSession(baseDir, id)` / `RenameSession(baseDir, id, name)` 独立函数
- `Session` 嵌入 `SessionInfo`，`session.ID` 等字段自动提升
- `Append` 直接更新嵌入的 `SessionInfo`，不再从磁盘重读
- `SaveApprovals(session)` 持久化审批记录

**文件：** [src/voyage/voyage.go](src/voyage/voyage.go)

### 4. server 模块 — 无状态 HTTP 后端

Server 完全无状态——去掉 `sessions map`、`mu`、`pendingAsks` channel map。

**路由（全部 POST + JSON body，除 list 为 GET）：**
```
POST /session/create   → voyage.CreateSession → 返回 sessionID
POST /session/resume   → voyage.ResumeSession → 返回 session 元数据
POST /session/prompt   → helm.Run(NewPromptState) → SSE 流
POST /session/delete   → voyage.DeleteSession
POST /ask/reply        → helm.Run(NewReplyState) → SSE 流
GET  /session/list     → voyage.ListSessions
```

**文件：**

| 文件 | 内容 |
|------|------|
| [src/server/server.go](src/server/server.go) | `Server` 结构体 + chi 路由 + CRUD handlers |
| [src/server/prompt.go](src/server/prompt.go) | `handlePrompt` + `handleAskReply` + `streamAndSave` |
| [src/server/sse.go](src/server/sse.go) | `writeSSE` — Event → JSON → SSE 格式 |
| [src/server/event_json.go](src/server/event_json.go) | `eventJSON` — `knot.Event` 的 JSON 序列化层（排除 channel/error 字段） |
| [src/server/state.go](src/server/state.go) | `saveLoopState` / `loadLoopState` 磁盘存取 |
| [src/server/types.go](src/server/types.go) | 请求体 + 响应体 + `sessionJSON` JSON 视图类型 |

### 5. knot.Event 清理

- 删除了 `Event.AskReply chan AskResult`，server 模式下无 channel 桥接
- 注释格式优化：所有行尾注释改为上方独立行
- `eventJSON` 在 server 包提供序列化层

### 6. sail 适配器 — 超时机制修正

- `ChatDefaultTimeout` 导出到包级别
- 超时逻辑从 `doChat` 提升到 `runPhaseLLM` 层——`llmCtx` 覆盖 LLM 调用 + SSE 消费全程，`defer cancel` 在 `for range` 消费完 events 后才触发

**文件：** [src/sail/adapter.go](src/sail/adapter.go), [src/sail/sail.go](src/sail/sail.go)

### 7. vault 模块 — 错误处理规范

- `readRecords` 只返回原始错误，文件不存在的判断上移到 `Load()` 层
- 与 `loadApprovals` / `ResumeSession` 保持一致的错误处理模式

### 8. 编译产物

- `argo-server.exe` — 项目根目录，可直接运行。监听 `:4090`

## 当前模块依赖

```
knot (共享类型，叶子)
  ↑
vault (纯存储 Append/Load)
  ↑
brig (Guard 接口 + FileGuard 实现)
  ↑
sail (LLM 适配层)
  ↑
helm (LoopState 状态机 + Run 入口)
  ↑
voyage (Session 聚合 Vault + Guard)
  ↑
server (无状态 HTTP 路由)
  ↑
argo-server/main.go (二进制入口)
```

## 遗留状态

- **CLI (`src/cli/main.go`)** 仍在用旧的 `helm.RunLoop` 接口（含 `onPause` 参数），传 `nil` 会在运行时 panic。已临时注释掉 `ev.AskReply <- result` 行（`AskReply` 字段已从 `knot.Event` 删除）
- 旧 `loop.go` / `checkpoint.go` 文件仍存在，CLI 迁完后可删除

## 下一步：CLI 改为 HTTP 客户端

目标：CLI 不再直接调用 helm/voyage/brig，改为通过 HTTP + SSE 与 argo-server 通信。

### 涉及文件

- **`src/cli/main.go`** — 大幅改写

### CLI 流程

```
1. GET /session/list → 展示会话
2. POST /session/create 或 /session/resume
3. 用户输入消息
4. POST /session/prompt (SSE)
   ├─ EventTextDelta → stdout
   ├─ EventAsk → stdin y/n → POST /ask/reply (SSE)
   └─ EventDone → 回到步骤 3
```

### 设计要点

- HTTP 客户端直连 `localhost:4090`（支持 `--addr` 覆盖）
- SSE 解析：按行读 `data: {...}`，反序列化 JSON
- `EventAsk` 处理：展示原因 → 读 stdin → POST `/ask/reply` → 继续消费 SSE
- CLI 不再需要 `voyage.Create/Resume/List` 等直接调用，全部通过 HTTP
- 旧 `helm.RunLoop` 可保留至 CLI 迁完，再整体移除 `loop.go` / `checkpoint.go`

### 建议实现顺序

1. 写 SSE 客户端解析器
2. HTTP 请求封装（create / list / resume / prompt / ask-reply）
3. 事件消费循环
4. 清理旧代码

### 手动测试

启动 server：
```bash
./argo-server.exe
```

curl 测试：
```bash
# 创建
curl -s -X POST http://localhost:4090/session/create -d '{"cwd":"./"}'

# 列表
curl -s http://localhost:4090/session/list | python -m json.tool

# 发送消息（SSE 流）
curl -N -X POST http://localhost:4090/session/prompt -d '{"sessionID":"sess_xxx","message":"hello"}'
```
