package voyage

import "argo/src2/pact"

const codingBehaviorCore = `You are an expert software engineering assistant.
Complete tasks by reading files, executing commands, editing code, and creating files.
Be concise. If you don't know something, say so — don't make things up.`

// TODO：以下 phase runner 均为桩，后续填入真实逻辑

func (v *Voyage) runSight(out chan<- pact.Event) {
	msgs := v.buildSystemPrompt()
	msgs = append(msgs, v.msgs...)
	for ev := range v.sight.Take(v.ctx, msgs) {
		out <- ev
		if ev.Type == pact.EventTypeToolUseStart {
			v.omens = append(v.omens, ev.Omens...)
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
	// 桩：跳过 board.Read() 裁决
	v.phase = PhaseHeave
}

func (v *Voyage) runHeave(out chan<- pact.Event) {
	// 桩：跳过 deck.Haul() 执行
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
