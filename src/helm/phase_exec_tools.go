package helm

import (
	"argo/src/brig"
	"argo/src/deck"
	"argo/src/knot"
	"argo/src/voyage"
	"context"
	"log/slog"
)

type PhaseRunnerExecTools struct {
}

func (pr *PhaseRunnerExecTools) Run(ctx context.Context, st *HelmState, emit func(knot.Event), session voyage.Voyage) {
	slog.Info("phase exec tools", "session id", session.ID(), "helm state", st)

	var deniedToolUses []knot.ToolUse
	var askingToolUses []knot.ToolUse

	// 需审核工具调用
	for i, tu := range st.ToolUses {
		if tu.VerdictResult != brig.Default {
			continue
		}

		verdict, reason := session.Evaluate(tu)
		if verdict == brig.Deny {
			tu.VerdictResult = brig.Deny
			tu.VerdictReason = reason
			deniedToolUses = append(deniedToolUses, tu)
			tu.Result = knot.ToolResult{
				Status: knot.StatusError,
				Output: "tool execution denied: " + tu.VerdictReason,
			}
		} else if verdict == brig.Ask {
			tu.VerdictResult = brig.Ask
			tu.VerdictReason = reason
			askingToolUses = append(askingToolUses, tu)
		} else {
			tu.VerdictResult = brig.Approve
		}

		// 更新helm state的工具审批结果
		st.ToolUses[i] = tu
	}

	if len(deniedToolUses) > 0 {
		emit(knot.Event{Type: knot.EventAsk, DeniedToolUses: deniedToolUses})
	}

	if len(askingToolUses) > 0 {
		st.Phase = HelmPhaseAsking
		emit(knot.Event{Type: knot.EventAsk, AskingToolUses: askingToolUses})
		slog.Info("tool uses need to be asked", "asking tool uses", askingToolUses)
		return
	}

	// 执行工具阶段
	slog.Info("executing tools", "phase", st.Phase, "num of tool calls", len(st.ToolUses))
	for i, tu := range st.ToolUses {
		if tu.VerdictResult != brig.Approve &&
			tu.VerdictResult != brig.UserApprove {
			continue
		}

		if tu.VerdictResult == brig.UserApprove {
			session.Approve(tu)
		}

		emit(knot.Event{Type: knot.EventToolUseStart, ToolUse: tu})
		slog.Info("执行工具", "tool", tu.Name, "session id", session.ID(), "cwd", session.WorkingDir())

		tool, found := deck.Lookup(tu.Name)
		if !found {
			tu.Result = knot.ToolResult{Status: knot.StatusError, Output: "tool not found: " + tu.Name}
			emit(knot.Event{Type: knot.EventToolUseDone, ToolUse: tu})
			st.ToolUses[i] = tu
			continue
		}
		result, err := tool.Execute(ctx, tu.Parameters)
		if err != nil {
			slog.Error("工具执行失败", "tool", tu.Name, "error", err)
			tu.Result = knot.ToolResult{Status: knot.StatusError, Output: tu.Name + " failed: " + err.Error()}
			emit(knot.Event{Type: knot.EventToolUseDone, ToolUse: tu})
			st.ToolUses[i] = tu
			continue
		}

		tu.Result = result
		emit(knot.Event{Type: knot.EventToolUseDone, ToolUse: tu})
		st.ToolUses[i] = tu
	}

	// 将工具执行结果转为 tool role 消息，追加到对话历史
	for i, tu := range st.ToolUses {
		if tu.VerdictResult == brig.Ask {
			continue
		}

		// 记录用户拒绝的工具调用结果
		if tu.Result.Status == knot.StatusDefault &&
			(tu.VerdictResult == brig.Deny ||
				tu.VerdictResult == brig.UserDeny) {
			tu.Result = knot.ToolResult{
				Status: knot.StatusError,
				Output: "tool execution denied: " + tu.VerdictReason,
			}

			st.ToolUses[i] = tu
		}

		newMsg := knot.Message{
			Role:       knot.MessageRoleTool,
			ToolCallID: tu.ID,
			Content:    tu.Result.Output,
		}
		st.Messages = append(st.Messages, newMsg)
		slog.Info("new message generated", "msg", newMsg)
	}

	st.Phase = HelmPhaseModel
}
