# vault 开发任务

## 1. 核心类型定义（无需测试）
- [ ] ▶ 1.1 定义 Memory 接口（Append / Load / Recall / Store）、MemoryEntry、RecallResult、Config

## 2. FileVault 实现（需测试）

**测试角度**：追加正确性、并发安全、重建 messages 完整性
**测试方式**：单元测试（临时目录 + 真实文件系统）
**测试策略**：每个操作验证写入内容、JSONL 格式、文件不存在容错

- [ ] 2.1 实现 NewFileVault — 会话目录创建（~/.argo/sessions/{session_id}/）
- [ ] 2.2 实现 Append — transcript.jsonl 同步追加写入（6 种 type：user/llm/tool/system/compact/profile），每条含 seq（session 级自增）
- [ ] 2.3 实现 Load — 读取 transcript.jsonl，过滤出 user/llm/tool 三种 type 重建 []Message
- [ ] 2.4 Append 测试：单条写入 + 写入后即时读取一致性 / 多条并发写入安全性
- [ ] 2.5 Load 测试：过滤出 user/llm/tool / system 和 compact 和 profile 不被加载 / 空文件 / 文件不存在

## 3. 长期记忆存储（需测试）

**测试角度**：追加写入、预算控制、AGENTS.md 只读
**测试方式**：单元测试（临时目录 + 真实文件）
**测试策略**：覆盖正常写入、预算边界、文件不存在容错

- [ ] 3.1 实现 Store — profile.md 追加写入 + budgetCheck（5,000 字符预算）。仅写 profile.md，不写 AGENTS.md
- [ ] 3.2 实现 Recall — 读取 profile.md + AGENTS.md 全文，文件不存在时返回空值
- [ ] 3.3 Store 测试：正常写入 / 追加多条 / budgetCheck 超限（= 5000 / = 5000+1 / 远大于 5000）
- [ ] 3.4 Recall 测试：正常加载 / profile.md 不存在 / AGENTS.md 不存在 / 两者都不存在

## 4. 后端注册（需测试）

**测试角度**：注册、切换、回退
**测试方式**：单元测试（Mock Memory 实现）
**测试策略**：覆盖正常注册/加载、同名覆盖、未注册回退

- [ ] 4.1 实现包级 Register / Load 函数（内部 unexported map 管理 Memory 实现）
- [ ] 4.2 Register 正常注册 + 同名覆盖测试
- [ ] 4.3 Load 命中 / 未命中（回退到默认 FileVault）测试
