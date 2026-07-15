# MVP：策略接口抽取执行计划

## 背景

`reef.Compact()` 是桩。`deck/truncation.go` 的截断逻辑通过 `postProcess()` 硬编码在 `builtin_tool.go` / `cli_tool.go` 的 `Execute` 末尾。要支持策略可插拔、可配置，MVP 只做接口抽取——零行为变更。

## 目标

1. 建立 `Compactor` / `Truncator` 双接口 + 注册表
2. 现有逻辑迁入接口实现，不改行为
3. `config.json` 支持策略名称配置
4. 新策略接入：`struct + init()`，不动其他包

## 核心接口

```go
// —— 双接口 ——

type Compactor interface {
    Compact(ctx, messages, limit, summarize SummaryFunc) ([]Message, error)
}

type Truncator interface {
    Truncate(result ToolResult) ToolResult
}

// —— 解耦 LLM 依赖 ——

type SummaryFunc func(ctx, systemPrompt string, messages []Message) (string, error)

// —— 注册表 ——

func RegisterCompactor(name string, factory CompactorFactory)
func RegisterTruncator(name string, factory TruncatorFactory)
func ResolveCompactor(name, opts) (Compactor, error)   // ""/"passthrough" → 默认
func ResolveTruncator(name, opts) (Truncator, error)

// —— 配置 ——

type ContextConfig struct {
    Compact        string
    Truncation     string
    CompactOpts    map[string]any
    TruncationOpts map[string]any
}
```

## 注入方式

### Truncator：字段注入

`builtinTool` 结构体加 `truncator` 字段，构造时注入，`Execute` 末尾直接调：

```
改造前:
  builtinTool.Execute() → handler() → postProcess(result) → return

改造后:
  builtinTool.Execute() → handler() → truncator.Truncate(result) → return
```

```go
type builtinTool struct {
    ...                 // name / handler 等（不变）
    truncator reef.Truncator  // ← 构造时注入
}
```

CLI 工具同理。`RegisterTools(trunc)` 时传入。

### Compactor：字段注入

`PhaseRunnerModel` 加 `Compactor` 字段，和 `Provider` / `Tools` / `SystemPrompt` 同级：

```go
type PhaseRunnerModel struct {
    Provider     sail.Provider
    Tools        []knot.Tool
    SystemPrompt string
    Compactor    reef.Compactor   // ← 构造时注入
}
```

## 模块交互

```
config.json
  │
  ├─→ ResolveTruncator("head-tail") ──→ HeadTailTruncator
  │         │
  │         └──→ RegisterTools(trunc) ──→ builtinTool.truncator = trunc
  │                                              │
  │                                              └──→ Execute() 时调用 Truncate()
  │
  └─→ ResolveCompactor("passthrough") ──→ PassthroughCompactor
            │
            └──→ NewPhaseRunnerModel() 时注入
                     │
                     └──→ Phase 3 切换 "llm-summary"
```

## 包布局变化

```
新增:
  src/reef/strategy.go              ← 双接口 + SummaryFunc
  src/reef/registry.go              ← 注册表
  src/reef/types.go                 ← ToolIntent / Observation 预声明
  src/reef/compact_passthrough.go   ← 迁入现有 Compact 桩
  src/reef/truncation_headtail.go   ← 从 deck/truncation.go 迁入
  src/reef/truncation_passthrough.go

修改:
  src/knot/types.go                 ← 加 ContextConfig
  src/knot/config.go                ← 默认值
  src/deck/builtin_tool.go          ← postProcess → truncator.Truncate
  src/deck/cli_tool.go              ← 同上
  src/deck/tool_registry.go         ← RegisterTools 签名加 trunc 参数
  src/helm/phase_model.go           ← PhaseRunnerModel 加 Compactor 字段
  src/argo-server/main.go           ← Resolve → 注入 RegisterTools + helm

删除:
  src/reef/reef.go                  ← 被 compact_passthrough.go 替代
  src/deck/truncation.go            ← 迁入 reef/truncation_headtail.go
```

## 验证

- [ ] 编译通过，`reef` 不 import `sail` / `deck` / `helm`
- [ ] HeadTail 截断行为与改前一致
- [ ] `config.json` 切换 `passthrough` 可关闭截断
