package voyage

import (
	"context"

	"argo/src2/board"
	"argo/src2/deck"
	"argo/src2/lore"
	"argo/src2/pact"
	"argo/src2/sight"
	"argo/src2/vault"
)

// Voyage 一次航程，持有所有域模块实例，自驱动执行五态循环。
type Voyage struct {
	phase   int
	sight   sight.Sight
	board   *board.Board
	journal vault.Journal
	deck    *deck.Deck
	lore    *lore.Lore
	omens   []pact.Omen
	ctx     context.Context
}

// New 装配一次航程。
func New(ctx context.Context, s sight.Sight, b *board.Board, j vault.Journal, d *deck.Deck, l *lore.Lore) *Voyage {
	return &Voyage{
		phase:   PhaseSight,
		ctx:     ctx,
		sight:   s,
		board:   b,
		journal: j,
		deck:    d,
		lore:    l,
	}
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
				// TODO：等待船长批复
				v.phase = PhaseHeave
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
