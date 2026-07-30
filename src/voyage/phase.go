package voyage

// Phase 六态常量。
const (
	// PhaseSight 观星：sight.Take() 获取 Omen
	PhaseSight = iota + 1
	// PhaseRead 评审：board.Read() 解读 Omen
	PhaseRead
	// PhaseSteer 掌舵：判断是否需要呼叫船长
	PhaseSteer
	// PhaseHeave 执行：deck.Haul() 拉绳
	PhaseHeave
	// PhaseDock 靠港：journal.Append() 归档
	PhaseDock
	// PhaseCall 呼叫：持久化 journal 并发送 EventTypeAsk
	PhaseCall
)
