package voyage

import (
	"context"
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

// SetMessages 设置对话历史，并自动检测是否需要恢复待审批会话。
func (v *Voyage) SetMessages(msgs []pact.Message) {
	v.msgs = msgs
	if len(msgs) == 0 {
		v.phase = PhaseSight
		return
	}
	last := msgs[len(msgs)-1]
	if last.Role != pact.RoleAssistant {
		v.phase = PhaseSight
		return
	}
	for _, o := range last.Omens {
		if o.Verdict == pact.VerdictAsk {
			v.omens = last.Omens
			v.phase = PhaseCall
			return
		}
	}
	v.phase = PhaseSight
}

// Resume 恢复待审批会话，根据用户审批结果更新 Omens 和 Board。
// decisions 的 key 为 Omen.ID，value 为 true 表示批准。
func (v *Voyage) Resume(decisions map[string]bool) {
	for i := range v.omens {
		o := &v.omens[i]
		approved, ok := decisions[o.ID]
		if !ok {
			continue
		}
		if approved {
			o.Verdict = pact.VerdictAllow
			v.board.Pass(*o)
		} else {
			o.Verdict = pact.VerdictDeny
		}
	}
	for _, o := range v.omens {
		if o.Verdict == pact.VerdictAllow {
			v.phase = PhaseHeave
			return
		}
	}
	v.phase = PhaseSight
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
