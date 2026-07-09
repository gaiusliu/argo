# Handoff — 2026-07-09

## 本次会话成果

### 1. CLI HTTP 客户端改造

CLI 不再直接调用 helm/voyage/brig，改为通过 HTTP + SSE 与 argo-server 通信。

**新增文件**：`src/cli/client.go`、`crud.go`、`sse.go`、`convert.go`、`server.go`、`ui.go`、`session_pick.go`、`ask.go`

**关键设计**（详见 [DEC-036](docs/decisions.md#dec-036)）：
- EventAsk 时 `cancel()`（`resp.Body.Close()` 包装）释放旧连接，`events` 变量替换为新 channel
- 单层 `for` 循环消费，不递归不嵌套
- CLI 自动探测并启动 argo-server 子进程（`startServer`）

### 2. Server 类型导出

`PromptRequest`、`EventJSON`、`SessionInfoJSON`、`ToolUseJSON` 等全部导出，CLI 直接 import server 包复用，不复制类型。

### 3. handlePrompt 合并

`PromptRequest` 新增 `AskID`/`Allow` 字段，`handlePrompt` 统一处理新消息和 Ask 回复。删除 `/ask/reply` 路由。

### 4. Vault 成为对话历史唯一数据源（DEC-037）

详见 [DEC-037](docs/decisions.md#dec-037)。

**LoopState 瘦身**：

| 删除 | 保留 |
|------|------|
| `Messages`, `MsgCount`, `ToolMeta`, `Usage`, `AskReply` | `Phase`, `TurnCount`, `ModelName`, `ToolUses`, `ToolUseIndex`, `AskReason`, `AskID` |

**写入时机改为逐条立刻写**：user → handlePrompt 入口，assistant → runPhaseLLM 结束，tool → 每条执行完。

**对话历史**：改为 `Run()` 的 `*[]knot.Message` 入参，goroutine 内 Phase 间通过指针共享。

### 5. 状态机暂停/恢复（DEC-035）

详见 [DEC-035](docs/decisions.md#dec-035)。server 无状态——EventAsk 时 goroutine 退出，LoopState 存 state.json，下次请求加载继续。

### 6. 模块设计文档

| 文档 | 操作 |
|------|------|
| `docs/modules/server/design.md` | 新建 |
| `docs/modules/voyage/design.md` | 新建 |
| `docs/modules/helm/design.md` | 重写（状态机模型） |
| `docs/modules/brig/design.md` | 更新（Guard 接口） |
| `docs/modules/vault/design.md` | 更新（Session CRUD 迁出） |
| `docs/modules/knot/design.md` | 更新（依赖图） |
| `docs/modules/sail/design.md` | 更新（超时位置） |

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

CLI 通过 HTTP + SSE 与 argo-server 通信，不直接依赖 helm/voyage。

## 遗留问题

### 待删除的旧代码

- `src/helm/loop.go` — 旧 `RunLoop` 函数，已无外部调用
- `src/helm/checkpoint.go` — 旧 `Checkpoint` 结构体，配套 loop.go
- `src/vault/vault.go` — 旧 Vault 接口（含 Session CRUD），已迁到 voyage
- `src/vault/file_vault.go` — 旧 FileVault 实现，已被精简版本替代

### 待处理的 TODO（详见 [迭代文档](docs/iterations/008-argo-separation/main.md#小-todo)）

- 跨平台 server 二进制名适配
- HTTP Client 超时设置（`context.WithTimeout`）
- 测试代码 `test_sse_*.go` 清理或归档

### 已知问题

- 配置文件 `config.json` 的 log level 当前设为 `info`（方便调试）
- `sail.Chat` 的 modelName 传空字符串时会走默认模型

## 下一步计划

1. **找出冗余代码，删掉** — 清理上述待删除文件 + 未使用的 import/函数
2. **code-review 整个项目** — 处理严重问题（性能、并发安全、错误处理）
3. **设计+开发 reef** — 上下文压缩模块

## 建议技能

- `/code-review` — 代码审查
- `/simplify` — 简化/清理冗余代码
