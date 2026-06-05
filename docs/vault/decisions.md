# vault 技术决策记录

---

## DEC-V01：对话记录（transcript）由 vault 管理

**日期**：2026-06-05
**状态**：已决定

**决策**：对话历史持久化是 vault 的职责（存储模块）。helm 每轮结束后调用 vault 写入新增消息，reef 和 LLM 通过 Read 工具按需读取。

**格式**：JSONL 同步追加，最简 4 字段（role/content/tool_calls/tool_call_id/timestamp）。见 [transcript.md](transcript.md)。

**拒绝**：异步写入——需管理 buffer/flush 时机/完整性检查，MVP 不值得。

**参考**：pi 的 JSONL 追加模式，O(1) 磁盘操作。
