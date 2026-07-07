# Handoff — vault 接入 RunLoop + CLI

## 上回会话成果

vault 模块 MVP 代码完成，8/10 会话内容见 git log `(当前未提交)`。

### CWD 重构（DEC-028 收尾）

`WorkingDir` 从 `knot.Config` 移除，改为 `brig.NewEngine(workingDir)` 构造函数注入。CWD 由 cli 入口捕获。

改动文件：[src/knot/types.go](src/knot/types.go)、[src/knot/config.go](src/knot/config.go)、[src/brig/brig.go](src/brig/brig.go)、[src/cli/main.go](src/cli/main.go)

### vault 模块 MVP（`src/vault/`）

| 文件 | 内容 |
|------|------|
| [vault.go](src/vault/vault.go) | `Vault` 接口（6 方法）+ `SessionInfo` + `SessionTokens` |
| [record.go](src/vault/record.go) | `Record`（user/llm/tool）+ `ToolVerdict`（5 态）+ `ToolCallRecord` |
| [file_vault.go](src/vault/file_vault.go) | `FileVault` 实现全部 6 方法（CreateSession、ListSessions、GetSession、DeleteSession、Append、Load） |
| [flock.go](src/vault/flock.go) | `LockDir`/`UnlockDir`（mkdir 目录锁 + 指数退避，DEC-034） |

### 设计决策

- **Record 类型精简**：MVP 只做 user/llm/tool，system/compact/profile 推迟
- **ToolVerdict 5 态追踪**：Argo 独有审计需求，三个参考项目无此能力
- **AGENTS.md 不经过 vault**：`helm.buildSystemPrompt` 直接读，与 pi/opencode/kilocode 一致
- **PROFILE.md 推迟**：Read/Write 暂不暴露，留待 deck 工具
- **目录锁**：`os.Mkdir` 方案（对标 opencode Flock），零外部依赖
- **Session 惰性创建**：CLI 启动不立即创建，首条消息时 `CreateSession()`

### 文档

- [docs/modules/vault/design.md](docs/modules/vault/design.md) — 模块设计（基于实际代码）
- [docs/modules/vault/transcript.md](docs/modules/vault/transcript.md) — Record 字段规格（与代码一致）
- [docs/decisions.md](docs/decisions.md) — DEC-034（mkdir 目录锁）
- [docs/iterations/007-vault-mvp/main.md](docs/iterations/007-vault-mvp/main.md) — 迭代文档

## 当前状态

```
src/vault/
├── vault.go          ← Vault 接口（6 方法）+ SessionInfo/SessionTokens
├── record.go         ← Record/RecordType/ToolVerdict/ToolCallRecord
├── file_vault.go     ← FileVault 完整实现
└── flock.go          ← LockDir/UnlockDir
```

vault 包独立可编译，零外部依赖。尚未接入任何模块。

## 下一步：vault 接入 RunLoop + CLI

### 涉及文件

- **`src/cli/main.go`** — 创建 FileVault 实例，注入 helm，集成会话生命周期
- **`src/helm/loop.go`** — RunLoop 每轮结束后调用 vault.Append()
- **`src/helm/types.go`** — TurnState 加 SessionID 字段
- 可能需要新增 **Message → Record 转换**的辅助代码（放在 vault 包或新文件）

### 接入要点

1. **CLI 启动时**：创建 `fileVault := vault.NewFileVault(helm.ArgoDir())`，展示最近会话列表（`fileVault.ListSessions()`），默认新会话状态
2. **首条用户消息时**：`vault.CreateSession(cwd)` → 获取 sessionID，存入 `TurnState.SessionID`
3. **RunLoop 每轮结束后**：将本轮新增的 `[]knot.Message` 转换为 `[]vault.Record`，调用 `vault.Append(sessionID, records)`。写入失败记日志不阻塞
4. **Message → Record 转换**：用户消息 → user record；LLM 回复 → llm record（含 model、tool_calls）；工具结果 → tool record（含 verdict、tool_call_id、status）
5. **brig verdict 传递**：RunLoop 中 Evaluate 返回的 verdict 需要填入 tool record。当前 brig 返回 `(Verdict, string)`，调用方已持有判决结果
6. **TurnState.SessionID**：[src/helm/types.go:25](src/helm/types.go#L25) 加字段

### 相关模块状态

| 模块 | 状态 |
|------|------|
| vault | ✅ MVP 代码完成，待集成 |
| helm | 🚧 待加 vault 写入点 |
| cli | 🚧 待加 vault 初始化 + 会话列表 |
| brig | ✅ 已有 ToolVerdict 对应的 brig.Verdict（Allow/Ask/Deny），需映射到 ToolVerdict 5 态 |

### 编译运行

```bash
cd c:/Users/Gaiusliu/Desktop/code/argo
go build -o argo.exe ./src/cli/
./argo.exe
```
