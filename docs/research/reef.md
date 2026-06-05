# 会话上下文压缩（reef）设计调研

> 调研对象：Claude-Code、OpenCode、pi、Hermes Agent、OpenClaw、Qoder、Trae、WorkBuddy/CodeBuddy、Eino、LangGraph、OpenAI Codex CLI

---

## 1. 触发条件：什么时候该压缩？

### 专业解释

所有项目都在 LLM 调用前检查 token 计数，但阈值差异很大：

| 项目 | 阈值公式 | 源码 |
|------|---------|------|
| **Claude-Code** | `contextWindow - AUTOCOMPACT_BUFFER(13000) - outputReserve(20000)` | autoCompact.ts:72-91, 33-49 |
| **OpenCode** | `total >= usable(context - reserved)`，reserved = `min(20000, maxOutputTokens)` | overflow.ts:8-26 |
| **pi** | `contextTokens > contextWindow - reserveTokens(16384)` | compaction.ts:196-199 |
| **Hermes Agent** | `prompt_tokens >= context_length * 0.50`，硬地板防止过早 | context_compressor.py:439-442,493-513 |
| **Qoder** | 用量 > 40%（~80K tokens）提示用户 | docs.qoder.com |
| **OpenAI Codex** | 硬编码 ~220K tokens（自动）/ 手动可选 | github.com/openai/codex/issues/4106 |

**共同规律**：阈值 = 上下文窗口 - 预留空间（给输出+余量）。没有一个项目在"满了才压"——全部在 40%-85% 之间提前触发。

**反抖动/断路器**（防止无限压缩循环）：

| 项目 | 机制 | 来源 |
|------|------|------|
| **Claude-Code** | `MAX_CONSECUTIVE_AUTOCOMPACT_FAILURES = 3` | autoCompact.ts:70,260-264 |
| **Hermes Agent** | `_ineffective_compression_count >= 2`（连续两次节省率 < 10% 则跳过）| context_compressor.py:504-512 |
| **OpenClaw** | `MAX_OVERFLOW_COMPACTION_ATTEMPTS` | run.ts:1725-1742 |

### 通俗翻译

就像一个水杯——不会等水满到溢出才开始喝，而是在 40%-85% 满的时候就开始喝一口。每个 Agent 框架都有自己的"喝水线"。喝完一口发现水还是太多（压缩效果不好），就不再继续喝了——这叫反抖动。

### 形象举例

你的手机存储空间。Claude-Code 的做法是：128GB 总空间，预留 13GB（系统输出缓存）+ 20GB（给还没拍的视频），实际触发点是 95GB。而 Hermes Agent 的做法更激进——64GB 内存，32GB 就弹窗警告。不是"空间不足"才清理，是"快不够了"就开始清。

---

## 2. 压缩策略：怎么压？

### 专业解释

11 个项目的策略可以归纳为四类，通常混合使用：

**策略 A：简单截断/剪枝（无 LLM，最快）**

| 项目 | 做法 | 来源 |
|------|------|------|
| **LangGraph** | `trim_messages(strategy="middle")` ——保留首尾，裁剪中间 | LangChain Academy 5.2 |
| **Claude-Code** | `snipCompact`：截断最旧消息尾部；`microcompact`：缓存编辑删除已缓存的工具结果 | snipCompact.ts, microCompact.ts:253-293 |
| **CodeBuddy** | v2.73.0 压缩前自动清理超 1 小时的旧工具结果，节省 80%+ 输入 token | codebuddy.ai release notes v2.73.0 |
| **OpenCode** | `prune()` 向后扫描工具结果，超过 40K 后标记为 compacted | compaction.ts:304-348 |
| **Hermes Agent** | `_prune_old_tool_results`：MD5 去重重复工具结果 + 15 种工具类型的结构化摘要 | context_compressor.py:522-685 |

**策略 B：LLM 语义摘要（有 LLM，保留信息最全）**

| 项目 | 做法 | 来源 |
|------|------|------|
| **Claude-Code** | 传统压缩：9 部分详细摘要模板，含 `<analysis>...<summary>...` XML 标签 | compact.ts:387-763, prompt.ts:61-143 |
| **OpenCode** | 7 部分模板（Goal/Constraints/Progress/Key Decisions/Next Steps/Critical Context/Relevant Files），支持增量更新 | compaction.ts:43-78,124-135 |
| **pi** | 6 字段结构化摘要（Goal/Constraints/Progress/Key Decisions/Next Steps/Critical Context），支持增量 UPDATE | compaction.ts:455-518 |
| **Hermes Agent** | 10 章节结构化摘要（含 Active Task/Goal/Constraints/Completed/In Progress/Blocked/Key Decisions 等），增量迭代模式 | context_compressor.py:793-1071 |
| **Qoder** | 调用轻量模型对 P2 内容生成 200-500 字摘要 | docs.qoder.com |
| **CodeBuddy** | 9 章节结构化摘要格式，v2.73.0 改进 | codebuddy.ai release notes v2.73.0 |
| **Codex CLI** | `/responses/compact` 服务器端加密压缩，ZDR 兼容 | developers.openai.com/blog/skills-shell-tips |

**策略 C：优先级分层（信息按重要性分级管理）**

| 项目 | 做法 | 来源 |
|------|------|------|
| **Qoder** | P0（任务目标+钉住文件）永不压缩；P1（最近 3 轮+当前文件）优先保留；P2（3-10 轮旧对话）可摘要；P3（10 轮前无关内容）直接丢弃 | docs.qoder.com |
| **CodeBuddy** | 稳定信息（前置复用）→ 半稳定（增量更新）→ 长期（按需检索注入）→ 动态（摘要/裁剪）| InfoQ 对黄广民的专访 |

**策略 D：服务器端压缩（API 层处理）**

| 项目 | 做法 | 来源 |
|------|------|------|
| **Codex CLI** | `/responses/compact` 返回 `type=compaction` 加密内容，保留潜在理解 | developers.openai.com |
| **Claude-Code** | `cache_edits` API 删除缓存工具结果，不需要重写整个前缀 | microCompact.ts:276-285 |

### 通俗翻译

四类策略从便宜到贵：
1. 截断 = 直接把旧日记撕掉（最快，会丢东西）
2. LLM 摘要 = 让助手帮你把旧日记写成一段话（慢但保留语义）
3. 优先级分层 = 给每页日记贴标签——"永远保留"、"可总结"、"直接扔掉"
4. 服务器端 = 让模型公司帮你在他们的服务器上做这件事

实际项目都是混着用——Hermes Agent 先用截断清掉重复的工具输出，再用 LLM 做语义摘要。

### 形象举例

你要搬家，需要把过去一年的笔记整理好带走：
- 截断策略 = 直接撕掉前 11 个月的，只留最近 30 天
- 摘要策略 = 用一个小时把前 11 个月的内容浓缩成 3 页要点
- 优先级分层 = 给每页笔记分类——合同文件装保险箱（P0）、最近笔记放背包（P1）、旧草稿拍照存档（P2）、废纸丢掉（P3）
- 服务器端 = 雇一个搬家公司帮你整理，你自己不用动手

---

## 3. 保留策略：压完后留下什么？

### 专业解释

所有项目在压缩后都保留最近的对话（"尾部"），差异在于保留量的计算方式：

| 项目 | 保留方式 | 来源 |
|------|---------|------|
| **Claude-Code** | `calculateMessagesToKeepIndex()` 向后扩展，minTokens=10000, minTextBlockMessages=5, maxTokens=40000 | sessionMemoryCompact.ts:324-397,57-61 |
| **OpenCode** | `preserveRecentBudget()` = 可用窗口的 25%，在 2000-8000 tokens 之间，默认保留 2 轮 | compaction.ts:137-142,259 |
| **pi** | `findCutPoint()` 从尾部累积 token 直到预算（默认 keepRecentTokens=20000） | compaction.ts:328-376,115 |
| **Hermes Agent** | 头部保护 3 条消息；尾部 token 预算 = `summary_target_ratio * context_length`（~20K tokens），硬性最小值 3 条 | context_compressor.py:1178,1272-1334 |

**API 边界保护**（所有项目都做）：

| 项目 | 做法 | 来源 |
|------|------|------|
| **Claude-Code** | `adjustIndexToPreserveAPIInvariants()` 不拆分 tool_use/tool_result 对和思考块 | sessionMemoryCompact.ts:232-313 |
| **OpenCode** | `splitTurn()` 在轮次级别切割 | compaction.ts:162-185 |
| **pi** | `findValidCutPoints()` 跳过 toolResult、compaction、branch_summary 条目 | compaction.ts:261-298 |
| **Hermes Agent** | `_align_boundary_forward()` 推进切割点到消息组边界 | context_compressor.py:1178 |
| **CodeBuddy** | v2.73.0 压缩前剥离图片内容为占位符 | codebuddy.ai release notes v2.73.0 |

### 通俗翻译

压缩不是一个字节一个字节地切——那样会把一句话切成两半。每个项目都确保"切割点在安全位置"：tool_use 和 tool_result 这对不能拆散（拆了模型就不知道这个结果对应哪个调用），同一个轮次不能拆开（中间砍一刀上下文就坏了）。

### 形象举例

你不能在一句话的中间把日记撕开——"今天我要去买..."——买什么？压缩也一样。Claude-Code 的处理方式就像在段落边界处撕，Hermes 在章节边界处切，pi 在页码边界处裁。数据不散架，模型才能继续读。

---

## 4. 增量更新：二次压缩时怎么办？

### 专业解释

| 项目 | 增量策略 | 来源 |
|------|---------|------|
| **OpenCode** | `buildPrompt()` 检测到前次摘要时用 "Update the anchored summary...Preserve still-true details, remove stale details, merge new facts" | compaction.ts:124-135 |
| **pi** | `UPDATE_SUMMARIZATION_PROMPT` 包含 `<previous-summary>` 内容 | compaction.ts:415-452 |
| **Hermes Agent** | `_previous_summary` 存在时切换到 `_SUMMARY_MODE.UPDATE`，补充新内容而非重写 | context_compressor.py:899-912 |
| **Qoder** | 不支持增量，每次都全文重摘要 | docs.qoder.com |
| **Claude-Code** | 不支持增量的传统路径；但会话记忆路径通过后台异步提取实现了类似的增量效果 | sessionMemory.ts:318 |

### 通俗翻译

第一次压缩前 100 轮对话 → 生成一段摘要。又聊了 50 轮，又要压缩——此时有两次选择：把前 50 轮的摘要 + 新的 50 轮当输入重新摘要（全文重摘要），还是在旧摘要的基础上追加新内容（增量更新）。OpenCode、pi、Hermes 的做法是第二种——"上次摘要说了这些，新发生的是这些，更新一下"。

### 形象举例

你的工作周报。全文重摘要 = 每次写周报都重新回忆这一季度的所有进展。增量更新 = 看上周的周报，把本周的新进展加进去，把已完成的事项划掉。pi 的 UPDATE_SUMMARIZATION_PROMPT 就是在做后者——告诉模型"这是上周的周报，请把本周的事加进去"。

---

## 5. 压缩标记：怎样告诉 LLM "前面被压缩了"？

### 专业解释

| 项目 | 标记方式 | 来源 |
|------|---------|------|
| **Claude-Code** | `compact_boundary` 系统消息 + `isCompactSummary` 用户消息 + 可选 "full transcript at: [path]" | messages.ts:4530, prompt.ts:337-374 |
| **OpenCode** | Assistant 消息 `mode: "compaction"` + `summary: true` + `CompactionPart.{auto, overflow, tail_start_id}` | compaction.ts:418-442,476-480 |
| **pi** | `<summary>...</summary>` XML 标签包裹 + `compactionSummary` 角色消息 | messages.ts:4-10 |
| **Hermes Agent** | 系统提示中注入压缩通知 + 摘要以独立消息插入 + `--- END OF CONTEXT SUMMARY ---` 分隔符 | context_compressor.py:1452-1556 |
| **Codex CLI** | `type=compaction` 加密内容（不透明给客户端但保留潜在理解） | developers.openai.com |

**消息顺序**：

Claude-Code（compact.ts:330-338）:
```
[boundaryMarker] → [summaryMessages] → [messagesToKeep] → [attachments] → [hookResults]
```

LangGraph 社区推荐:
```
[system prompt] → [tool definitions] → [compaction summary] → [recent messages]
```

### 通俗翻译

压缩后，每个项目都会在对话里插入一个"分隔线"——告诉 LLM "前面的对话已经被浓缩成下面这段摘要了"。这个分隔线的形式各不相同——Claude-Code 是一条系统消息，OpenCode 是消息上的一个标记字段，pi 是一段 XML。

### 形象举例

你看一部电视剧，跳过第 3-8 集直接看第 9 集。画面一开头就出现 "前情提要" 字幕——告诉你前几集发生了什么。压缩标记就是这个"前情提要"——告诉 LLM "刚才那些对话已经帮你浓缩好了，接着往下聊"。

---

## 6. OpenClaw 的三路触发 + OpenCode 的 REPLAY

### 专业解释

**OpenClaw 的三路压缩触发**（run.ts）：

| 触发路径 | 行号 | 条件 |
|---------|------|------|
| preflightRecovery | 1531-1542 | attempt 执行前上下文已超限 |
| 超时触发 | 1565-1658 | LLM 超时 + token 使用率 > 65% |
| 上下文溢出 | 1691-1992 | promptError / assistantError 检测到溢出 |

三层恢复：先 SDK 自动压缩 → 不成就显式 `contextEngine.compact()` → 再不行就截断工具结果。

**OpenCode 的 REPLAY 机制**（compaction.ts:371-386）：

压缩完成后，原始用户消息的非媒体部分被**重新注入**——防止压缩丢失"用户在问什么"。

**Eino 的 MessageRewriter**：

```go
type MessageRewriter func(ctx, state *AgentState) (*AgentState, error)
```

在 ChatModel 调用前持久化修改消息历史，先于 MessageModifier 执行。来自多个来源：[1] cloudwego.io Eino ReAct Agent Manual [2] github.com/cloudwego/eino

### 通俗翻译

OpenClaw 是三只眼睛看着上下文——预检（出发前检查）、超时（路上发现不对劲）、溢出（到达时被告知装不下了）。三只眼睛各管一段。OpenCode 有一个聪明的设计叫 REPLAY——压缩完后把用户的最后一条问题重新扔给 LLM，因为压缩过程中可能把"用户在问什么"这个最关键的信息搞丢了。

OpenClaw 的三路触发就像你开车出远门——出发前检查油箱（preflight）、开到一半发现油耗太高停下来加油（超时）、到了目的地被告知停车场满了（溢出）。三个检查点覆盖了整个旅程。

OpenCode 的 REPLAY 就像你让同事帮你整理一份会议记录——整理完后同事把会议一开始老板布置的那句话原样念给你听，确保你没忘记"到底是让干嘛的"。

### Eino 的 MessageRewriter：

这是个钩子函数，每次问 LLM 之前自动跑一遍。如果钩子发现消息太多就改一改——这个改动是**持久化**的，下一轮 LLM 看到的就是改动后的版本。另一个叫 MessageModifier 的钩子只做临时修改（比如给这一轮加条系统提示），不影响下一轮。

就像你有一个秘书，每次你进门开会前他都帮你整理桌面。MessageRewriter 会把整理好的文件归档（持久化），MessageModifier 只是在你开会时放一杯咖啡（临时）。

---

## 7. Qoder 的四级优先级 + CodeBuddy 的四层分类

### 专业解释

**Qoder P0-P3 体系**：

| 优先级 | 内容 | 保留策略 |
|------|------|---------|
| P0 | 任务目标、核心计划、用户钉住的文件 | 永不压缩 |
| P1 | 最近 3 轮对话、当前打开文件、最近修改 5 个文件 | 优先保留 |
| P2 | 3-10 轮前对话、无关文件、旧计划 | 可摘要 |
| P3 | 10 轮前、完全无关内容 | 直接丢弃，按需从向量数据库检索 |

来源：[1] docs.qoder.com Smart Context Control [2] CSDN 上下文窗口管理深度解析

**CodeBuddy 四层上下文资产分类**：

| 层级 | 类型 | 管理方式 |
|------|------|---------|
| 稳定信息 | System Prompt / 规范 / 工具说明 | 前置并复用 |
| 半稳定信息 | 环境与仓库概况 | 增量更新 |
| 长期信息 | 偏好与项目约定 | 按需检索注入 |
| 动态信息 | 对话与执行状态 | 摘要/裁剪保持短而准 |

来源：[1] InfoQ 对 CodeBuddy 产品负责人黄广民的专访 [2] codebuddy.ai memory docs

### 通俗翻译

Qoder 把对话历史分成四个等级——和企业的文件保密级别一模一样。P0 是绝密文件放保险柜（任务目标），P1 是今天办公桌上的文件（最近几轮对话），P2 是文件柜里的旧档案（可以摘要一下），P3 是碎纸机里的垃圾。CodeBuddy 的分类维度不同——它不是按"重要性"分级，而是按"变化频率"分层。变化最慢的东西放最前面（System Prompt），变化最快的东西放最后（对话状态）。

### 形象举例

Qoder 的四级优先级就像一个搬家公司的分类系统：
- P0：祖传家谱——一个标点都不能丢，贴身携带
- P1：最近买的东西——装箱子里，搬进新家
- P2：去年的旧书——拍个照存手机，书本身捐掉
- P3：已经过期两年的杂志——直接回收。但如果搬进新家后发现某本杂志上有重要信息，可以翻手机相册（向量数据库）找回来

---

## 8. 对 Argo reef 的启示

1. **压缩是 Agent Loop 的一阶原语**——Codex CLI 和 OpenCode 的经验证明，压缩不是应急回退，而是在循环中有确定位置的标准动作
2. **混合策略是唯一正确解**——没有一个项目只靠一种策略。至少需要：截断（快）+ LLM 摘要（准）+ 优先级标记（精）
3. **增量更新是压缩质量的基石**——OpenCode/pi/Hermes 全部做增量更新。全文重摘要的 Qoder 是少数派
4. **API 边界保护不可或缺**——所有调研项目都做了 tool_use/tool_result 对保护，这是压缩和 API 协议之间的硬性合约
5. **反抖动/断路器防止压缩死循环**——三个项目有独立的反抖动机制。没有这个，压缩本身可能吃掉比它节省的还多的 token
6. **Qoder 的 P0-P3 和 CodeBuddy 的四层分类**代表了压缩策略的两条路——按重要性分还是按变化频率分。Argo 可以先按重要性（简单），后续加变化频率维度
7. **Codex CLI 的服务器端压缩**和 **Claude-Code 的 cache_edits** 是未来方向——利用 API 层能力做压缩，比应用层更高效
