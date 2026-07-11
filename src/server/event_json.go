package server

import "argo/src/knot"

// EventJSON knot.Event 的 SSE 序列化结构，不含 channel、error 等不可序列化字段。
type EventJSON struct {
	Type      string       `json:"type"`
	Delta     string       `json:"delta,omitempty"`
	ToolUse   *ToolUseJSON `json:"toolUse,omitempty"`
	Usage     *UsageJSON   `json:"usage,omitempty"`
	AskReason string       `json:"askReason,omitempty"`
	AskID     string       `json:"askID,omitempty"`
	Err       string       `json:"error,omitempty"`
}

type ToolUseJSON struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Parameters map[string]any `json:"parameters"`
	Output     string         `json:"output,omitempty"`
	Status     string         `json:"status"`
}

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

func ToEventJSON(ev knot.Event) EventJSON {
	e := EventJSON{
		Type:  string(ev.Type),
		Delta: ev.Delta,
		// AskReason: ev.AskReason,
		// AskID:     ev.AskID,
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
			e.ToolUse.Status = statusString(ev.ToolUse.Result.Status)
		}
	}
	if ev.Type == knot.EventDone {
		e.Usage = &UsageJSON{Input: ev.Usage.InputTokens, Output: ev.Usage.OutputTokens}
	}
	return e
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

func statusString(s knot.ResultStatus) string {
	switch s {
	case knot.StatusSuccess:
		return "success"
	case knot.StatusError:
		return "error"
	default:
		return "pending"
	}
}
