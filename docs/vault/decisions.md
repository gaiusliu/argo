# vault 技术决策记录

---

## DEC-V01：对话记录（transcript）由 vault 管理

**日期**：2026-06-05
**状态**：已决定

**决策**：对话历史持久化是 vault 的职责。helm 每轮结束后调用 vault.Append() 写入新增消息。JSONL 同步追加，4 字段（role/content/tool_calls/tool_call_id/timestamp）。

**格式**：见 [transcript.md](transcript.md)。

---

## DEC-V02：长期记忆拆分为 profile.md + rules.md

**日期**：2026-06-08
**状态**：已决定

**决策**：长期记忆使用两个文件而非一个。

| 文件 | 记什么 | 谁来写 | 淘汰策略 |
|------|------|------|------|
| `profile.md` | 用户画像、偏好、风格 | LLM 为主 | 5K 预算，LLM 写入时自行合并/丢弃 |
| `rules.md` | 硬性约束、规则 | 用户 + LLM 共建 | 用户手写永不淘汰，LLM 追加可由用户审核 |

两文件在会话开始时通过 vault.Recall() 注入 system prompt。

**拒绝**：单文件 MEMORY.md + 类型标签——OpenClaw 的模式在少量记忆时够用，但 budget 控制时不同类型的淘汰策略不同（偏好可自动淘汰，规则不可），单文件无法区分。

---

## DEC-V03：业务记忆归 SKILL + 工具

**日期**：2026-06-08
**状态**：已决定

**决策**：vault 只负责 Agent 框架级的共享状态（profile + rules + transcript）。业务记忆（Vug 的技术概念卡片、叩问的学生混淆矩阵）由各业务的 SKILL + 专用工具自行管理。

**原因**：业务记忆的结构和检索方式完全由业务场景决定——概念卡片需要分类树和双向链接，混淆矩阵需要按学生 ID 和知识点查询。框架级记忆系统不可能设计一个通用的数据结构满足所有业务。

**拒绝**：设计通用的事实提取引擎（如 Cortex 的 Extract 流水线）——覆盖所有业务类型会导致 schema 膨胀和提取质量下降。

---

## DEC-V04：热插拔后端通过注册表

**日期**：2026-06-08
**状态**：已决定

**决策**：vault 提供 Memory 接口 + BackendRegistry 注册表。默认实现为文件系统（FileVault）。Agent 配置中指定后端名称，启动时加载。切换 = 改配置 + 重启，不做数据迁移。

**参考**：Hermes Agent 的 8 个 MemoryProvider 插件体系。

**拒绝**：运行时热切换——切换记忆系统意味着旧系统数据的组织方式已不适合新场景，旧记忆大概率不想搬到新系统中。是新 Agent 的定义决策，不是运行时的操作。

---

## DEC-V05：reef 不依赖 vault

**日期**：2026-06-08
**状态**：已决定

**决策**：reef 压缩时从 helm 的内存中读取消息，不从 vault 读 transcript 文件。数据流是单向的：helm → reef → helm → vault。reef 和 vault 之间没有调用关系。
