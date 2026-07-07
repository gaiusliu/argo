# 007：vault MVP

**日期**：2026-07-07 ~
**状态**：🚧 进行中
**涉及模块**：vault（新建）、helm（集成待定）

## 目标

实现 vault 模块 MVP——Argo 的统一存储层。提供 session transcript 读写、框架级记忆存取（profile.md + AGENTS.md）、session CRUD。MVP 阶段使用文件系统后端，接口层面预留后端切换能力。

## 范围

### 包含

- Vault 接口定义：Session CRUD + Transcript Append/Load + Recall + WriteProfile
- FileVault 实现（文件系统后端：JSONL + Markdown）
- Record 类型定义（user / llm / tool，3 种）
- Message ↔ Record 单向转换（vault 内部，helm 不感知 Record 结构）
- Session 管理（创建 / 列表 / 查看 / 删除）

### 不含

- SQLite 或其他非文件系统后端
- `Store(key, value, metadata)` / `Retrieve(query, limit)` 通用 key-value 接口（留待后续）
- helm.RunLoop 集成（等 vault 包稳定后单独迭代）
- profile.md 写入时的 LLM 合并/去重逻辑（LLM 自行处理，vault 只做覆盖写）
- profile record 写入 transcript（Append 阶段暂跳过，后续补）

## 公共设计约束

- **文件方案**：transcript.jsonl 存储对话记录，profile.md + AGENTS.md 存储框架级记忆。Vault 接口不暴露底层存储细节，为未来后端切换留口子
- **vault 依赖 knot**：`Load()` 返回 `[]knot.Message`（而非 `[]Record`），因为 what.md 明确要求映射回 Message。vault 是唯一依赖 knot 的存储层模块
- **写入失败不阻塞**：`Append` 写入失败只记 `slog.Error`，不影响主循环
- **Session 惰性创建**：CLI 启动时不立即创建 session，首条用户消息发出时才 `CreateSession()`
- **单向转换**：vault 内部提供 `MessageToRecord` 和 `RecordsToMessages` 两个单向函数，TurnState 不直接对应 Record
- **文件锁**：profile.md 为跨会话共享文件，使用 `os.Mkdir` 目录锁方案保护并发写入（对标 opencode 的 Flock 实现）。零外部依赖，全平台原子操作。[DEC-034](../../decisions.md#dec-034vault-使用-mkdir-目录锁保护共享文件)
- **规格依据**：[vault/what.md](../modules/vault/what.md)（行为规约）、[vault/transcript.md](../modules/vault/transcript.md)（record 字段定义）、[what.md](../what.md) Memory 协议

## 整体开发状态

| 功能 | 状态 | 子文档 | 关联测试 |
|------|------|--------|---------|
| Vault 接口 + Record 类型定义 | ✅ 完成 | — | — |
| FileVault 实现 | ⏳ 待开始 | — | — |
| Session CRUD | ⏳ 待开始 | — | — |
| Message ↔ Record 转换 | ⏳ 待开始 | — | — |

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-07-07 | Vault 接口 + Record 类型定义完成；新增 DEC-034（mkdir 目录锁） |
| 2026-07-07 | 迭代开始 |
