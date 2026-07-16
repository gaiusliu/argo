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
| 2026-07-16 | 新增：compact 标记持久化 + BuildRequest compact 视图（剩余工作） |

---

## Phase 2：Compact 标记持久化 + BuildRequest 应用

### 背景

Phase 1 完成了 LLMSummaryCompactor（`Compact()` 返回 `(summary, from, to, err)`），但 `Tale.MarkCompact()` 是空桩，`BuildRequest()` 不做 compact 处理。当前 `Run()` 手写消息拼接作为临时方案，重启后 compact 信息丢失。

### 核心设计决策

compact 标记直接作为 `knot.Message{Role: "compact"}` 存入 Tale.messages。**不加单独列表，不加额外游标**。Tale.Unsaved() 自然包含 compact 消息，checkpoint/Vault.Load/AppendMessages/Tale.New 签名全部不变。

### 已确定决策

- DEC-041：compact 原文保留在 transcript.jsonl 中，compact 以新增 `RecordCompact` 类型与 user/model/tool 并列。

---

## Layer 1：架构设计（确认于 2026-07-16）

### 受影响模块

| 模块 | 变更角色 | 说明 |
|------|---------|------|
| **knot** | 修改 | Message 结构体新增 `CompactFrom`/`CompactTo` 字段 + `MessageRoleCompact` 角色常量 |
| **vault** | 修改 | 新增 `RecordCompact` 类型；Record 结构体增加 compact 字段；MessagesToRecords / recordsToMessages 各加一个 case |
| **tale** | 修改 | `MarkCompact()` 实现（往 messages 追加 compact 消息）；`BuildRequest()` 实现 compact 视图（按时间顺序应用 compact） |
| **helm** | 修改 | `Run()` 移除手动 splice；Compactor 输入改为 `Tale.BuildRequest("")`（压缩视图） |
| **Vault 接口 / Voyage / Server** | 不变 | AppendMessages、Load、New 签名全部不变 |

### 模块关系变更

```
改造前：
  tale ──BuildRequest()──→ helm (Run)
  tale ──Unsaved()───────→ helm (checkpoint) ──→ voyage ──→ vault

改造后：
  tale ──BuildRequest("")───→ helm (Run)        ← Compactor 用压缩视图
  tale ──BuildRequest(sp)───→ helm (Run)        ← LLM 用压缩视图（含 system prompt）
  tale ──MarkCompact()──────→ helm (Run)        ← 实现（追加 compact 消息）
  tale ──Unsaved()──────────→ helm (checkpoint) ← 自动包含 compact 消息，不改
         │                              │
         └──────────────────────────────┘
                                        ↓
                                   voyage ──→ vault
                                            │
                                  transcript.jsonl
                                  [user, model, tool, compact]
                                            │
                                  Load() → []Message  ← 签名不变
                                            │
                                  Tale.New(msgs, "")  ← 签名不变
```

### 数据流变更

```
【写入路径】
Run() 中:
  compactView = Tale.BuildRequest("")                  ← 压缩视图（不含 system prompt）
  summary, from, to = Compactor.Compact(ctx, compactView, inputTokens)
    → Compactor 看到的是已应用之前 compact 的压缩视图
    → 返回的 from/to 对应当前压缩视图的索引
MarkCompact(from, to, summary)
  → t.messages 追加 Message{Role:compact, Content:summary, CompactFrom:from, CompactTo:to}
  → Unsaved() 自动包含该 compact 消息                    ← 零改动
  → checkpoint() → AppendMessages()                      ← 零改动
  → MessagesToRecords() 加 case MessageRoleCompact      → Record{Type:compact, ...}
  → 写入 transcript.jsonl

【读取路径】
Vault.Load()                                            ← 签名不变
  → recordsToMessages() 加 case RecordCompact
    → Message{Role:compact, Content:..., CompactFrom:..., CompactTo:...}
  → Tale.New(msgs, "")                                  ← 签名不变

【LLM 请求 / Compactor 输入路径（同一函数）】
BuildRequest(systemPrompt)
  → 取 messages 中非 compact 角色的消息
  → 按时间顺序依次应用每个 compact（C1 → C2 → ...）
      每个 compact 的 from/to 对应当前中间结果的索引
  → 前置 system prompt（如提供）
  → 返回压缩后的消息列表
```

### 前提假设

- ⚠️ **Compactor 输入为压缩视图**：`Run()` 调用 `Tale.BuildRequest("")`（无 system prompt）作为 Compactor 输入。这与 PI、Opencode 的做法一致——Compactor 看到的是已应用之前所有 compact 的压缩视图。Compactor 的 `compactHeadKept` 从 `3` 改为 `2`（无 system prompt）。

- ⚠️ **compact 索引语义**：每个 compact 的 from/to 是相对于「该 compact 创建时的压缩视图」的索引。BuildRequest 按时间顺序应用 compact，每个 compact 作用于前一步的结果，索引自然匹配。这与 PI 的 `buildSessionContext` 行为一致。

- ⚠️ **Message 结构体的 omitempty 保证**：`CompactFrom`/`CompactTo` 带 `omitempty`，非 compact 消息序列化时不出现在 JSON。`ToolCallID` 已经是这种模式——只对 tool 角色有意义。

- ⚠️ **旧 transcript 兼容**：旧文件中无 RecordCompact 条目。`recordsToMessages` 的 switch 对未知类型跳过，行为正确。

### 范围边界

**包含：**
- knot.Message 新增 `CompactFrom`/`CompactTo` 字段 + `MessageRoleCompact`
- vault.Record 新增 `RecordCompact` + compact 字段
- vault.MessagesToRecords / recordsToMessages 各加一个 case
- Tale.MarkCompact() 实现
- Tale.BuildRequest() 按时间顺序应用 compact
- helm Run() Compactor 输入改为压缩视图 + 移除手动 splice

**不包含：**
- Truncator temp file 移除
- 增量摘要
- 客户端 compact 可视化

---

## Layer 2：接口设计（确认于 2026-07-16）

### 关键接口变更

**knot/types.go — Message 结构体**

```go
const MessageRoleCompact MessageRole = "compact"  // 新增

type Message struct {
    Role        MessageRole
    Content     string
    ToolCallID  string
    ToolCalls   []ToolCall
    CompactFrom int  `json:"compact_from,omitempty"`  // 新增
    CompactTo   int  `json:"compact_to,omitempty"`    // 新增
}
```

**vault/record.go — Record 结构体**

```go
const RecordCompact string = "compact"  // 新增

// Record 新增字段:
CompactFrom    int    `json:"compact_from,omitempty"`    // 新增
CompactTo      int    `json:"compact_to,omitempty"`      // 新增
CompactSummary string `json:"compact_summary,omitempty"` // 新增
```

**tale/tale.go — 2 个方法变更**

```go
// MarkCompact 实现（替换空桩）
func (t *Tale) MarkCompact(from, to int, summary string)

// BuildRequest 重写（按时间顺序应用 compact 视图）
func (t *Tale) BuildRequest(systemPrompt string) []knot.Message
```

**Vault 接口 — 不变**

```go
type Vault interface {
    Append(records []Record) error
    Load() ([]knot.Message, error)   // 签名不变
}
```

**Voyage.AppendMessages — 不变**

**Tale.New — 不变**

### 核心数据结构

无新增结构体。唯一的类型扩展是 `knot.Message` 和 `vault.Record` 各加几个可选字段。

### 关键流程

**流程 1：MarkCompact**

```
输入: from (int), to (int), summary (string)
前提: from/to 基于 Tale 内部 messages 索引（不含 system prompt）

1. t.messages = append(t.messages, Message{
       Role:        compact,
       Content:     summary,    // LLM 生成的 Markdown 摘要
       CompactFrom: from,
       CompactTo:   to,
   })
```

**流程 2：BuildRequest（同时为 Compactor 和 LLM 服务）**

```
输入: systemPrompt (string)，为空时不含 system prompt（供 Compactor 用）
输出: []Message

1. 取 t.messages 中非 compact 角色的消息作为 result
2. 按 compact 在 t.messages 中的顺序（时间顺序）依次应用:
     - result = result[:from] + [{Role:system, Content:summary}] + result[to:]
     - 每个 compact 的 from/to 对应当前 result 的索引
3. 如果 systemPrompt 非空，前置 {Role:system, Content:systemPrompt}
4. 返回 result
```

**流程 3：Run() 中的 compact 触发**

```
1. compactView = Tale.BuildRequest("")                    // 压缩视图，不含 system prompt
2. summary, from, to, err = Compactor.Compact(ctx, compactView, inputTokens)
3. if triggered:
     Tale.MarkCompact(from, to, summary)                   // from/to 对应当前压缩视图
4. msgs = Tale.BuildRequest(pr.SystemPrompt)               // LLM 用，含 system prompt
5. Provider.Chat(ctx, msgs, tools)
```

Compactor 内部 `compactHeadKept = 2`（无 system prompt）。

**流程 4：MessagesToRecords（写入）**

```
在现有 switch msg.Role 中新增:
  case MessageRoleCompact:
      → Record{Type: RecordCompact, CompactFrom, CompactTo,
               CompactSummary: msg.Content}
```

**流程 5：recordsToMessages（读取）**

```
在现有 switch r.Type 中新增:
  case RecordCompact:
      → Message{Role: compact, Content: r.CompactSummary,
                CompactFrom: r.CompactFrom, CompactTo: r.CompactTo}
```

### 索引验证（含两轮 compact）

```
第一轮:
  Tale.messages = [U1, A1, U2, A2, T1, U3, A3, U4]
  Compactor 输入 = [U1, A1, U2, A2, T1, U3, A3, U4]    // 无历史 compact
  → 返回 from=2, to=6
  → MarkCompact(2, 6, "S1")
  Tale.messages = [U1, A1, U2, A2, T1, U3, A3, U4, C1(2,6,"S1")]

第二轮（又聊了 5 轮后）:
  Tale.messages = [U1, A1, U2, A2, T1, U3, A3, U4, C1, U5, A5, T2, U6, A6]
  Compactor 输入 = BuildRequest("")
    = [U1, A1, S1, U5, A5, T2, U6, A6]                 // C1 已应用
    索引:   0   1   2   3   4   5   6   7
  → 返回 from=2, to=6
  → MarkCompact(2, 6, "S2")
  Tale.messages = [U1, A1, U2, A2, T1, U3, A3, U4, C1, U5, A5, T2, U6, A6, C2(2,6,"S2")]

BuildRequest 最终输出（systemPrompt="SP"）:
  result = [U1, A1, U2, A2, T1, U3, A3, U4, U5, A5, T2, U6, A6]  // 非 compact
  应用 C1: result = [U1, A1, S1, U5, A5, T2, U6, A6]              // C1.from=2,to=6
  应用 C2: result = [U1, A1, S2, A6]                              // C2.from=2,to=6
  前置 SP: [SP, U1, A1, S2, A6]                                    ✅
```

### 变更预估

| 文件 | 新增行数 |
|------|---------|
| knot/types.go | ~5 |
| vault/record.go | ~8 |
| vault/convert.go | ~10 |
| vault/file_vault.go | ~6 |
| tale/tale.go | ~30 |
| helm/compact_llm_summary.go | ~2 |
| helm/phase_model.go | ~10 |
| **合计** | **~71** |

单文件均 ≤30 行。
