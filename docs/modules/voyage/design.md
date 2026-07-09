# voyage

## 概述

voyage 是 Argo 的会话聚合层。Session 结构体聚合 Vault（纯存储）+ brig.Guard（权限门控）+ SessionInfo（元数据），为 helm.Run 提供完整运行时上下文。负责会话 CRUD（创建/恢复/列表/重命名/删除）和审批记录持久化。

voyage 依赖 vault、brig，不依赖 helm/sail/deck。

## 核心概念

| 概念 | 说明 |
|------|------|
| Session | 运行时会话上下文，嵌入 SessionInfo，持有 Vault + Guard |
| SessionInfo | 会话元数据：ID、WorkingDir、CreatedAt、LastActiveAt、MessageCount、Name、Tokens |
| CreateSession | 生成 ID → 建目录 → 写 session.json → 组装 Session |
| ResumeSession | 从 session.json 恢复元数据 → 从 approvals.json 恢复审批记录 |
| Append | 追加 records 到 transcript.jsonl，并更新 SessionInfo 动态字段（LastActiveAt、MessageCount、Tokens） |
| approvals.json | 用户审批记录持久化文件（brig.ApprovalEntry 的 JSON 数组） |
| session.json | SessionInfo 的 JSON 持久化文件 |

## 会话目录结构

```
~/.argo/sessions/{sessionID}/
  ├── session.json        ← SessionInfo 元数据
  ├── transcript.jsonl    ← 对话记录（Vault）
  ├── approvals.json      ← 用户审批记录（Guard Snapshot/Restore）
  └── state.json          ← LoopState 控制流（server 包管理）
```

## Session 结构

Session 嵌入 SessionInfo，ID/WorkingDir/CreatedAt 等字段自动提升。Vault 绑定 session 目录实现纯存储层隔离，Guard 绑定 WorkingDir 实现审批指纹。

## 行为合约

### CreateSession / ResumeSession

| 操作 | 行为 |
|------|------|
| CreateSession(baseDir, cwd) | 生成 ID → Mkdir → 写 session.json → 组装 Session（含 Vault + Guard） |
| ResumeSession(baseDir, id) | 读 session.json → 组装 Session → 读 approvals.json → Guard.Restore |

### Append

- 写 record 到 transcript.jsonl（委托 Vault.Append）
- 更新 LastActiveAt = now
- 累加 MessageCount、InputTokens、OutputTokens
- 写 session.json 持久化更新后的 SessionInfo

### SaveApprovals

- 调用 Guard.Snapshot() 获取审批快照
- 序列化为 JSON 写入 approvals.json
- 在 streamAndSave 事件流结束后自动调用

### 错误处理

- approvals.json 不存在（新会话）视为正常，不报错
- session.json 损坏时返回错误
- Vault 写入失败时日志告警，不阻塞流程
