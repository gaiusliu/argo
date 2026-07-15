package server

import "argo/src/knot"

type EventJSON struct {
	Type           string        `json:"type"`
	Delta          string        `json:"delta,omitempty"`
	ToolUse        *ToolUseJSON  `json:"toolUse,omitempty"`
	Usage          *UsageJSON    `json:"usage,omitempty"`
	DeniedToolUses []ToolUseJSON `json:"deniedToolUses,omitempty"`
	AskingToolUses []ToolUseJSON `json:"askingToolUses,omitempty"`
	Err            string        `json:"error,omitempty"`
}

type ToolUseJSON struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Parameters    map[string]any `json:"parameters"`
	Output        string         `json:"output,omitempty"`
	Status        int            `json:"status"`
	VerdictReason string         `json:"verdictReason,omitempty"`
	VerdictResult int            `json:"verdictResult,omitempty"`
}

type UsageJSON struct {
	Input   int `json:"input"`
	Output  int `json:"output"`
	Context int `json:"context,omitempty"`
}

func ToEventJSON(ev knot.Event) EventJSON {
	e := EventJSON{
		Type:           string(ev.Type),
		Delta:          ev.Delta,
		DeniedToolUses: toolUsesToJSON(ev.DeniedToolUses),
		AskingToolUses: toolUsesToJSON(ev.AskingToolUses),
	}
	if ev.Err != nil {
		e.Err = ev.Err.Error()
	}
	if ev.ToolUse.ID != "" {
		e.ToolUse = &ToolUseJSON{
			ID:         ev.ToolUse.ID,
			Name:       ev.ToolUse.Name,
			Parameters: ev.ToolUse.Parameters,
		}
		if ev.ToolUse.Result.Status != knot.StatusDefault {
			e.ToolUse.Output = ev.ToolUse.Result.Output
			e.ToolUse.Status = int(ev.ToolUse.Result.Status)
		}
	}
	if ev.Type == knot.EventDone || ev.Type == knot.EventTextDone {
		e.Usage = &UsageJSON{Input: ev.Usage.InputTokens, Output: ev.Usage.OutputTokens, Context: ev.Usage.ContextWindow}
	}
	return e
}

// toolUsesToJSON 将 []knot.ToolUse 转为可序列化的 []ToolUseJSON
func toolUsesToJSON(tus []knot.ToolUse) []ToolUseJSON {
	if len(tus) == 0 {
		return nil
	}
	result := make([]ToolUseJSON, len(tus))
	for i, tu := range tus {
		tj := ToolUseJSON{
			ID:            tu.ID,
			Name:          tu.Name,
			Parameters:    tu.Parameters,
			VerdictReason: tu.VerdictReason,
			VerdictResult: tu.VerdictResult,
		}
		if tu.Result.Status != knot.StatusDefault {
			tj.Output = tu.Result.Output
			tj.Status = int(tu.Result.Status)
		}
		result[i] = tj
	}
	return result
}
