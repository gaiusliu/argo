# IDEA-008：Model-Authored Observation Digest — 主模型自裁剪工具结果实验

**父索引**：[../ideas.md](../ideas.md)
**日期**：2026-07-16

## 背景

[IDEA-003](IDEA-003-reef-phase2.md) 提出 Intent-Evidence Truncator：工具完整结果默认不进入消息历史，由系统根据 ToolIntent 提取证据包。这是更接近最终形态的上下文准入控制，但也要求系统侧已经具备足够可靠的结构化提取器。

本 idea 是一个更偏验证的中间方案：**允许原始 tool result 进入主模型一次，让主模型基于调用前声明的意图问题集生成持久化 Observation Digest，后续上下文用 digest 替代原始 tool result**。

核心要验证的问题不是“能否把成本压到最低”，而是：

- 模型能否稳定识别一次工具调用真正服务的问题
- 模型生成的 digest 是否足以支撑后续任务推进
- digest 是否能降低 context drag、无关旁支探索和长任务注意力稀释
- raw result 只进主模型一次的成本，是否能被后续轮次节省抵消

## 与 IDEA-003 的区别

| | IDEA-003 Intent-Evidence | IDEA-008 Model-Authored Digest |
|------|------|------|
| 目标 | 最终型上下文准入控制 | 验证模型自裁剪是否有效 |
| raw tool result | 默认不进入主模型 | 进入主模型一次 |
| 裁剪主体 | 系统侧 deterministic extractor | 主模型自己生成 digest |
| 成本目标 | 尽早降低 token 和注意力负担 | 优先验证语义保真和任务成功率 |
| 风险 | 系统提取器可能漏证据 | 主模型可能摘要漂移或过度压缩 |
| 适合阶段 | Phase 2 正式能力 | Phase 2 前置实验 / feature flag |

## 方案设想

### 1. ToolIntent：工具调用必须声明目标问题集

模型发起工具调用时，除了工具参数，还必须给出结构化意图：

```yaml
tool_intent:
  hypothesis: 当前假设或调查焦点
  questions:
    - 这次工具调用要回答的问题
  sufficient_evidence:
    - 哪些证据足以支持下一步判断
  stop_conditions:
    - 哪些结果出现后不应继续扩大调查
```

这部分不是给用户看的解释，而是给后续 digest 使用的裁剪锚点。它让模型在看到 tool result 后必须回到“为什么调用这个工具”。

### 2. Raw result 进入主模型一次

工具执行后，本轮 model call 仍然收到完整或当前截断策略下的 raw tool result。这样可以最大限度验证：

- 模型在看到完整上下文后是否会选择正确证据
- 模型是否会被旁支线索诱导继续探索
- digest 和 raw result 相比丢失了哪些关键信息

完整原文仍应落盘或由 vault/transcript 保管，digest 中只保留回读引用。

### 3. 主模型生成 Observation Digest

工具结果进入主模型后，prompt 追加一段固定指令：模型在继续规划或调用下一个工具前，必须为刚完成的 tool call 生成 Observation Digest。

建议结构：

```yaml
observation_digest:
  tool_call_id: ...
  tool: ...
  args: ...

  intent_questions:
    - ...

  answers:
    - question: ...
      status: answered | partially_answered | unanswered
      finding: ...
      evidence:
        - ref: path:line | log_range | output_excerpt
          quote: 精确保留的关键原文

  counter_evidence:
    - 与原假设冲突、必须保留的证据

  unexpected:
    - 与 intent 无关但明显异常、可能影响当前任务的观察

  task_state_update:
    - 本次观察如何改变当前假设、相关文件、失败点或工作集

  omitted:
    - 省略了什么类型的信息、数量或范围
    - full_result_ref: observation://...

  deferred_leads:
    - lead: 新方向或旁支线索
      reason_to_defer: 为什么不应立即展开
      resume_condition: 什么情况下再探索
```

Digest 的目标不是“总结短”，而是形成**下一轮推理可继续依赖的证据包**。

### 4. 后续上下文替换 raw result

当 digest 生成并通过基本校验后，后续轮次中同一个 tool result part 可以被替换为 digest。

关键约束：

- 替换决策按 `tool_call_id` / `part_id` 持久化
- 一个 part 一旦从 raw result 替换为 digest，后续所有轮次保持相同 digest
- 不做“保留最近 N 条，老的滑出窗口再替换”的滑窗策略
- 不反复重写同一个 digest，除非显式产生新版本并记录版本号

这避免知乎文章中提到的滑窗式 stub 替换导致 prompt cache 每步失效。

## Digest 写作约束

### 必须保留

- 调用前的 intent questions
- 每个 question 的回答状态：answered / partially_answered / unanswered
- 关键证据的原文片段，不改写代码、错误消息、测试名、命令、路径、行号
- `counter_evidence`：推翻当前假设的证据
- `unexpected`：虽然没被问到但明显异常的信息
- `omitted`：明确说明省略了什么，以及如何回读完整结果
- `deferred_leads`：记录新方向，但不默认触发探索

### 尽量避免

- 泛泛的自然语言总结
- 没有证据指针的判断
- 把 observation 写成下一步计划
- 把多个独立工具观察揉成一个不可追溯的段落
- 因压缩预算不足而删除 unanswered / omitted / counter_evidence 槽位

### 预算建议

- 默认 digest：约 1024 token
- 复杂工具输出：最多约 2048 token
- 超过 2048 仍无法表达清楚时，说明本次工具结果包含多个独立观察，应拆分或要求模型选择优先级，而不是继续扩写 digest

## 新方向处理

工具结果经常暴露调用前没有预期的新方向。它们不应被丢弃，也不应自动诱导模型无限探索。

建议在 digest 中记录为 `deferred_leads`，并按三级处理：

| 级别 | 判定 | 行为 |
|------|------|------|
| auto | 不探索就无法完成当前用户目标，或直接推翻当前假设 | 允许继续工具调用 |
| queued | 相关但不是当前 blocker，或需要较大范围读取 | 记录，当前目标完成后再处理 |
| ask_user | 明显扩大任务边界、涉及产品/架构决策、可能显著增加成本 | 请求用户确认 |

实现上可以先只要求模型产出 `deferred_leads`，由人工或后续调度策略观察是否足够。

## Cache 与水位线关系

这个方案不是每次 tool call 后额外调一个 compact 模型，而是在下一次主模型调用中顺手生成 digest。因此它避免了额外模型调用成本，但仍有两个成本：

- 当前轮 raw result 仍然进入主模型一次
- 后续把 raw result 替换为 digest 时，对应前缀会发生一次字节变化

可接受的前提是：**替换是单调的、一次性的、按 part 持久化的**。它不是滑窗 compact。

与四级水位线的关系：

- IDEA-008 处理“新工具结果如何从 raw 变成 observation”
- reef 水位线处理“旧 observation / 旧消息在上下文压力下如何 snip、prune、summarize”
- 二者可以叠加，但不应互相反复重写同一段历史

## 验证指标

### 质量指标

- 任务成功率不低于当前 truncator 策略
- digest 后续轮次是否足以继续任务，减少对 full result 的回读
- digest 是否保留了关键错误、路径、行号、测试名、函数名
- 是否出现摘要漂移：路径/错误/结论被改写或误记
- 新方向是否被正确放入 deferred，而不是自动扩大任务

### 成本指标

- 总 input token / output token
- cache_read / cache_write 比例
- 每任务工具调用次数
- 每任务模型轮数
- raw result 被 digest 替换后的后续累计节省
- full result 回读次数

### 行为指标

- 因旁支线索触发的额外工具调用数量
- unanswered 问题是否会被模型诚实保留，而不是伪装成已回答
- counter_evidence 是否被保留并影响后续判断

## 决策条件

值得试验：

- 工具结果占上下文比例高，且长任务中反复携带旧工具输出
- 当前 HeadTailTruncator 能防爆，但仍让模型暴露过多无关观察面
- 希望先验证“模型自裁剪是否足够可靠”，再投入 deterministic extractor

转向 IDEA-003：

- digest 质量稳定，回读率低，关键证据保留率高
- 主要成本瓶颈变成 raw result 进入主模型一次
- 工具类型的证据选择模式已经清晰，可以沉淀为系统侧 extractor

应停止或回退：

- digest 经常丢失关键错误、路径、行号或用户约束
- 模型把 deferred leads 当成下一步计划频繁展开
- raw->digest 替换造成 cache_write 成本明显上升且无法通过 part 持久化控制
- full result 回读率过高，说明 digest 信息密度不足

## 开放问题

- ToolIntent 是否应作为工具 schema 的显式字段，还是由 prompt 约束模型在工具调用前输出？
- Digest 应作为 assistant 消息进入 transcript，还是作为 tool_result 的 replacement metadata？
- 替换 raw result 后，用户界面是否仍默认展示 raw result，模型上下文只看 digest？
- 是否需要对 digest 做 schema validation，失败时保留 raw result 或退回 HeadTail？
- 对并行 tool calls，digest 是逐个生成还是合并生成？

## 推荐落地方式

先以 feature flag 做最小实验：

1. 要求模型在工具调用前声明 `tool_intent.questions`
2. 工具结果仍按当前策略进入主模型一次
3. 下一次 model prompt 要求生成 `observation_digest`
4. 通过 `tool_call_id` 持久化 digest，并在后续上下文替换 raw result
5. trace 中记录 digest token、raw token、回读次数、cache 命中、deferred leads 数量

如果实验显示 digest 能稳定保留关键证据，再把高频工具类型沉淀为 IDEA-003 的 deterministic Intent-Evidence Truncator。
