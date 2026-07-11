package helm

import (
	"argo/src/knot"
	"argo/src/voyage"
	"context"
)

// PhaseRunner 状态机各阶段的执行器接口。
type PhaseRunner interface {
	Run(ctx context.Context, st *HelmState, emit func(knot.Event), session voyage.Voyage)
}

// newRunner 按 Phase 返回对应的 runner 实现。
func newRunner(phase int) PhaseRunner {
	switch phase {
	case HelmPhaseModel:
		return NewPhaseRunnerModel()
	case HelmPhaseExecTools:
		return &PhaseRunnerExecTools{}
	case HelmPhaseAsking:
		return &PhaseRunnerAsking{}
	default:
		return &PhaseRunnerDone{}
	}
}
