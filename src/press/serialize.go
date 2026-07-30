package press

import (
	"strings"

	"argo/src/pact"
)

const (
	// compactThreshold 触发压缩的上下文窗口百分比
	compactThreshold = 80
	// compactHeadKept 压缩时保留的头部消息数
	compactHeadKept = 2
	// tailTokenBudget 压缩时保留的尾部 token 预算（估算值）
	tailTokenBudget = 8_000
)

// serialize 将消息序列化为紧凑文本供 LLM 摘要输入。
func serialize(msgs []pact.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case pact.RoleUser:
			b.WriteString("[User]: " + m.Content + "\n\n")
		case pact.RoleAssistant:
			b.WriteString("[Assistant]: " + m.Content + "\n")
			for _, o := range m.Omens {
				b.WriteString("[Hand call]: " + o.Name + "(" + o.Arguments + ")\n")
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// summaryTemplate 结构化摘要系统提示，含指令、模板、规则。
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
