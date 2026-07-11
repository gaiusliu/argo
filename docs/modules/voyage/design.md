# voyage

## 概述

voyage 是 Argo 的会话聚合层。`Session` 结构体聚合 Vault（纯存储）+ brig.Guard（权限门控）+ SessionInfo（元数据），为 `helm.Run` 提供 `Voyage` 接口。负责会话 CRUD（创建/恢复/列表/重命名/删除）和审批记录持久化。

voyage 依赖 vault、brig、knot，不依赖 helm/sail/deck。

## 核心概念

| 概念 | 说明 |
|------|------|
| Voyage | 会话能力接口，定义 Load/Append/Evaluate/Approve/UserApprovals/Save/ID/Rename/WorkingDir 9 个方法。`helm.Run` 依赖此接口而非具体 Session |
| Session | 运行时会话上下文，嵌入 SessionInfo，持有 Vault + Guard，实现 Voyage 接口 |
| SessionInfo | 会话元数据：ID、ArgoDir、WorkingDir、CreatedAt、LastActiveAt、Name |
| NewSession | 生成 ID → 建目录 → 写 session.json → 组装 Session（含 Vault + Guard） |
| ResumeSession | 从 session.json 恢复完整 Session，同时恢复已持久化的审批记录 |
| ResumeVoyage | 与 ResumeSession 逻辑相同，返回 Voyage 接口（供 server 使用） |
| SaveApprovals | 将 Guard 中的用户审批快照持久化到 approvals.json，由 helm.checkpoint 调用 |

## 会话目录结构

```
~/.argo/sessions/{sessionID}/
  ├── session.json        ← SessionInfo 元数据（voyage 管理）
  ├── transcript.jsonl    ← 对话记录（Vault 管理）
  ├── approvals.json      ← 用户审批记录（Guard Snapshot/Restore）
  └── state.json          ← HelmState 控制流（helm 管理）
```

## Voyage 接口

```go
type Voyage interface {
    Save() error
    Load() ([]knot.Message, error)
    Rename(name string)
    ID() string
    Evaluate(knot.ToolUse) (int, string)
    WorkingDir() string
    Append([]vault.Record) error
    UserApprovals() []brig.ApprovalEntry
    Approve(tu knot.ToolUse)
}
```

`helm.Run` 通过 Voyage 接口与 session 交互，不直接依赖 Session 结构体，方便测试和替换。

## Session 结构

Session 嵌入 SessionInfo，ID/WorkingDir/CreatedAt 等字段自动提升。Vault 绑定 session 目录实现纯存储层隔离，Guard 绑定 WorkingDir 实现审批指纹（`brig.NewFileGuard(workingDir)`）。

```go
type Session struct {
    SessionInfo
    Vault vault.Vault
    Guard brig.Guard
}
```

## 行为合约

### NewSession / ResumeSession / ResumeVoyage

| 操作 | 签名 | 行为 |
|------|------|------|
| NewSession | `(argoDir, workingDir string) (*Session, error)` | 生成 ID → Mkdir → 写 session.json → 组装 Session |
| ResumeSession | `(argoDir, id string) (*Session, error)` | 读 session.json → 组装 Session → 读 approvals.json → Guard.Restore |
| ResumeVoyage | `(id string) (Voyage, error)` | 与 ResumeSession 相同，返回 Voyage 接口（argoDir 从 knot.ArgoDir() 获取） |

### Append

```
session.Vault.Append(records)     ← 写 transcript.jsonl
session.LastActiveAt = time.Now()  ← 更新活跃时间
saveSessionInfo(...)              ← 写 session.json
```

与旧版差异：不再累加 MessageCount/InputTokens/OutputTokens（SessionInfo 中无这些字段）。

### SaveApprovals

- 接收 Voyage 接口，`v.UserApprovals()` 获取审批快照
- 序列化为 JSON 写入 approvals.json
- 由 `helm.checkpoint()` 在 Asking / Done 阶段调用，不在 streamAndSave 中

### 错误处理

- approvals.json 不存在（新会话）视为正常，不报错
- session.json 损坏时返回错误
- Vault 写入失败时返回 error，由上层（helm.checkpoint）slog.Error 记录

## 关键文件

| 文件 | 职责 |
|------|------|
| `voyage.go` | Voyage 接口 + Session 结构体 + SessionInfo + NewSession + ResumeSession + ResumeVoyage + 全部 CRUD 操作 + SaveApprovals + Append |
