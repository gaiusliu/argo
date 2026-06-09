# vault 开发任务

## 1. 核心类型定义
- [ ] ▶ 1.1 定义 Memory 接口（Append / Recall / Store）、MemoryEntry、RecallResult、Config
- [ ] 1.2 实现包级 Register / Load 函数（内部 unexported map 管理 Memory 实现）

## 2. FileVault 文件系统实现
- [ ] 2.1 实现 FileVault 骨架（NewFileVault + 目录创建 + Memory 接口占位）
- [ ] 2.2 实现 Append — transcript.jsonl 同步追加写入（JSONL 格式：role / content / tool_calls / timestamp）
- [ ] 2.3 实现 Store — profile.md / rules.md 追加写入 + badgetCheck 预算控制
- [ ] 2.4 实现 Recall — 读取 profile.md + rules.md 全文，文件不存在时返回空值

## 3. 会话目录管理
- [ ] 3.1 实现会话目录创建（~/.argo/sessions/{session_id}/）与目录结构初始化

## 4. 测试
- [ ] 4.1 FileVault Append 追加写入 + 并发安全测试
- [ ] 4.2 FileVault Store / Recall + 预算边界测试
- [ ] 4.3 Register / Load 后端切换测试
- [ ] 4.4 文件不存在/空目录容错测试
