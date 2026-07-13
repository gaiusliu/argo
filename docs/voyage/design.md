# voyage

## 概述

Voyage 是 Argo 会话运行时的唯一聚合体，将元数据、存储、门控、对话历史和控制流状态统一在一个 struct 中。无接口——`*Voyage` 直接使用，变化性由内部 Vault/Gate 接口注入覆盖。

## 结构

```
Voyage
├── Info（嵌入）          ← 元数据：ID / WorkingDir / CreatedAt / …
├── Vault（接口）          ← 纯存储：FileVault 或测试替身
├── Gate（接口）           ← 门控：Warden 或测试替身
├── tale *tale.Tale        ← 对话历史内存态：消息列表 + 持久化游标
├── Phase int              ← 状态机当前阶段
├── Model string           ← 当前模型名
├── ToolUses []ToolUse      ← 本回合待执行的工具调用（含裁决/结果）
├── PendingMessages []Message ← Asking 阶段暂存未落盘消息（user + assistant(tool_calls)）
└── Saved int              ← 持久化游标桥梁（SaveState/LoadState 时与 Tale 同步）
```

## 核心概念

| 概念 | 说明 |
|------|------|
| `Voyage` | 单一聚合 struct，会话运行时的自足上下文 |
| `Info` | 会话元数据：ID / ArgoDir / WorkingDir / CreatedAt / LastActiveAt / Name |
| `Tale` | 对话历史封装（`src/tale/`），管理消息追加、请求构建、游标一致性 |
| `Option` | 函数选项 `func(*Voyage)`，用于 `WithVault` / `WithGate` |

## 生命周期

### 创建

`New(argoDir, workingDir, ...opts) (*Voyage, error)`：

- 生成唯一 session ID，创建 `~/.argo/sessions/{id}/` 目录
- 写入 `session.json`（Info 元数据）
- 绑定 `FileVault` + `Warden(&BuiltinRuleLoader{})`
- 从 Vault 加载消息构造 Tale（新会话为空列表）
- `opts` 可覆盖 Vault / Gate

### 恢复

`Resume(argoDir, id, ...opts) (*Voyage, error)`：

- 从 `session.json` 读取 Info
- 重建 Vault + Gate（可通过 opts 覆盖）
- 从 Vault 加载消息构造 Tale
- 恢复用户审批记录到 Gate

`ResumeFromArgoDir(id, ...opts) (*Voyage, error)`：从默认 Argo 目录恢复，handlePrompt 入口用。

### 控制流持久化

`SaveState()`：序列化前从 Tale 同步游标（`v.Saved = v.tale.Saved()`），将完整 Voyage 序列化写入 `state.json`。Ask/Done 断点时调用。Asking 阶段额外包含 `PendingMessages`（尚未落盘的 user + assistant(tool_calls)），供 LoadState 恢复。

`LoadState()`：从 `state.json` 反序列化 Voyage，恢复游标到 Tale（`v.tale.SetSaved(v.Saved)`），并将 `PendingMessages` 通过 `tale.AppendMessages()` 恢复到对话历史中。Ask 回复路径用。

`DeleteState(id)`：删除 `state.json`。中断时调用——PendingMessages 随文件一起丢失，下次 Resume 自然回到上一 checkpoint。

### 中断回滚

```
中断（Ctrl+C 或 /interrupt）:
  vault:      [history...]              ← 干净
  state.json: {Pendings=[user,工具调用]} ← 删掉
  → Resume 从 vault 加载 → 回到断点前
```

核心原则：中断时不调 checkpoint，不写 vault。Asking 阶段的 PendingMessages 只存 state.json，删文件即回滚。

### 对话记录持久化

`AppendMessages(delta, toolUses, model)`：内部调用 `vault.MessagesToRecords` 转换 + `Vault.Append` 写盘 + `tale.MarkSaved()` 更新游标 + 更新 `LastActiveAt` + 写回 `session.json`。调用者只需传消息差量和工具列表，不感知 Record 转换和游标管理。

## 门控委托

`Evaluate` / `Approve` / `UserApprovals` 全部委托给内部 Gate，Voyage 自身不参与裁决逻辑。

## 包级函数

| 函数 | 说明 |
|------|------|
| `New(argoDir, workingDir, ...opts)` | 创建新会话 |
| `Resume(argoDir, id, ...opts)` | 从磁盘恢复 |
| `ResumeFromArgoDir(id, ...opts)` | 从默认 Argo 目录恢复 |
| `List(argoDir)` | 列出所有会话（按活跃时间倒序） |
| `Delete(baseDir, id)` | 物理删除会话目录 |
| `RenameByID(baseDir, id, name)` | 修改会话名称 |
| `SaveApprovals(v)` | 持久化审批记录 |

## 设计决策

- **无接口**：Voyage 无多实现需求，变化性由内部 Vault/Gate 覆盖（[DEC-038](/docs/decisions.md#dec-038voyage-单一聚合体去接口--session-改名--helmstate-合并--tale-归属)）
- **Tale 归属**：归 Voyage 而非 HelmState。Voyage 负责会话记录管理，Tale 是其内存态（[DEC-038](/docs/decisions.md#dec-038voyage-单一聚合体去接口--session-改名--helmstate-合并--tale-归属)）
- **HelmState 并入**：生命周期一致，分开只增复杂度（[DEC-038](/docs/decisions.md#dec-038voyage-单一聚合体去接口--session-改名--helmstate-合并--tale-归属)）
- **AppendMessages 内化**：`vault.MessagesToRecords` 调用收敛于 Voyage，helm 不再 import vault
- **SaveState/LoadState**：游标同步封装在 Voyage 内部，调用者不传来传去
- **Asking 延迟持久化**：PhaseAsking 不写 vault，PendingMessages 暂存 state.json，中断时删除即可回滚（[DEC-040](/docs/decisions.md#dec-040asking-延迟持久化phaseasking-不写-vaultpendingmessages-暂存-statejson)）
- **DeleteState**：删除 state.json，用于 Asking 阶段终止和中断回滚
