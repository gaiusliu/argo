package helm

import (
	"argo/src/knot"
	"argo/src/voyage"
	"context"
	"log/slog"
)

// PhaseRunner 状态机各阶段的执行器接口。
type PhaseRunner interface {
	Run(ctx context.Context, v *voyage.Voyage, emit func(knot.Event)) bool
}

// newRunner 按 Phase 返回对应的 runner 实现。
func newRunner(phase int) PhaseRunner {
	switch phase {
	case voyage.PhaseModel:
		return NewPhaseRunnerModel()
	case voyage.PhaseExecTools:
		return &PhaseRunnerExecTools{}
	case voyage.PhaseAsking:
		return &PhaseRunnerAsking{}
	case voyage.PhaseDone:
		return &PhaseRunnerDone{}
	default:
		slog.Error("unknown helm phase, falling back to done", "phase", phase)
		return &PhaseRunnerDone{}
	}
}
