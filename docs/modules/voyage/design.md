# voyage

## 概述

voyage 是 Argo 的会话聚合层。`Voyage` 结构体聚合 Info（元数据）+ Vault（纯存储）+ Gate（权限门控）+ Tale（对话历史内存态）+ 状态机控制流字段，是 helm 运行时的唯一上下文载体。

voyage 依赖 vault、brig、knot、tale，不依赖 helm/sail/deck。

## 核心概念

| 概念 | 说明 |
|------|------|
| `Voyage` | 会话运行时上下文——嵌入 Info 自动提升元数据字段，持有 Vault + Gate + Tale + 状态机控制流。是具体 struct，非接口 |
| `Info` | 会话元数据：ID、ArgoDir、WorkingDir、CreatedAt、LastActiveAt、Name |
| `New` | 创建新会话：生成 ID → Mkdir → 写 session.json → 组装 Voyage（含默认 Vault + Gate）→ 构造空 Tale |
| `Resume` | 从 session.json 恢复完整 Voyage：读元数据 → 组装 Vault + Gate → 加载消息构造 Tale → 恢复审批记录 |
| `Option` | 函数选项 `func(*Voyage)`，用于注入 Vault / Gate 依赖。`WithVault` / `WithGate` 覆盖默认实现 |
| `Tale` | 对话历史内存态——封装消息列表 + 持久化游标 + system prompt。提供 `AppendMessages` / `BuildRequest` / `Unsaved` / `MarkSaved` 等操作 |
| `AppendMessages` | 将消息差量转为 vault.Record 写入 transcript.jsonl，自动 `MarkSaved` + 更新 `LastActiveAt` |
| `SaveState` | 将控制流状态（Phase/Model/ToolUses 等）序列化到 state.json。调用前从 Tale 同步 Saved 游标 |
| `LoadState` | 从 state.json 恢复控制流状态。恢复后将 PendingMessages 回填到 Tale，重建断点前的完整对话历史 |
| `DeleteState` | 删除 state.json——Asking 阶段终止时使用，下次 Resume 自然回到断点前干净状态 |
| `SaveApprovals` | 独立函数，将 Gate 中的用户审批快照持久化到 approvals.json |
| `SaveInfo` | 持久化 session.json（LastActiveAt 等元数据更新） |

## 会话目录结构

```
~/.argo/sessions/{sessionID}/
  ├── session.json        ← Info 元数据（voyage 管理）
  ├── transcript.jsonl    ← 对话记录（Vault 管理）
  ├── approvals.json      ← 用户审批记录（Gate Snapshot/Restore）
  └── state.json          ← 控制流状态（helm 断点持久化）
```

## 行为合约

### New / Resume — 会话生命周期

| 操作 | 签名 | 行为 |
|------|------|------|
| `New` | `(argoDir, workingDir string, opts ...Option) (*Voyage, error)` | 生成 ID → Mkdir → 写 session.json → 默认 Vault + Gate → 构造空 Tale |
| `Resume` | `(id string, opts ...Option) (*Voyage, error)` | 读 session.json → 组装 Vault + Gate → Vault.Load 构造 Tale → loadApprovals → Gate.Restore |

默认依赖：Vault = `vault.NewFileVault(sessionDir)`，Gate = `brig.NewWarden(workingDir, &brig.BuiltinRuleLoader{})`。通过 `WithVault` / `WithGate` Option 可在测试中注入 mock。

### AppendMessages — 差量持久化

```
vault.MessagesToRecords(delta, toolUses, model) → Vault.Append(records)
→ tale.MarkSaved()
→ LastActiveAt = time.Now()
→ saveInfo(session.json)
```

### SaveState / LoadState — 断点控制流

**SaveState**（Ask/Done 阶段断点时调用）：
```
tale.Saved() → Voyage.Saved（同步游标）
→ json.MarshalIndent(Voyage) → state.json
```

**LoadState**（Ask 回复路径恢复时调用）：
```
state.json → json.Unmarshal → Voyage
→ tale.SetSaved(Saved)（恢复游标）
→ tale.AppendMessages(PendingMessages)（回填 Asking 阶段的 user + assistant 消息）
→ PendingMessages = nil
```

### 门控委托

`Voyage.Evaluate(tu)` / `Approve(tu)` / `UserApprovals()` 直接委托给内部 Gate，不添加额外逻辑。

### CRUD 操作

| 操作 | 签名 | 行为 |
|------|------|------|
| `List` | `(argoDir string) ([]Info, error)` | 遍历 sessions 目录，按 LastActiveAt 倒序，损坏的 session 跳过 |
| `RenameByID` | `(baseDir, id, name string) error` | 读 session.json → 改 Name → 写回 |
| `Delete` | `(baseDir, id string) error` | `os.RemoveAll(sessionDir)` |
| `DeleteState` | `(id string) error` | 删除 state.json（不存在视为正常） |

### 错误处理

- session.json 损坏 → 返回 error
- approvals.json 不存在（新会话）→ 视为正常
- Vault 写入失败 → 返回 error，由上层（helm.checkpoint）slog.Error 记录
- List 遍历时单个 session 损坏 → slog.Error 记录后跳过，不影响其他

### 依赖边界

voyage 依赖 vault、brig、knot、tale，不依赖 helm/sail/deck。`Voyage` 是具体 struct，不是接口——变化性通过内部 `Vault` / `Gate` 接口注入覆盖。
