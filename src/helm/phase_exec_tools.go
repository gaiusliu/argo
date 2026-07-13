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

func (pr *PhaseRunnerExecTools) Run(ctx context.Context,
	v *voyage.Voyage, emit func(knot.Event)) bool {
	slog.Info("phase exec tools", "session id", v.ID())

	var deniedToolUses []knot.ToolUse
	var askingToolUses []knot.ToolUse

	// 需审核工具调用
	for i, tu := range v.ToolUses {
		if tu.VerdictResult != brig.Default {
			continue
		}

		verdict, reason := v.Evaluate(tu)
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
		v.ToolUses[i] = tu
	}

	if len(deniedToolUses) > 0 {
		emit(knot.Event{Type: knot.EventAsk, DeniedToolUses: deniedToolUses})
	}

	if len(askingToolUses) > 0 {
		v.Phase = voyage.PhaseAsking
		emit(knot.Event{Type: knot.EventAsk, AskingToolUses: askingToolUses})
		slog.Info("tool uses need to be asked", "asking tool uses", askingToolUses)
		return false
	}

	// 执行工具阶段
	slog.Info("executing tools", "phase", v.Phase, "num of tool calls", len(v.ToolUses))
	for i, tu := range v.ToolUses {
		if tu.VerdictResult != brig.Approve &&
			tu.VerdictResult != brig.UserApprove {
			continue
		}

		if tu.VerdictResult == brig.UserApprove {
			v.Approve(tu)
		}

		emit(knot.Event{Type: knot.EventToolUseStart, ToolUse: tu})
		slog.Info("执行工具", "tool", tu.Name, "session id", v.ID(), "cwd", v.WorkingDir())

		tool, found := deck.Lookup(tu.Name)
		if !found {
			tu.Result = knot.ToolResult{Status: knot.StatusError, Output: "tool not found: " + tu.Name}
			emit(knot.Event{Type: knot.EventToolUseDone, ToolUse: tu})
			v.ToolUses[i] = tu
			continue
		}
		result, err := tool.Execute(ctx, tu.Parameters)
		if err != nil {
			slog.Error("工具执行失败", "tool", tu.Name, "error", err)
			tu.Result = knot.ToolResult{Status: knot.StatusError, Output: tu.Name + " failed: " + err.Error()}
			emit(knot.Event{Type: knot.EventToolUseDone, ToolUse: tu})
			v.ToolUses[i] = tu
			continue
		}

		tu.Result = result
		emit(knot.Event{Type: knot.EventToolUseDone, ToolUse: tu})
		v.ToolUses[i] = tu
	}

	// 将工具执行结果追加到对话历史
	for i, tu := range v.ToolUses {
		if tu.VerdictResult == brig.Ask {
			continue
		}

		if tu.Result.Status == knot.StatusDefault &&
			(tu.VerdictResult == brig.Deny ||
				tu.VerdictResult == brig.UserDeny) {
			tu.Result = knot.ToolResult{
				Status: knot.StatusError,
				Output: "tool execution denied: " + tu.VerdictReason,
			}
			v.ToolUses[i] = tu
		}

		v.Tale().AppendTool(tu.ID, tu.Result.Output)
		slog.Info("new message generated", "toolCallID", tu.ID, "content", tu.Result.Output)
	}

	v.Phase = voyage.PhaseModel
	return false
}
