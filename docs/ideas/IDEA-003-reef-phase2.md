# IDEA-003：Truncator Intent-Evidence — 意图驱动的工具结果提取（reef Phase 2）

**父索引**：[../ideas.md](../ideas.md)
**日期**：2026-07-16

## 背景

MVP 的 HeadTailTruncator 是"输出太长，砍掉"的防爆机制——保留首尾各约 8K 字符，中间截断。

Phase 2 的 Intent-Evidence Truncator 本质不同：不是截断，而是**上下文准入控制**。完整结果落盘为临时文件，默认不进入消息历史，只有经过任务意图过滤的"证据包"才进入 LLM 上下文。

核心理念：工具结果是**局部证据**（在调用后少数几轮内使用），不是**长期记忆**。

## 方案设想

### ToolIntent → Observation 流程

```
工具调用前：模型声明 ToolIntent
  - 当前假设
  - 这次调用要回答的问题
  - 需要什么证据才算够
  - 什么情况下应该停止调查

工具返回后：系统提取 Observation
  - answers:          回答原问题的证据
  - counter_evidence: 推翻当前假设的证据（强制保留，防御确认偏差）
  - unexpected:       虽然没被问到，但明显异常的信息（强制保留）
  - deferred_leads:   可能有用但不该立刻展开的方向
  - originalPath:     原文临时文件路径
```

### 必须坚持的约束

- 对代码、日志、错误信息，尽量保留原文片段 + 行号/范围，不做纯 NL 总结
- `counter_evidence` 和 `unexpected` 槽位不可被过滤规则忽略
- 每个结论都要能回指到原文文件及原文范围

### 与普通截断的本质区别

| | HeadTailTruncator（MVP） | IntentEvidenceTruncator（Phase 2） |
|------|------|------|
| 做什么 | 截断长输出，防爆 | 按意图提取证据，控制上下文准入 |
| 什么进入上下文 | 首尾各 8K 字符 | 仅证据槽位中的内容 |
| 原文 | 丢弃 | 落盘为临时文件，可重取 |
| 过滤器 | 无 | ToolIntent 声明的意图 |

---

## 决策条件

- Agent 执行多轮工具调用后，老旧工具结果明显干扰 LLM 判断（U 形注意力问题）
- 工具输出中的旁支线索（旧错误、异常代码）频繁引发无关的工具调用链
- token 消耗中工具结果占比过高（HeadTail 8K+8K 仍然太大）
- 有足够可靠的意图声明机制（可能需要 LLM 配合声明 ToolIntent）

> Phase 2 的 IntentEvidenceTruncator 不依赖 LLM——ToolIntent 由模型声明、系统提取，truncator 本身是纯文本/结构化处理，不调模型。
