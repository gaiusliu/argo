# reef 实现设计

## 概述

reef 提供 `Compact()` 函数，在每轮 LLM 调用前由 helm 调用。流程骨架由 Go 代码固化（触发判断、切割算法、文件 I/O、消息重组），摘要模板和维度分类等执行细节由 SKILL.md 定义。

## 内部结构

```
reef/
├── compact.go        // Compact() 入口 + 触发判断 + 切割算法 + 消息重组
├── prompt.go         // buildCompactPrompt() 从 SKILL 加载模板
├── types.go          // ReefDeps, CompactResult
└── reef_test.go
```

## 核心类型

```go
// ReefDeps — reef 消费的外部接口
type ReefDeps struct {
    Sail       LLMCaller        // 调 LLM 生成摘要（复用 sail 模块）
    Skills Skills // 加载 SKILL 摘要模板
}

type LLMCaller interface {
    Chat(ctx context.Context, req ModelRequest) (*ModelResponse, error)
}

type Skills interface {
    Load(ctx context.Context, name string) (string, error)
}

// CompactResult — 压缩产物
type CompactResult struct {
    Messages       []Message // 重组后的消息列表
    DidCompact     bool      // 是否实际执行了压缩
    TokensBefore   int
    TokensAfter    int
}
```

## 函数签名

```go
// Compact — 入口。判断是否需要压缩，如需则执行完整流程
func Compact(ctx context.Context, deps ReefDeps, messages []Message, window TokenLimit) (CompactResult, error)

// shouldCompact — 触发判定
// 默认 80% 阈值，基于调研数据取整（Claude-Code ~83.5%，Codex CLI ~81%）
// 可配置，非硬性参数
func shouldCompact(messages []Message, window TokenLimit) bool

// findCutPoint — 从后往前扫描消息，找到安全切割位置
// 返回 head（需要压缩的消息）和 tail（完整保留的消息）
func findCutPoint(messages []Message, keepTokens int) (head []Message, tail []Message)

// buildCompactPrompt — 从 SKILL 加载模板（结构见 compact-template.md）+ 拼接头部消息
func buildCompactPrompt(ctx context.Context, deps ReefDeps, head []Message) (string, error)

// buildCompactedMessages — 重组：SUMMARY_PREFIX + 摘要 + 尾部消息
func buildCompactedMessages(summary string, tail []Message) []Message

```

## 数据流

```
helm 调用 Compact(messages, window)
  │
  ├── 1. shouldCompact()
  │     estimateTokens(messages) <= 80% * window.MaxTokens → 原样返回，DidCompact=false
  │
  ├── 2. findCutPoint()
  │     从后往前累加 token → 到达保留预算时找安全切割位置
  │     默认 ~40K（Claude-Code max，通用 Agent 轮次较稀疏），可配置
  │     → head（旧消息，进压缩）+ tail（最近消息，完整保留）
  │
  ├── 3. buildCompactPrompt()
  │     deps.Crew.Load("reef-compact") → 摘要模板
  │     拼接：head 消息 + 模板 → 完整 prompt
  │
  ├── 4. deps.Sail.Chat()（优先默认模型，失败重试，最后兜底备用模型）
  │     ├── 鉴权失败 / token 额度耗尽 → 显式通知用户，等待回应
  │     ├── 其他错误 → 等待 1s → 重试（同模型）
  │     ├── 重试仍失败 → 切换备用模型/备用厂商
  │     └── 备用模型也失败 → 硬截断兜底
  │
  └── 5. buildCompactedMessages()
        [系统消息: SUMMARY_PREFIX + summary] + tail
        → 返回 CompactResult{Messages, DidCompact=true}
```
