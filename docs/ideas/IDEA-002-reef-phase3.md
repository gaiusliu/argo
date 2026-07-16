# IDEA-002：Compactor LLM Summary + 自适应预算（reef Phase 3）

**父索引**：[../ideas.md](../ideas.md)
**日期**：2026-07-16

## 背景

reef MVP（迭代 014）已完成策略接口抽取，当前 Compact 默认策略为 `"llm-summary"`，但实现尚未注册——`resolveCompactor()` 在工厂缺失时回退 `passthrough` + warn 日志。

Phase 3 的目标是实现真正的 LLM Summary Compactor，以及配套的自适应预算和断路器机制。

参考：全部 7 个主流 agent（Codex CLI、Claude Code、Hermes、OpenClaw、PI、Kilocode、Opencode）的默认 compact 策略均为 LLM Summarization。

## 方案设想

### 1. LLM Summary Compactor

调 LLM 生成结构化摘要，旧消息被摘要替代，原文由 vault 保管。支持增量 UPDATE（传上次摘要 + 新对话，指令是"更新摘要"而非从头生成）。

**7 段结构化模板**（对齐 Hermes / Opencode 经实战验证的格式）：

```
## Goal
## Constraints & Preferences
## Progress（Done / In Progress / Blocked）
## Key Decisions
## Next Steps
## Critical Context
## Relevant Files
```

关键约束：
- **bullet 而非段落**——紧贴 token 预算
- **保留原文字符串**——文件路径、错误信息、命令不做改写
- **每个 Section 必须出现**——空则写 `(none)`，防止 LLM 跳过
- **不提 compaction 过程本身**
- **迭代更新**——增量模式，不从头生成

### 2. 自适应预算

根据上下文窗口利用率动态调整 compact 触发阈值。当前固定阈值为 80%（对齐 Claude Code）。Phase 3 可引入自适应策略：利用率持续偏高时提前触发，偏低时延后。

### 3. 断路器

连续 2 次 compact 节省 < 10% → 本次 session 跳过后续 compact。防止反复调用 LLM 却无实际收益。

### 4. 新方向三级处理

工具结果中发现的额外线索，不全部自动展开，也不全部封堵：

| 级别 | 判定 | 行为 |
|------|------|------|
| **auto** | 不探索就无法完成原任务 / 推翻当前假设 | 自动展开调查 |
| **queued** | 相关但不是当前 blocker / 需要较大范围读取 | 排队，当前任务完成后处理 |
| **ask_user** | 明显扩大任务边界 / 涉及产品决策 | 提请用户决定 |

---

## 决策条件

- LLM Summary Compactor：当前上下文窗口利用率持续 > 80%，passthrough 回退已不够用
- 自适应预算：token 消耗波动大，固定阈值不够精准
- 断路器：LLM Summary 频繁调用的 token 开销需要控制
- 新方向三级处理：Agent 因自动展开过多无关方向而偏离主任务

> Phase 3 的 Compactor 需要调 LLM，但 reef 不依赖 sail。按 MVP 已设计的 `SummaryFunc` 适配器解耦——reef 定义签名，helm 用 `sail.Provider` 实现注入。
