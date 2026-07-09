package helm

import (
	"context"
	"log/slog"

	"argo/src/brig"
	"argo/src/deck"
	"argo/src/knot"
	"argo/src/vault"
	"argo/src/voyage"
)

// runPhaseExecuteTools 从 st.ToolUseIndex 开始逐条门控并执行工具调用。
// 流式事件通过 emit 回调输出。遇 Ask 时设置 PhaseWaitAsk 并返回 false。
// msgs 指向对话历史，Phase 间共享。
func runPhaseExecuteTools(ctx context.Context, st *LoopState, session *voyage.Session, msgs *[]knot.Message, emit func(knot.Event)) bool {
	for i := st.ToolUseIndex; i < len(st.ToolUses); i++ {
		tu := &st.ToolUses[i]

		tool, found := deck.Lookup(tu.Name)
		if !found {
			tu.Result = knot.ToolResult{Status: knot.StatusError, Output: "tool not found: " + tu.Name}
			emit(knot.Event{Type: knot.EventToolUseDone, ToolUse: *tu})

			// 立刻写入 vault
			appendToolResult(msgs, session, *tu, vault.VerdictAutoDeny, st.ModelName)
			continue
		}

		// brig 门控：三个出口 —— Deny / Ask / Allow
		verdict, reason := session.Guard.Evaluate(*tu)
		switch verdict {
		case brig.Deny:
			tu.Result = knot.ToolResult{Status: knot.StatusError, Output: reason}
			emit(knot.Event{Type: knot.EventToolUseDone, ToolUse: *tu})
			appendToolResult(msgs, session, *tu, vault.VerdictAutoDeny, st.ModelName)

		case brig.Ask:
			st.ToolUseIndex = i
			st.AskReason = reason
			st.AskID = knot.NewAskID()
			st.Phase = PhaseWaitAsk
			emit(knot.Event{Type: knot.EventAsk, ToolUse: *tu, AskReason: reason, AskID: st.AskID})
			return false

		case brig.Allow:
			v := vault.VerdictAutoAllow
			if session.Guard.IsCachedApproval(*tu) {
				v = vault.VerdictCachedAllow
			}
			emit(knot.Event{Type: knot.EventToolUseStart, ToolUse: *tu})
			slog.Info("执行工具", "tool", tu.Name, "session id", session.ID, "cwd", session.WorkingDir)

			result, err := tool.Execute(ctx, tu.Parameters)
			if err != nil {
				slog.Error("工具执行失败", "tool", tu.Name, "error", err)
				tu.Result = knot.ToolResult{Status: knot.StatusError, Output: tu.Name + " failed: " + err.Error()}
			} else {
				tu.Result = result
			}
			emit(knot.Event{Type: knot.EventToolUseDone, ToolUse: *tu})
			appendToolResult(msgs, session, *tu, v, st.ModelName)
		}
	}

	st.Phase = PhaseLLM
	return true
}

// appendToolResult 将单条工具执行结果立刻写入 vault。
func appendToolResult(msgs *[]knot.Message, session *voyage.Session, tu knot.ToolUse, verdict vault.ToolVerdict, modelName string) {
	status := "error"
	if tu.Result.Status == knot.StatusSuccess {
		status = "success"
	}

	toolMsg := knot.Message{
		Role:       knot.MessageRoleTool,
		Content:    tu.Result.Output,
		ToolCallID: tu.ID,
	}
	*msgs = append(*msgs, toolMsg)

	meta := map[string]vault.ToolRecordMeta{
		tu.ID: {Name: tu.Name, Verdict: verdict, Status: status},
	}
	records := vault.MessagesToRecords([]knot.Message{toolMsg}, meta, modelName)
	if err := session.Append(records); err != nil {
		slog.Error("vault 写入 tool record 失败", "error", err)
	}
}
