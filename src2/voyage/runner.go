package voyage

import (
	"encoding/json"
	"log/slog"

	"argo/src2/board"
	"argo/src2/pact"
)

const codingBehaviorCore = `You are an expert software engineering assistant.
Complete tasks by reading files, executing commands, editing code, and creating files.
Be concise. If you don't know something, say so — don't make things up.`

// TODO：以下 phase runner 均为桩，后续填入真实逻辑

func (v *Voyage) runSight(out chan<- pact.Event) {
	if v.press != nil && v.press.Full(v.tokenUsage.InputTokens) {
		summary, from, to, err := v.press.Press(v.ctx, v.msgs, v.tokenUsage.InputTokens)
		if err != nil {
			slog.Error("press failed", "error", err)
		} else if summary != "" {
			v.msgs = append(v.msgs[:from],
				append([]pact.Message{{Role: pact.RoleCompact, Content: summary}}, v.msgs[to:]...)...)
		}
	}
	msgs := v.buildSystemPrompt()
	msgs = append(msgs, v.msgs...)
	for ev := range v.sight.Take(v.ctx, msgs) {
		out <- ev
		if ev.Type == pact.EventTypeToolUseStart {
			v.omens = append(v.omens, ev.Omens...)
		}
		if ev.Type == pact.EventTypeTextDone {
			v.tokenUsage = ev.Usage
		}
		if ev.Type == pact.EventTypeDone {
			break
		}
	}
	if len(v.omens) > 0 {
		v.phase = PhaseRead
	} else {
		v.phase = PhaseDock
	}
}

func (v *Voyage) runRead(out chan<- pact.Event) {
	v.askOmens = nil
	hasPass := false
	for i := range v.omens {
		o := &v.omens[i]
		switch action, _ := v.board.Read(*o); action {
		case board.ActionApprove:
			hasPass = true
		case board.ActionDeny:
			o.Verdict = pact.VerdictDeny
		default:
			v.askOmens = append(v.askOmens, *o)
		}
	}
	if len(v.askOmens) > 0 {
		v.phase = PhaseCall
	} else if hasPass {
		v.phase = PhaseHeave
	} else {
		v.phase = PhaseSight
	}
}

func (v *Voyage) runHeave(out chan<- pact.Event) {
	for i := range v.omens {
		o := &v.omens[i]
		hand, ok := v.deck.Lookup(o.Name)
		if !ok {
			o.Status = pact.StatusError
			o.Result = "tool not found: " + o.Name
			continue
		}
		var params map[string]any
		if err := json.Unmarshal([]byte(o.Arguments), &params); err != nil {
			o.Status = pact.StatusError
			o.Result = err.Error()
			continue
		}
		result, err := hand.Haul(params)
		if err != nil {
			o.Status = pact.StatusError
			o.Result = err.Error()
		} else {
			o.Status = pact.StatusSuccess
			o.Result = result
		}
	}
	v.phase = PhaseSight
}

func (v *Voyage) runCall(out chan<- pact.Event) {
	out <- pact.Event{
		Type:  pact.EventTypeAsk,
		Omens: v.askOmens,
	}
	// 持久化对话记录
	if v.journal != nil {
		var records []string
		for _, m := range v.msgs {
			records = append(records, m.Content)
		}
		_ = v.journal.Append(records)
	}
	// 持久化审批状态
	_ = v.board.Seal(v.checkpoint)
	// 持久化断点快照
	if v.checkpoint != nil {
		_ = v.checkpoint.Save("") // TODO: 序列化 Voyage 状态
	}
	v.phase = PhaseDock
}

func (v *Voyage) runDock(out chan<- pact.Event) {
	// 桩：跳过 journal.Append() 归档
	out <- pact.Event{Type: pact.EventTypeDone}
}

// buildSystemPrompt 组装 system prompt，包含角色设定、工具列表和技能列表。
func (v *Voyage) buildSystemPrompt() []pact.Message {
	// TODO: 补充环境信息、可用技能列表、工具列表、AGENTS.md
	return []pact.Message{{
		Role:    pact.RoleSystem,
		Content: codingBehaviorCore,
	}}
}
