package helm

import (
	"argo/src/knot"
	"argo/src/voyage"
	"context"
	"log/slog"
)

type PhaseRunnerAsking struct {
}

func (pr *PhaseRunnerAsking) Run(ctx context.Context, st *HelmState, emit func(knot.Event), session voyage.Voyage) {
	slog.Info("phase asking", "session id", session.ID(), "helm state", st)
	// 断点前将本轮状态快照落盘
	checkpoint(session, st)
	st.Phase = HelmPhaseExecTools
	st.Save(session.ID())
}
