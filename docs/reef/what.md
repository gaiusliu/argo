# reef 行为规约

## 概述

reef 负责单次会话内的上下文裁剪，防止 LLM 调用因 token 超限而失败（413 prompt too long）。它在每轮 LLM 调用前被 helm 调用，判断消息历史是否需要压缩，如需则切割旧消息、委托 LLM 生成结构化摘要，将摘要注入上下文以替换被切割的内容。

reef 只管理会话内的上下文长度，不涉及跨会话的记忆存储（那是 vault 的职责）。

---

## 需求

### 需求：触发判定

reef SHALL 在消息历史 token 数超过窗口的 80% 时触发压缩。未超阈值时原样返回消息，不做任何修改。

#### 场景：消息未超阈值

- 给定：消息历史 token 数 = 30K，context window = 200K
- 当：reef.Compact() 被调用
- 则：返回原消息列表，不触发压缩

#### 场景：消息超阈值

- 给定：消息历史 token 数 = 170K，context window = 200K
- 当：reef.Compact() 被调用
- 则：触发压缩流程

---

### 需求：安全切割

reef SHALL 从后往前扫描消息，找到安全的切割位置。切割点必须满足：(a) 不在 tool_use 和对应 tool_result 之间；(b) 不在同一轮次的 assistant 消息中间。切割后，尾部（最近的 ~40K tokens）完整保留，头部（旧消息）进入压缩。

#### 场景：切割点落在 tool_use/tool_result 对中间

- 给定：消息序列 [... tool_use(grep), tool_result(grep), tool_use(read), user_msg]
- 当：token 累加在 tool_result(grep) 处触发切割
- 则：切割点前移，将 tool_use(grep) + tool_result(grep) 作为完整对纳入头部

#### 场景：切割点落在轮次中间

- 给定：助手消息包含 3 个 tool_use + 部分 tool_result
- 当：切割点在助手消息内部
- 则：切割点前移至该轮次的起始 user 消息位置

---

### 需求：LLM 摘要

reef SHALL 将头部消息块交给 LLM，按照 SKILL 定义的模板生成结构化摘要。摘要模板由 reef/compact SKILL.md 提供（具体结构见 [compact-template.md](compact-template.md)），reef 自身不硬编码摘要结构。

#### 场景：生成摘要

- 给定：头部包含 80 轮对话，SKILL 定义了 Active Task / Progress / Key Decisions / Constraints / Critical Context 五字段模板
- 当：reef 调 LLM 生成摘要
- 则：LLM 返回按模板结构组织的摘要，Active Task 字段包含用户最近请求的逐字引用

---

### 需求：消息重组

reef SHALL 将 LLM 返回的摘要包装为 system 消息，标记 SUMMARY_PREFIX（"[CONTEXT COMPACTION -- REFERENCE ONLY]"），后接保留的尾部消息，返回完整重组后的消息列表。

#### 场景：重组压缩后消息

- 给定：LLM 返回摘要 + 尾部消息 10 条
- 当：组装最终消息列表
- 则：消息列表 = [system: "CONTEXT COMPACTION...\n<summary>...</summary>"] + 尾部消息

---

## 边界情况

- 摘要 LLM 调用失败：鉴权失败/token 额度耗尽 → 显式通知用户并等待回应。其他错误 → 默认模型重试 1 次（间隔 1s）→ 切换备用模型/厂商 → 全部失败则硬截断兜底
- 连续压缩触发：连续 3 次压缩后 token 仍超阈值，跳过本次压缩，避免无限循环
- 会话无 SKILL 定义：使用内置的最简模板（仅 Active Task + Progress）
