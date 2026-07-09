package main

import (
	"errors"

	"argo/src/knot"
	"argo/src/server"
)

// toKnotEvent 将 server.EventJSON 转回 knot.Event。
func toKnotEvent(ej server.EventJSON) knot.Event {
	ev := knot.Event{
		Type:      knot.EventType(ej.Type),
		Delta:     ej.Delta,
		AskReason: ej.AskReason,
		AskID:     ej.AskID,
	}
	if ej.Err != "" {
		ev.Err = errors.New(ej.Err)
	}
	if ej.ToolUse != nil {
		ev.ToolUse = knot.ToolUse{
			ID:         ej.ToolUse.ID,
			Name:       ej.ToolUse.Name,
			Parameters: ej.ToolUse.Parameters,
			Result: knot.ToolResult{
				Output: ej.ToolUse.Output,
				Status: parseStatus(ej.ToolUse.Status),
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
