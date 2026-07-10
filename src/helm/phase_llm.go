package helm

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"argo/src/deck"
	"argo/src/knot"
	"argo/src/reef"
	"argo/src/sail"
	"argo/src/vault"
	"argo/src/voyage"
)

// runPhaseLLM 执行 PhaseLLM：调用 LLM、消费流式响应、收集工具调用列表。
// 流式事件通过 emit 回调输出，完成后 st.Phase 变为 PhaseExecuteTools 或 PhaseDone。
// msgs 指向对话历史，Phase 间共享。
func runPhaseLLM(ctx context.Context, st *LoopState, session *voyage.Session, msgs *[]knot.Message, emit func(knot.Event)) {
	tools := deck.List()
	sp := buildSystemPrompt(tools)
	sysMsg := knot.Message{Role: knot.MessageRoleSystem, Content: sp}

	// 裁剪上下文，失败降级使用原始消息
	if compacted, err := reef.Compact(ctx, *msgs, sail.Window(st.ModelName)); err == nil {
		*msgs = compacted
	}

	// LLM 调用
	req := knot.ChatRequest{
		Messages: append([]knot.Message{sysMsg}, *msgs...),
		Tools:    tools,
	}

	llmCtx, cancel := context.WithTimeout(ctx, sail.ChatDefaultTimeout)
	defer cancel()

	sailEvents, err := sail.Chat(llmCtx, st.ModelName, req)
	if err != nil {
		slog.Error("chat failed", "error", err, "model", st.ModelName)
		emit(knot.Event{Type: knot.EventError, Err: err})
		st.Phase = PhaseDone
		return
	}

	// 流式消费 LLM 响应，收集文本 + 工具调用
	var toolUses []knot.ToolUse
	var textBuf strings.Builder
	for se := range sailEvents {
		if se.Type == knot.EventToolUseDelta && se.ToolUse.ID != "" {
			toolUses = append(toolUses, se.ToolUse)
			continue
		}
		if se.Type == knot.EventTextDelta {
			textBuf.WriteString(se.Delta)
		}
		emit(se)
	}

	// 无工具调用 → 写入 assistant 记录，PhaseDone
	if len(toolUses) == 0 {
		assistantMsg := knot.Message{Role: knot.MessageRoleAssistant, Content: textBuf.String()}
		*msgs = append(*msgs, assistantMsg)
		writeRecords(session, []knot.Message{assistantMsg}, nil, st.ModelName)
		st.Phase = PhaseDone
		return
	}

	// 有工具调用 → 记录 tool_calls，写入 assistant 记录，转入执行阶段
	toolCalls := make([]knot.ToolCall, len(toolUses))
	for i, tu := range toolUses {
		argsBytes, _ := json.Marshal(tu.Parameters)
		toolCalls[i] = knot.ToolCall{ID: tu.ID, Name: tu.Name, Arguments: string(argsBytes)}
	}
	assistantMsg := knot.Message{
		Role:      knot.MessageRoleAssistant,
		Content:   textBuf.String(),
		ToolCalls: toolCalls,
	}
	*msgs = append(*msgs, assistantMsg)
	writeRecords(session, []knot.Message{assistantMsg}, nil, st.ModelName)

	st.ToolUses = toolUses
	st.ToolUseIndex = 0
	st.Phase = PhaseExecuteTools
}

// writeRecords 将消息转为 Record 并立刻写入 vault。
// toolMeta 仅在消息类型为 tool 时需要，非 tool 消息传 nil 即可。
func writeRecords(session *voyage.Session, msgs []knot.Message, toolMeta map[string]vault.ToolRecordMeta, modelName string) {
	records := vault.MessagesToRecords(msgs, toolMeta, modelName)
	if err := session.Append(records); err != nil {
		slog.Error("vault 写入失败", "error", err)
	}
}
