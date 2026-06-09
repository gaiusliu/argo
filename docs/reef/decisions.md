# reef 技术决策记录

---

## DEC-R02：压缩摘要结构由 SKILL.md 定义

**日期**：2026-06-04
**状态**：已决定

**决策**：reef 的摘要模板不硬编码在 Go 代码中。使用 SKILL.md 文件定义摘要结构和要求，通过 talents 的渐进式披露机制注入到压缩 prompt 中。

**原因**：

1. 不同场景需要不同的摘要结构——叩问的教练会话需要"学习进度/混淆点/下次检测建议"，Vug 的技术开采需要"已发现概念/已建卡片/待检索源"
2. Anthropic 的 agentskills.io 已经是行业标准方向，Context-Reset-Planner skill 已有先例
3. 用户可通过修改 SKILL.md 定制摘要内容，不需重新编译 Argo

**实现方式**：见 [compact-template.md](compact-template.md)。SKILL.md 定义 5 字段模板（Active Task / Progress / Key Decisions / Constraints / Critical Context），reef 代码只做流程骨架不碰模板内容。

**拒绝**：硬编码模板——不灵活，不同场景的摘要需求不同。叩问的教练会话需要"学习进度/混淆点"，Vug 的技术开采需要"已发现概念/已建卡片"。

---

## DEC-R03：LLM 回溯完整历史用内置 Read 工具

**日期**：2026-06-04
**状态**：已决定

**决策**：reef 压缩完成后，不自动注入文件路径预览。LLM 如需要更多上下文，通过内置 Read 工具按需读取 vault 管理的 [transcript.jsonl](../vault/transcript.md)。待 deck 开发完成后实现 Read 工具时再具体设计，当下仅记录此决定。

**拒绝**：Claude-Code 的 `<persisted-output>` 自动注入——每次都带文件路径和 2KB 预览增加 token 消耗，MVP 让 LLM 主动决定是否回查更经济。

---

## DEC-R05：摘要引用原文——seq 范围 + 原文锚点

**日期**：2026-06-09
**状态**：已决定

**决策**：reef 压缩摘要中每条关键信息附带 `[seq:X-Y] 锚点:"原文开头片段"` 格式的引用。seq 范围定位 transcript 中的消息区间，锚点验证 seq 准确性 + 让 LLM 快速判断是否需要深入阅读。

```markdown
### Key Decisions
选 v1.3.0 而非 v2.0.0 —— v2.0.0 有 breaking change
[seq:401-435] 锚点: "看了 change log，它把 Context 的第一个参数改了"
```

**实现分工**：

| 环节 | 谁做 |
|------|------|
| seq 号生成 | vault（消息落盘时自增） |
| 压缩 prompt 带 seq 前缀 | reef（拼装 prompt 时给每条消息前缀 `[seq:N]`） |
| LLM 输出 seq 范围 + 锚点 | LLM（摘要时附带） |
| 框架验证 seq 引用 | reef（后处理：在 seq 范围内搜索锚点文本。匹配命中→有效；未命中→标注 `[seq:N-M?]`） |

**拒绝**：纯 seq 引用（无锚点）——seq 号 LLM 可能幻觉，框架无法验证。纯锚点（无 seq 范围）——锚点文本可能在 transcript 中出现多次，无法定位。

---

## DEC-R04：摘要模板设计——5 字段通用模板

**日期**：2026-06-05
**状态**：已决定

**决策**：Argo 作为通用 Agent 框架（非编程专用），摘要模板为 5 字段：Active Task（逐字引用）+ Progress（Done/InProgress/Blocked）+ Key Decisions + Constraints + Critical Context。

**去掉的编程特有字段**：Files and Code Sections、Relevant Files、Active State、Completed Actions、Errors and fixes。

**合并的冗余字段**：Resolved/Pending Questions 归入 Progress，Goal 与 Active Task 合并，Remaining Work 归入 Progress.Blocked。

**设计原则**：

| 原则 | 来源 | 说明 |
|------|------|------|
| Active Task 逐字引用 | pi/Hermes | 唯一不允许改写的字段 |
| 空段保留 "(none)" | OpenCode | 保证结构一致性 |
| SUMMARY_PREFIX | Hermes | `[CONTEXT COMPACTION -- REFERENCE ONLY]` 防混淆 |
| 不包含文件/工具记录 | Argo | 原文在 vault 的 transcript.jsonl 中可按需回溯 |

**拒绝**：13 字段完整模板（pi/Hermes）——字段量对小型压缩模型输出不稳定，且多数字段为编程专用。
