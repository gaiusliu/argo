package voyage

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"argo/src2/board"
	"argo/src2/pact"
)

const codingBehaviorCore = `You are an expert software engineering assistant.
Complete tasks by reading files, executing commands, editing code, and creating files.
Be concise. If you don't know something, say so — don't make things up.`

func (v *Voyage) runSight() {
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
	var content strings.Builder
	for ev := range v.sight.Take(v.ctx, msgs) {
		v.out <- ev
		if ev.Type == pact.EventTypeTextDelta {
			content.WriteString(ev.Delta)
		}
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
	v.msgs = append(v.msgs, pact.Message{
		Role:      pact.RoleAssistant,
		Content:   content.String(),
		Omens:     v.omens,
		Model:     v.sight.Name(),
		Timestamp: time.Now().Format(time.RFC3339),
	})
	if len(v.omens) > 0 {
		v.phase = PhaseRead
	} else {
		v.phase = PhaseDock
	}
}

func (v *Voyage) runRead() {
	hasApproved := false
	needsAsking := false
	for i := range v.omens {
		o := &v.omens[i]
		if o.Verdict == pact.VerdictApprove || o.Verdict == pact.VerdictDeny {
			continue
		}
		switch action, _ := v.board.Read(*o); action {
		case board.ActionApprove:
			o.Verdict = pact.VerdictApprove
			hasApproved = true
		case board.ActionDeny:
			o.Verdict = pact.VerdictDeny
		default:
			o.Verdict = pact.VerdictAsk
			needsAsking = true
		}
	}
	if needsAsking {
		v.phase = PhaseSteer
	} else if hasApproved {
		v.phase = PhaseHeave
	} else {
		v.phase = PhaseSight
	}
}

func (v *Voyage) runHeave() {
	for i := range v.omens {
		o := &v.omens[i]
		if o.Verdict != pact.VerdictApprove {
			continue
		}
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

func (v *Voyage) runSteer() {
	needsAsking := false
	for _, o := range v.omens {
		if o.Verdict == pact.VerdictAsk {
			needsAsking = true
			break
		}
	}
	if needsAsking {
		v.phase = PhaseCall
		return
	}
	// Resume 后进入：根据审批结果决定下一阶段
	for _, o := range v.omens {
		if o.Verdict == pact.VerdictApprove {
			v.phase = PhaseHeave
			return
		}
	}
	v.phase = PhaseSight
}

// runCall 呼叫船长：持久化 journal 并发送 EventTypeAsk。
func (v *Voyage) runCall() {
	if v.journal != nil && v.journalIndex < len(v.msgs) {
		var records []string
		for _, m := range v.msgs[v.journalIndex:] {
			data, err := json.Marshal(m)
			if err != nil {
				slog.Error("journal marshal", "error", err)
				continue
			}
			records = append(records, string(data))
		}
		if err := v.journal.Append(records); err != nil {
			slog.Error("journal append", "error", err)
			v.out <- pact.Event{Type: pact.EventTypeError, Err: err}
		}
	}
	v.out <- pact.Event{
		Type:  pact.EventTypeAsk,
		Omens: v.omens,
	}
}

func (v *Voyage) runDock() {
	if v.journal != nil && v.journalIndex < len(v.msgs) {
		var records []string
		for _, m := range v.msgs[v.journalIndex:] {
			data, err := json.Marshal(m)
			if err != nil {
				slog.Error("journal marshal", "error", err)
				continue
			}
			records = append(records, string(data))
		}
		if err := v.journal.Append(records); err != nil {
			slog.Error("journal append", "error", err)
		} else {
			v.journalIndex = len(v.msgs)
		}
	}
	v.out <- pact.Event{Type: pact.EventTypeDone}
}

// buildSystemPrompt 组装 system prompt，包含角色设定、环境信息、技能列表、工具列表。
func (v *Voyage) buildSystemPrompt() []pact.Message {
	var sb strings.Builder
	sb.WriteString(codingBehaviorCore)
	sb.WriteString("\n\n")
	fmt.Fprintf(&sb, "Platform: %s\n", runtime.GOOS)
	fmt.Fprintf(&sb, "Today's date: %s\n\n", time.Now().Format("2006-01-02"))
	// 技能列表
	if skills := v.lore.List(); len(skills) > 0 {
		sb.WriteString("## Available Skills\n")
		for _, s := range skills {
			fmt.Fprintf(&sb, "- %s: %s\n", s.Name, s.Description)
		}
		sb.WriteString("\n")
	}
	// 工具列表
	if tools := v.deck.List(); len(tools) > 0 {
		sort.Slice(tools, func(i, j int) bool {
			return tools[i].Name() < tools[j].Name()
		})
		sb.WriteString("## Available Tools\n")
		for _, t := range tools {
			fmt.Fprintf(&sb, "- %s: %s\n", t.Name(), t.Description())
		}
		sb.WriteString("\n")
	}
	// ARGO.md 用户指令（不存在则跳过）
	if data, err := os.ReadFile(argoHome() + "/ARGO.md"); err == nil {
		sb.Write(data)
		sb.WriteByte('\n')
	}
	return []pact.Message{{
		Role:    pact.RoleSystem,
		Content: sb.String(),
	}}
}
