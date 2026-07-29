package voyage

// Phase 五态常量，取值见 PhaseSight / PhaseRead / PhaseCall / PhaseHeave / PhaseDock。
const (
	// PhaseSight 观星：sight.Take() 获取 Omen
	PhaseSight = iota + 1
	// PhaseRead 评审：board.Read() 解读 Omen
	PhaseRead
	// PhaseCall 裁决：等待船长批复
	PhaseCall
	// PhaseHeave 执行：deck.Haul() 拉绳
	PhaseHeave
	// PhaseDock 靠港：journal.Append() 归档
	PhaseDock
)
