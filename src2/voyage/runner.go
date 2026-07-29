package voyage

import "argo/src2/pact"

// TODO：以下 phase runner 均为桩，后续填入真实逻辑

func (v *Voyage) runSight(out chan<- pact.Event) {
	// 桩：跳过 sight.Take() 流式处理
	v.phase = PhaseDock
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
	out <- pact.Event{Kind: pact.KindDone}
}
