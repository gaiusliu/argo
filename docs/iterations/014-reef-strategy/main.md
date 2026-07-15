# 014：reef 策略接口抽取（Compactor / Truncator MVP）

**日期**：2026-07-15 ~ 2026-07-15
**状态**：✅ 已完成
**涉及模块**：reef（重构）、deck、helm、knot

## 目标

将 `reef.Compact()` 桩和 `deck/truncation.go` 的硬编码截断逻辑抽取为 `Compactor` / `Truncator` 双接口 + 注册表 + 字段注入。各模块内部通过 `knot.GetConfig()` → `reef.Resolve*()` 自行解析策略，不通过调用链参数传递。

## 范围

### 包含

- **接口 + 注册表** — Compactor / Truncator 接口定义、工厂注册表、Resolve 降级
- **策略实现** — PassthroughCompactor（迁入 Compact 桩）、HeadTailTruncator（从 deck/truncation.go 迁入）、PassthroughTruncator
- **ContextConfig 配置** — knot.Config 新增 context 段（compact、compactThreshold、truncation、*Opts），支持 config.json 驱动策略选择
- **Truncator 注入到 deck** — builtinTool + cliTool 加 truncator 字段，`RegisterTools()` 内部 resolveTruncator()
- **Compactor 注入到 helm** — PhaseRunnerModel 加 Compactor 字段，`NewPhaseRunnerModel()` 内部 resolveCompactor()

### 不含

- ToolIntent → Observation 意图驱动提取（Phase 2）
- LLM Summary 压缩实现（Phase 3，格式约定已确定，见下文）
- 自适应预算 + deferred_lead 三级处理（Phase 3）
- 断路器等降级机制（Phase 3）

## 设计决策

### 默认策略：llm-summary

调研 Codex CLI、Claude Code、Hermes Agent、OpenClaw、PI、Kilocode、Opencode，全部 7 个 agent 的默认 compact 策略均为 **LLM Summarization**。Argo 默认 Compact 从 `"passthrough"` 改为 `"llm-summary"`。

`"llm-summary"` 当前未实现，`resolveCompactor()` 在工厂缺失时回退 passthrough + warn 日志。Phase 3 实现后只需 `init()` 注册即可，配置无需改动。

### 默认阈值：80%

对齐 Claude Code。Hermes 发现 50% 太激进（每轮触发），80% 是经验证的平衡点。

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `context.compact` | `"llm-summary"` | 默认 compact 策略 |
| `context.compactThreshold` | `80` | 触发 compact 的上下文窗口百分比（1–100） |
| `context.truncation` | `"head-tail"` | 默认 truncation 策略 |

### Compactor / Truncator 内部自行解析

不通过 Server Config 或调用链参数传递策略实例。各包内部调用 `knot.GetConfig()` + `reef.Resolve*()`：

```
deck.RegisterTools() → resolveTruncator() → knot.GetConfig() → reef.ResolveTruncator()
helm.NewPhaseRunnerModel() → resolveCompactor() → knot.GetConfig() → reef.ResolveCompactor()
```

`argo-server/main.go` 和 `server/` 无需任何改动。

### LLM Summary 格式约定（Phase 3 实现依据）

复用 Hermes / Opencode 经实战验证的结构化模板，7 个固定 Section：

```
## Goal
- [单句任务目标]

## Constraints & Preferences
- [用户约束、偏好、技术栈要求，或 "(none)"]

## Progress
### Done
- [已完成的工作，或 "(none)"]
### In Progress
- [进行中的工作，或 "(none)"]
### Blocked
- [卡点，或 "(none)"]

## Key Decisions
- [技术决策及原因，或 "(none)"]

## Next Steps
- [按顺序的下一步行动，或 "(none)"]

## Critical Context
- [重要的技术事实、错误信息、未解决问题，或 "(none)"]

## Relevant Files
- [文件路径: 为什么重要，或 "(none)"]
```

关键约束：
- **bullet 而非段落**——紧贴 token 预算
- **保留原文字符串**——文件路径、错误信息、命令不做改写
- **每个 Section 必须出现**——空则写 `(none)`，防止 LLM 跳过
- **不提 compaction 过程本身**
- **迭代更新**——传上次摘要 + 新对话，指令是"更新摘要"，不从头生成

## 整体开发状态

| 功能 | 状态 | 关联文件 |
|------|------|---------|
| 1. 接口 + 注册表 | ✅ 完成 | `src/reef/strategy.go` `registry.go` `types.go` |
| 2. 策略实现 | ✅ 完成 | `src/reef/compact_passthrough.go` `truncation_headtail.go` `truncation_passthrough.go` |
| 3. ContextConfig 配置 | ✅ 完成 | `src/knot/types.go` `config.go` |
| 4. Truncator 注入到 deck | ✅ 完成 | `src/deck/builtin_tool.go` `cli_tool.go` `tool.go` |
| 5. Compactor 注入到 helm | ✅ 完成 | `src/helm/phase_model.go` |
| 6. 旧文件清理 | ✅ 完成 | 删除 `src/reef/reef.go` `src/deck/truncation.go` |

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-07-15 | LLM Summary 格式约定确定（7 段结构化模板，对齐 Hermes / Opencode） |
| 2026-07-15 | 默认策略调整：Compact `"passthrough"` → `"llm-summary"`，阈值 80%。各模块内部 `knot.GetConfig()` 自行 Resolve |
| 2026-07-15 | 全部功能完成。接口 + 注册表 + 策略实现 + 注入 + 旧文件清理 |
| 2026-07-15 | 迭代开始 |
