package helm

import (
	"argo/src/knot"
	"argo/src/voyage"
	"context"
	"log/slog"
)

type PhaseRunnerAsking struct {
}

func NewPhaseRunnerAsking() *PhaseRunnerAsking {
	return &PhaseRunnerAsking{}
}

func (pr *PhaseRunnerAsking) Run(ctx context.Context, st *HelmState, emit func(knot.Event), session voyage.Voyage) {
	slog.Info("phase asking", "session id", session.ID(), "helm state", st)
	// Asking完下一个阶段时exec tools
	st.Phase = HelmPhaseExecTools
	st.Save(session.ID())
}
