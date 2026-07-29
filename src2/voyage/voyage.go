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

// Voyage 一次航程，持有所有域模块实例，自驱动执行五态循环。
type Voyage struct {
	phase      int
	sight      sight.Sight
	board      *board.Board
	journal    vault.Journal
	checkpoint vault.Checkpoint
	deck       *deck.Deck
	lore       *lore.Lore
	omens      []pact.Omen
	askOmens   []pact.Omen
	press      press.Press
	tokenUsage pact.TokenUsage
	msgs       []pact.Message
	ctx        context.Context
}

// New 装配一次航程。内部创建各域模块、会话目录、journal和checkpoint。
func New(ctx context.Context, cfg pact.AgentCfg) (*Voyage, error) {
	client := &http.Client{}
	d := deck.New(deck.BuiltinTools(), deck.NewClip(cfg.Clip.Strategy))
	hands := make([]deck.HandMeta, 0, len(d.List()))
	for _, h := range d.List() {
		hands = append(hands, deck.Describe(h))
	}
	s, err := sight.New(cfg.Sight, cfg.Sight.DefaultModel, hands, client)
	if err != nil {
		return nil, err
	}
	p, err := press.New(cfg.Press.Strategy, s)
	if err != nil {
		return nil, err
	}
	l := lore.New(argoHome()+"/skills", ".argo/skills")
	b := board.New(
		toBoardRules(cfg.Board.Iron),
		toBoardRules(cfg.Board.Cord),
	)
	dir := argoHome()
	return &Voyage{
		phase:      PhaseSight,
		ctx:        ctx,
		sight:      s,
		board:      b,
		journal:    vault.NewFileJournal(dir + "/journal.jsonl"),
		checkpoint: vault.NewFileCheckpoint(dir + "/checkpoint.json"),
		deck:       d,
		lore:       l,
		press:      p,
	}, nil
}

// toBoardRules 将 pact.BoardRule 列表转为 board.Rule 列表。
func toBoardRules(rules []pact.BoardRule) []board.Rule {
	out := make([]board.Rule, len(rules))
	for i, r := range rules {
		out[i] = board.Rule{
			Hand:    r.Hand,
			Pattern: r.Pattern,
			Action:  r.Action,
			Type:    r.Type,
		}
	}
	return out
}

// SetMessages 设置对话历史，供 sight.Take 使用。
func (v *Voyage) SetMessages(msgs []pact.Message) {
	v.msgs = msgs
}

// SetSail 扬帆起航，启动五态自驱动循环，返回流式事件 channel。
func (v *Voyage) SetSail() <-chan pact.Event {
	out := make(chan pact.Event, 8)
	go func() {
		defer close(out)
		for {
			switch v.phase {
			case PhaseSight:
				v.runSight(out)
			case PhaseRead:
				v.runRead(out)
			case PhaseCall:
				v.runCall(out)
			case PhaseHeave:
				v.runHeave(out)
			case PhaseDock:
				v.runDock(out)
				return
			}
		}
	}()
	return out
}
