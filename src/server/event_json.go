package server

import "argo/src/knot"

type EventJSONNew struct {
	Type           string           `json:"type"`
	Delta          string           `json:"delta,omitempty"`
	ToolUse        *ToolUseJSONNew  `json:"toolUse,omitempty"`
	Usage          *UsageJSON       `json:"usage,omitempty"`
	DeniedToolUses []ToolUseJSONNew `json:"deniedToolUses,omitempty"`
	AskingToolUses []ToolUseJSONNew `json:"askingToolUses,omitempty"`
	Err            string           `json:"error,omitempty"`
}

type ToolUseJSONNew struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Parameters    map[string]any `json:"parameters"`
	Output        string         `json:"output,omitempty"`
	Status        int            `json:"status"`
	VerdictReason string         `json:"verdictReason,omitempty"`
	VerdictResult int            `json:"verdictResult,omitempty"`
}

type UsageJSON struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

func ToEventJSONNew(ev knot.Event) EventJSONNew {
	e := EventJSONNew{
		Type:           string(ev.Type),
		Delta:          ev.Delta,
		DeniedToolUses: toolUsesToJSON(ev.DeniedToolUses),
		AskingToolUses: toolUsesToJSON(ev.AskingToolUses),
	}
	if ev.Err != nil {
		e.Err = ev.Err.Error()
	}
	if ev.ToolUse.ID != "" {
		e.ToolUse = &ToolUseJSONNew{
			ID:         ev.ToolUse.ID,
			Name:       ev.ToolUse.Name,
			Parameters: ev.ToolUse.Parameters,
		}
		if ev.ToolUse.Result.Status != knot.StatusDefault {
			e.ToolUse.Output = ev.ToolUse.Result.Output
			e.ToolUse.Status = int(ev.ToolUse.Result.Status)
		}
	}
	if ev.Type == knot.EventDone {
		e.Usage = &UsageJSON{Input: ev.Usage.InputTokens, Output: ev.Usage.OutputTokens}
	}
	return e
}

// toolUsesToJSON 将 []knot.ToolUse 转为可序列化的 []ToolUseJSONNew
func toolUsesToJSON(tus []knot.ToolUse) []ToolUseJSONNew {
	if len(tus) == 0 {
		return nil
	}
	result := make([]ToolUseJSONNew, len(tus))
	for i, tu := range tus {
		tj := ToolUseJSONNew{
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

