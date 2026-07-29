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
	for ev := range v.sight.Take(v.ctx, msgs) {
		v.out <- ev
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

func (v *Voyage) runRead() {
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

func (v *Voyage) runHeave() {
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

func (v *Voyage) runCall() {
	v.out <- pact.Event{
		Type:  pact.EventTypeAsk,
		Omens: v.askOmens,
	}
	// 持久化对话记录
	if v.journal != nil {
		var records []string
		for _, m := range v.msgs {
			data, err := json.Marshal(m)
			if err != nil {
				slog.Error("journal marshal", "error", err)
				continue
			}
			records = append(records, string(data))
		}
		if err := v.journal.Append(records); err != nil {
			slog.Error("journal append", "error", err)
		}
	}
	// 持久化审批状态
	_ = v.board.Seal(v.checkpoint)
	// 持久化断点快照
	if v.checkpoint != nil {
		// TODO: 架构化存储 Voyage 状态
		_ = v.checkpoint.Save("")
	}
	v.phase = PhaseDock
}

func (v *Voyage) runDock() {
	if v.journal != nil {
		var records []string
		for _, m := range v.msgs {
			data, err := json.Marshal(m)
			if err != nil {
				slog.Error("journal marshal", "error", err)
				continue
			}
			records = append(records, string(data))
		}
		if err := v.journal.Append(records); err != nil {
			slog.Error("journal append", "error", err)
		}
	}
	if v.checkpoint != nil {
		// TODO: 序列化 Voyage 状态
		_ = v.checkpoint.Save("")
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
