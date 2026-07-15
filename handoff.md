# Handoff — 2026-07-16

## 本次会话成果

### 迭代 014：reef 策略接口抽取 + LLM Summary Compactor

| 变更 | 文件 | 说明 |
|------|------|------|
| ContextConfig | `src/knot/types.go`, `config.go` | 新增 context 段（compact, compactThreshold=80, truncation），默认 compact="llm-summary", truncation="head-tail" |
| Truncator 接口 + 实现 | `src/deck/truncator.go`, `truncation_headtail.go`, `truncation_types.go` | Truncator 接口 + HeadTailTruncator + ToolIntent/Observation 预声明；reef 包已删除 |
| Compactor 接口 + 实现 | `src/helm/compactor.go`, `compact_llm_summary.go` | Compactor 接口 + LLMSummaryCompactor（7 段结构化模板，阈值 80%，head 3 条 + tail 8K token 倒推，serializeForCompact 对齐 Opencode 合并 tool call+result） |
| Truncator 注入 deck | `src/deck/builtin_tool.go`, `cli_tool.go`, `tool.go` | builtinTool/cliTool 加 truncator 字段，RegisterTools 内部 resolveTruncator() |
| Compactor 注入 helm | `src/helm/phase_model.go` | PhaseRunnerModel 加 Compactor 字段，resolveCompactor(provider) 在 Run() 中调用，Compact 在 BuildRequest 之后、Chat 之前 |
| MarkCompact 桩 | `src/tale/tale.go` | 空实现，等待 transcript 持久化方案确定 |

### commit 记录

未提交。当前 git status：修改 12 个文件，新增 6 个文件，删除 reef/ 目录下 4 个文件。

### 关键设计决策

| 决策 | 说明 |
|------|------|
| reef 包解散 | Compactor 归 helm（依赖 LLM），Truncator 归 deck（依赖工具），reef/ 目录删除 |
| 默认 compact="llm-summary" | 调研 Codex、Claude Code、Hermes、OpenClaw、PI、Kilocode、Opencode 全部 7 个 agent，均默认 LLM Summary；Argo 从 passthrough 改为 llm-summary |
| compactThreshold=80% | 对齐 Claude Code；Hermes 的 50% 太激进（每轮都触发） |
| LLM Summary 7 段格式 | 复用 Hermes/Opencode 实战模板：Goal / Constraints / Progress / Key Decisions / Next Steps / Critical Context / Relevant Files |
| serializeForCompact 合并 tool call+result | 对齐 Opencode：预建 ToolCallID→Content map，assistant 消息的 ToolCalls 与对应 tool 消息合并输出为 `[Tool call]: name(args) [Tool result]: content` |
| serializeForCompact 不截断 tool 内容 | 截断是 Truncator 的职责，Compactor 不应重复处理 |
| Compactor 返回值用区间 | `(summary, from, to, err)`——被覆盖的消息区间 [from, to)，不返回重构后的消息列表 |
| Compact 阈值判断在 Compact 内部 | Run() 不关心触发条件，只传入 inputTokens（v.LastUsage.InputTokens API 精确值） |
| MaxTokens 通过 Provider 获取 | sail.Window(provider.Model())，不在接口传参 |
| 策略内部自行 Resolve | deck.RegisterTools() 内部 resolveTruncator()；helm.NewPhaseRunnerModel()→Run() 内部 resolveCompactor(provider)，不通过调用链参数传递 |
| no SummaryFunc | Provider 直接注入 LLMSummaryCompactor 结构体；模板硬编码在 Compact 方法内 |

## 待解决问题

### 1. transcript.jsonl 持久化：compact 标记应该写在哪里

**现状**：Tale.MarkCompact() 是空桩。compaction 发生后的摘要需要持久化到 transcript.jsonl 以便下次加载时恢复压缩视图。

**讨论过的方案**：

| 方案 | 描述 | 结论 |
|------|------|------|
| state.json 存储 CompactMeta | 和 Saved 游标一起序列化 | ❌ state.json 只管控制流状态，不应掺和 transcript 内容 |
| 两条写路径（AppendMessages + AppendCompact） | checkpoint 时分别写消息 delta 和 compact 记录 | ❌ 顺序可能不一致，compact 应在消息流中按时间排列 |
| **compact 作为 transcript 的一种 entry 类型** | 添加 `RecordCompact` 类型，compaction 标记按时间顺序写入 transcript.jsonl，和 user/model/tool 并列 | ✅ 对齐 PI 和 Opencode 的做法 |

**Opencode/PI 做法**：compaction 是 entry 列表中和 message 并列的一类 entry，按时间顺序排列。`buildSessionContext()` 遇到 compaction 时应用压缩视图。

### 2. 客户端 vs Model 收到什么消息

**加载对话记录**（给客户端）：全量加载 transcript.jsonl 中的所有 record——含 user/model/tool/compact。客户端拿到完整对话 + compaction 标记，按需渲染。

**BuildRequest**（给 LLM）：遇到 compact 消息时跳过被覆盖的 `messages[from:to]`，插入 `{role:system, content:summary}` 替代。LLM 看不到被压缩的原始消息，只看到摘要 + 尾部最新消息。

### 3. 摘要中的原文路径应该指向哪个文件

**现状**：HeadTailTruncator 将超长工具输出截断后用 temp file 存储原文（`/tmp/argo-truncation/argo-<ts>-<rand>`），metadata["truncation"]["originalPath"] 指向该 temp 文件。

**讨论**：既然 transcript.jsonl 全量保留消息，原文不应"外存"到 temp file。原文就在消息本身的 `Content` 字段中。Truncator 的 `Truncate()` 应只加 metadata 标记（不删 Content），`BuildRequest()` 做实际截断 + 标记"原文在 transcript 第 N 行"。temp file 完全没有必要。

**未决议**：这个改动是否放入当前迭代。

## 下一步

1. 确定 compact 标记在 transcript.jsonl 的 Record 结构（加 `RecordCompact` 类型 + `CompactFrom/To/Summary` 字段）
2. 实现 `Tale.MarkCompact()`：将 compact 标记追加到消息列表，供 `Unsaved()`→checkpoint→transcript.jsonl 统一落盘
3. 实现 `vault.Record` 的 compact 序列化/反序列化（`MessagesToRecords` + `recordsToMessages`）
4. 实现 `Tale.BuildRequest()` 的 compact 视图：跳过被覆盖的消息，插入摘要
5. 讨论 Truncator temp file 是否需要移除（改为 metadata 标记 + BuildRequest 时截断）
6. 考虑迭代闭环，生成 `docs/modules/helm/design.md` 更新

## 建议技能

- `two-axis-review`：代码审查
- `grow-doc`：迭代闭环时更新模块 design.md
- `verify`：端到端验证 compaction 行为
