# voyage

## 概述

voyage 是会话聚合层，将 Vault（存储）、Gate（门控）和 SessionInfo（元数据）聚合为一个 `Session` 运行时上下文。对外通过 `Voyage` 接口暴露，供 helm 状态机使用。会话的创建、恢复、列表、改名、删除由包级函数提供。

## 核心概念

| 概念 | 说明 |
|------|------|
| `Voyage` | 会话运行时接口，聚合 Save / Load / Append / Evaluate / Approve 等能力 |
| `Session` | Voyage 的具体实现，嵌入 SessionInfo + Vault + Gate |
| `SessionInfo` | 会话元数据：ID / ArgoDir / WorkingDir / 创建时间 / 最后活跃时间 / 名称 |
| `SessionOption` | 函数选项（`func(*Session)`），用于注入 Vault / Gate，不传时使用默认实现 |
| `WithVault` / `WithGate` | 覆盖默认 FileVault / Warden 的 Option 构造器 |

## 行为合约

### 会话创建

`NewSession(argoDir, workingDir, ...opts)` 的行为保证：

- 生成唯一 session ID，创建 `~/.argo/sessions/{id}/` 目录
- 写入 `session.json` 元数据
- 默认绑定 `FileVault` + `Warden(&BuiltinRuleLoader{})`
- `opts` 可覆盖 Vault / Gate

### 会话恢复

`ResumeSession(argoDir, id, ...opts)` 的行为保证：

- 从 `session.json` 读取元数据
- 重建 Vault + Gate（可通过 opts 覆盖）
- 从 `approvals.json` 恢复用户审批记录，注入 Gate

### 对话记录持久化

`Session.Append(records)` 的行为保证：

- 追加写入 `transcript.jsonl`（增量，不重复）
- 自动更新 `LastActiveAt` 时间戳
- 同步写回 `session.json`

### 门控委托

- `Evaluate` / `Approve` / `UserApprovals` 均委托给内部 Gate，voyage 本身不参与裁决逻辑
