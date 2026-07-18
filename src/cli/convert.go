package main

import (
	"errors"

	"argo/src/knot"
	"argo/src/server"
)

// toKnotEvent 将 server.EventJSON 转回 knot.Event。
func toKnotEvent(ej server.EventJSON) knot.Event {
	ev := knot.Event{
		Type:  knot.EventType(ej.Type),
		Delta: ej.Delta,
	}
	if ej.Err != "" {
		ev.Err = errors.New(ej.Err)
	}

	for _, tu := range ej.AskingToolUses {
		ev.AskingToolUses = append(ev.AskingToolUses, toolUseFromJSON(tu))
	}
	for _, tu := range ej.DeniedToolUses {
		ev.DeniedToolUses = append(ev.DeniedToolUses, toolUseFromJSON(tu))
	}

	if ej.ToolUse != nil {
		ev.ToolUse = toolUseFromJSON(*ej.ToolUse)
	}
	if ej.Usage != nil {
		ev.Usage = knot.TokenUsage{
			InputTokens:   ej.Usage.Input,
			OutputTokens:  ej.Usage.Output,
			ContextWindow: ej.Usage.Context,
		}
	}
	return ev
}

// toolUseFromJSON 将 server.ToolUseJSON 转为 knot.ToolUse。
func toolUseFromJSON(tu server.ToolUseJSON) knot.ToolUse {
	return knot.ToolUse{
		ID:            tu.ID,
		Name:          tu.Name,
		Parameters:    tu.Parameters,
		VerdictReason: tu.VerdictReason,
		VerdictResult: tu.VerdictResult,
		Result: knot.ToolResult{
			Output: tu.Output,
			Status: knot.ResultStatus(tu.Status),
		},
	}
}
