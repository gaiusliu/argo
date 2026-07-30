package helm

import (
	"context"
	"fmt"
	"strings"

	"argo/src_old/knot"
	"argo/src_old/sail"
)

// LLM Summary 7-section 结构化模板，对齐 Hermes / Opencode 的实战格式
const summaryTemplate = `Output exactly the Markdown structure shown inside <template> and keep the section order unchanged. Do not include the <template> tags in your response.
<template>
## Goal
- [single-sentence task summary, or "(none)"]

## Constraints & Preferences
- [user constraints, preferences, specs, or "(none)"]

## Progress
### Done
- [completed work, or "(none)"]

### In Progress
- [current work, or "(none)"]

### Blocked
- [blockers, or "(none)"]

## Key Decisions
- [decision and why, or "(none)"]

## Next Steps
- [ordered next actions, or "(none)"]

## Critical Context
- [important technical facts, errors, open questions, or "(none)"]

## Relevant Files
- [file path: why it matters, or "(none)"]
</template>

Rules:
- Keep every section, even when empty. Write "(none)" for empty sections.
- Use terse bullets, not prose paragraphs.
- Preserve exact file paths, commands, error strings, and identifiers when known.
- Do not mention the summary process or that context was compacted.`

// 阈值及预算默认值
const (
	compactThreshold = 80    // 触发 compact 的上下文窗口百分比
	tailTokenBudget  = 8_000 // tail 保留的 token 预算
	compactHeadKept  = 2     // 头部保护的消息条数（primacy，不含 system prompt）
)

// LLMSummaryCompactor 用 LLM 将旧消息压缩为结构化摘要，
// 保护头部消息和尾部 token 预算内的最新消息。
type LLMSummaryCompactor struct {
	Provider sail.Provider
}

// ShouldCompact 判断是否需要触发 compact，仅检查阈值，不执行实际压缩。
func (c *LLMSummaryCompactor) ShouldCompact(inputTokens int) bool {
	maxTokens := sail.Window(c.Provider.Model()).MaxTokens
	return maxTokens > 0 && inputTokens >= maxTokens*compactThreshold/100
}

// Compact 判断是否需要压缩，需要时调用 LLM 生成摘要。
// 返回摘要正文 + 被覆盖的消息区间 [from, to)。未触发时 from == to == 0。
func (c *LLMSummaryCompactor) Compact(ctx context.Context, messages []knot.Message, inputTokens int) (string, int, int, error) {
	if !c.ShouldCompact(inputTokens) {
		return "", 0, 0, nil
	}

	from := compactHeadKept
	if len(messages) <= from {
		return "", 0, 0, nil
	}

	// 从尾部倒推，累计 token 到 tailTokenBudget 即为切割点
	to := len(messages)
	acc := 0
	for i := len(messages) - 1; i >= from; i-- {
		acc += len(messages[i].Content) / 4
		if acc >= tailTokenBudget {
			to = i
			break
		}
	}
	if to <= from {
		return "", 0, 0, nil
	}

	// 序列化待压缩消息 → 调用 LLM
	serialized := serializeForCompact(messages[from:to])
	summaryMsgs := []knot.Message{
		{Role: knot.MessageRoleSystem, Content: summaryTemplate},
		{Role: knot.MessageRoleUser, Content: serialized},
	}
	events := c.Provider.Chat(ctx, summaryMsgs, nil)

	var buf strings.Builder
	for evt := range events {
		if evt.Type == knot.EventTextDelta {
			buf.WriteString(evt.Delta)
		}
		if evt.Type == knot.EventError {
			return "", 0, 0, evt.Err
		}
	}

	summary := strings.TrimSpace(buf.String())
	if summary == "" {
		return "", 0, 0, nil
	}
	return summary, from, to, nil
}

// serializeForCompact 将消息序列化为紧凑文本，供 LLM 摘要输入。
// 对齐 Opencode：assistant 的 ToolCalls 与对应 tool 消息合并输出。
func serializeForCompact(messages []knot.Message) string {
	// 预建 tool 结果查找表，按 ToolCallID 配对
	results := make(map[string]string)
	for _, m := range messages {
		if m.Role == knot.MessageRoleTool {
			results[m.ToolCallID] = m.Content
		}
	}

	var b strings.Builder
	for _, m := range messages {
		switch m.Role {
		case knot.MessageRoleUser:
			fmt.Fprintf(&b, "[User]: %s\n\n", m.Content)
		case knot.MessageRoleAssistant:
			fmt.Fprintf(&b, "[Assistant]: %s\n", m.Content)
			for _, tc := range m.ToolCalls {
				fmt.Fprintf(&b, "[Tool call]: %s(%s)\n", tc.Name, tc.Arguments)
				if result, ok := results[tc.ID]; ok {
					fmt.Fprintf(&b, "[Tool result]: %s\n", result)
				}
			}
			b.WriteString("\n")
		case knot.MessageRoleTool:
			// 已在 assistant 的 ToolCalls 中合并输出，跳过
		case knot.MessageRoleSystem:
			fmt.Fprintf(&b, "[System]: %s\n\n", m.Content)
		}
	}
	return b.String()
}

func init() {
	RegisterCompactor("llm-summary", func(provider sail.Provider, opts map[string]any) (Compactor, error) {
		return &LLMSummaryCompactor{Provider: provider}, nil
	})
}
