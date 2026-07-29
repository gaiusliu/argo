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
	s, err := sight.New(cfg.Sight, cfg.Sight.DefaultModel, nil, client)
	if err != nil {
		return nil, err
	}
	p, err := press.New(cfg.Press.Strategy, s)
	if err != nil {
		return nil, err
	}
	// TODO: 传入 builtin hands + clip
	d := deck.New(nil, nil)
	// TODO: 传入用户/项目技能目录
	l := lore.New("", "")
	// TODO: 传入规则
	b := board.New(nil, nil)
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
