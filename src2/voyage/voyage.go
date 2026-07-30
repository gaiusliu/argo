package voyage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"argo/src2/board"
	"argo/src2/deck"
	"argo/src2/lore"
	"argo/src2/pact"
	"argo/src2/press"
	"argo/src2/sight"
	"argo/src2/vault"
)

// Voyage 一次航程，持有所有域模块实例，自驱动执行六态循环。
type Voyage struct {
	phase        int
	sight        sight.Sight
	board        *board.Board
	journal      vault.Journal
	deck         *deck.Deck
	lore         *lore.Lore
	omens        []pact.Omen
	press        press.Press
	tokenUsage   pact.TokenUsage
	msgs         []pact.Message
	journalIndex int
	ctx          context.Context
	out          chan<- pact.Event
}

// New 装配一次航程。内部创建各域模块、会话目录、journal和checkpoint。
func New(ctx context.Context, cfg AgentCfg) (*Voyage, error) {
	client := &http.Client{}
	d := deck.New(deck.BuiltinTools(), deck.NewClip(cfg.Clip))
	hands := make([]deck.HandMeta, 0, len(d.List()))
	for _, h := range d.List() {
		hands = append(hands, deck.HandMeta{
			Name:        h.Name(),
			Description: h.Description(),
			Parameters:  h.ParametersSchema(),
		})
	}
	s, err := sight.New(cfg.Sight, cfg.Sight.DefaultModel, hands, client)
	if err != nil {
		return nil, err
	}
	p, err := press.New(cfg.Press, s)
	if err != nil {
		return nil, err
	}
	l := lore.New(argoHome()+"/skills", ".argo/skills")
	b := board.New(cfg.Board.Iron, cfg.Board.Cord)
	dir := argoHome()
	return &Voyage{
		phase:   PhaseSight,
		ctx:     ctx,
		sight:   s,
		board:   b,
		journal: vault.NewFileJournal(dir + "/journal.jsonl"),
		deck:    d,
		lore:    l,
		press:   p,
	}, nil
}

// AppendUserMessage 追加用户消息到对话历史。
func (v *Voyage) AppendUserMessage(prompt string) {
	v.msgs = append(v.msgs, pact.Message{Role: pact.RoleUser, Content: prompt})
}

// Resume 恢复待审批会话，从最后一条 assistant 消息恢复 omens 并更新 verdict。
// decisions 的 key 为 Omen.ID，value 取值见 pact.VerdictApprove / pact.VerdictDeny。
func (v *Voyage) Resume(decisions map[string]int) error {
	if len(v.msgs) == 0 {
		return fmt.Errorf("resume: no message history")
	}
	last := v.msgs[len(v.msgs)-1]
	if last.Role != pact.RoleAssistant || len(last.Omens) == 0 {
		return fmt.Errorf("resume: last message is not assistant with omens")
	}
	v.omens = last.Omens
	for i := range v.omens {
		o := &v.omens[i]
		verdict, ok := decisions[o.ID]
		if !ok {
			continue
		}
		o.Verdict = verdict
		if verdict == pact.VerdictApprove {
			v.board.Pass(*o)
		}
	}
	v.phase = PhaseSteer
	return nil
}

// LoadMessages 从 journal 加载历史消息并赋值给 v.msgs。
func (v *Voyage) LoadMessages() error {
	lines, err := v.journal.Scan()
	if err != nil {
		return err
	}
	var msgs []pact.Message
	for _, line := range lines {
		var m pact.Message
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	v.msgs = msgs
	return nil
}

// SetSail 扬帆起航，启动六态自驱动循环，返回流式事件 channel。
func (v *Voyage) SetSail() <-chan pact.Event {
	out := make(chan pact.Event, 8)
	v.out = out
	go func() {
		defer close(out)
		for {
			switch v.phase {
			case PhaseSight:
				v.runSight()
			case PhaseRead:
				v.runRead()
			case PhaseSteer:
				v.runSteer()
			case PhaseCall:
				v.runCall()
				return
			case PhaseHeave:
				v.runHeave()
			case PhaseDock:
				v.runDock()
				return
			}
		}
	}()
	return out
}
