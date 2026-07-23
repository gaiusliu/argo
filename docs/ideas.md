# 想法池

> 调研过程中发现的、当下不做的未来方向。append-only，想法采纳/否决后改状态。

## IDEA-001：架构长期演进 — 分层、领域模型、事件驱动

**日期**：2026-07-16
**状态**：💡 待评估
**引用**：[详细展开](ideas/IDEA-001-architecture-evolution.md)

三层/四层架构拆分、Tale 封装消息与持久化游标、ToolUse 职责拆分、事件总线替代 Ask 断连。分三阶段渐进推进，DI 已完成后分层自然浮现。

## IDEA-002：Compactor LLM Summary + 自适应预算（reef Phase 3）

**日期**：2026-07-16
**状态**：💡 待评估
**引用**：[详细展开](ideas/IDEA-002-reef-phase3.md)

7 段结构化 LLM 摘要替代 passthrough、自适应预算动态调整 compact 阈值、断路器防止无效 compact、新方向三级分诊（auto / queued / ask_user）。

## IDEA-003：Truncator Intent-Evidence — 意图驱动的证据提取（reef Phase 2）

**日期**：2026-07-16
**状态**：💡 待评估
**引用**：[详细展开](ideas/IDEA-003-reef-phase2.md)

工具结果从"默认进入长期上下文"改为"意图约束的证据提取 + 按需重取"。ToolIntent 声明假设和问题，Observation 提取 answers / counter_evidence / unexpected / deferred_leads 四个证据槽位。

## IDEA-004：事件总线 + Ask 双向流

**日期**：2026-07-16
**状态**：💡 待评估
**引用**：[详细展开](ideas/IDEA-001-architecture-evolution.md)（见 IDEA-001 第 3 节）

引入内存事件总线，Ask 流程改为 `helm → EventBus → cli → EventBus → helm` 双向流，不再断连重建 SSE。

## IDEA-005：依赖注入后续 — 移除全局 Config、AgentService 抽象

**日期**：2026-07-16
**状态**：💡 待评估
**引用**：[dependency-injection-plan.md](dependency-injection-plan.md)（未执行项）

迭代 010/011 跳过的项目：移除 `knot.GetConfig()` 全局单例（需改动 server 层引入 agent 依赖）、Server 层 `AgentService` 抽象（待 `handlePrompt` 进一步膨胀或需要新传输层）。

## IDEA-006：依赖注入改造（已完成部分）

**日期**：2026-07-13
**状态**：✅ 已采纳 → 迭代 [010](iterations/010-di-refactor/main.md) / [011](iterations/011-sail-provider/main.md)
**引用**：[dependency-injection-plan.md](dependency-injection-plan.md)

`sail` 注入 `*http.Client`、`brig` 拆分为 Gate + Warden + RuleLoader、`voyage.Session` 注入 Vault + Gate（functional options）。记录为已完成想法，保留原文作为实施参考。

## IDEA-007：HandoffCompactor — Amp 模式的新会话交接压缩

**日期**：2026-07-16
**状态**：💡 待评估

**来源**：调研 Codex CLI、Claude Code、Codex、OpenClaw、PI、Kilocode、Opencode、Hermes-agent、Amp 共 9 个 agent 的 compact 策略。详见 [DEC-041](../decisions.md) 和 [知乎文章](https://zhuanlan.zhihu.com/p/2047337481255884504)。

**核心思路**：Amp 不做递归压缩，用 `/handoff` 生成交接文档 → 开新会话从头开始。与 LLMSummaryCompactor（会话内压缩）互补，形成双轨策略。

**设计要点**：
- Compactor 接口的新实现（`HandoffCompactor`），与 `LLMSummaryCompactor` 并列
- 调用 LLM 生成结构化交接文档（目标/进展/决策/待办/关键上下文/相关文件）
- 自动创建新 session（通过 Voyage），将交接文档作为首条 system message 注入
- 旧 session 保留完整对话记录，通过 session ID 引用（类似 Amp 的 `@@` 引用）
- 用户可审查和编辑交接文档再发送

**适用场景**：
- 任务阶段性完成，需要切换焦点到新任务
- 会话过长，递归摘要质量已明显下降
- 用户主动想「打包当前进度，重新开始」

**参考**：
- Amp Handoff 官方博客：https://ampcode.com/news/handoff
- Amp 线程模型：`threads: map` 可视化 + `@@` 跨线程引用
- OpenAI 内部研究：递归摘要导致性能持续衰减

**与当前 LLMSummaryCompactor 的关系**：不替代，互补。LLMSummaryCompactor 处理「还在同一个任务里，只是上下文装不下」；HandoffCompactor 处理「任务阶段切换，需要新起会话」。

## IDEA-008：Model-Authored Observation Digest — 主模型自裁剪工具结果实验

**日期**：2026-07-16
**状态**：💡 待评估
**引用**：[详细展开](ideas/IDEA-008-model-authored-observation-digest.md)

IDEA-003 的前置验证分支：工具结果先进入主模型一次，由主模型基于调用前声明的 `ToolIntent` / `intent_questions` 生成持久化 Observation Digest；后续上下文用 digest 替代 raw tool result。重点验证模型自裁剪能否保留关键证据、降低 context drag、控制 deferred leads，同时用单调 part 替换避免滑窗式 prompt cache 失效。
