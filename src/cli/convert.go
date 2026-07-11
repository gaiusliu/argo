package main

import (
	"errors"

	"argo/src/knot"
	"argo/src/server"
)

// toKnotEvent 将 server.EventJSON 转回 knot.Event。
func toKnotEvent(ej server.EventJSONNew) knot.Event {
	ev := knot.Event{
		Type:  knot.EventType(ej.Type),
		Delta: ej.Delta,
	}
	if ej.Err != "" {
		ev.Err = errors.New(ej.Err)
	}

	for _, tu := range ej.AskingToolUses {
		ev.AskingToolUses = append(ev.AskingToolUses, knot.ToolUse{
			ID:            tu.ID,
			Name:          tu.Name,
			Parameters:    tu.Parameters,
			VerdictReason: tu.VerdictReason,
			VerdictResult: tu.VerdictResult,
			Result: knot.ToolResult{
				Output: tu.Output,
				Status: knot.ResultStatus(tu.Status),
			},
		})
	}

	for _, tu := range ej.DeniedToolUses {
		ev.DeniedToolUses = append(ev.DeniedToolUses, knot.ToolUse{
			ID:            tu.ID,
			Name:          tu.Name,
			Parameters:    tu.Parameters,
			VerdictReason: tu.VerdictReason,
			VerdictResult: tu.VerdictResult,
			Result: knot.ToolResult{
				Output: tu.Output,
				Status: knot.ResultStatus(tu.Status),
			},
		})
	}

	if ej.ToolUse != nil {
		ev.ToolUse = knot.ToolUse{
			ID:         ej.ToolUse.ID,
			Name:       ej.ToolUse.Name,
			Parameters: ej.ToolUse.Parameters,
			Result: knot.ToolResult{
				Output: ej.ToolUse.Output,
				Status: knot.ResultStatus(ej.ToolUse.Status),
			},
		}
	}
	if ej.Usage != nil {
		ev.Usage = knot.TokenUsage{
			InputTokens:  ej.Usage.Input,
			OutputTokens: ej.Usage.Output,
		}
	}
	return ev
}

// parseStatus 将 SSE 中的状态字符串转回 knot.ResultStatus。
func parseStatus(s string) knot.ResultStatus {
	switch s {
	case "success":
		return knot.StatusSuccess
	case "error":
		return knot.StatusError
	default:
		return knot.StatusDefault
	}
}
