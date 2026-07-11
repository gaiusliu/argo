package helm

import (
	"argo/src/knot"
	"argo/src/voyage"
	"context"
	"log/slog"
)

type PhaseRunnerDone struct {
}

func NewPhaseRunnerDone() *PhaseRunnerDone {
	return &PhaseRunnerDone{}
}

func (pr *PhaseRunnerDone) Run(ctx context.Context, st *HelmState, emit func(knot.Event), session voyage.Voyage) {
	slog.Info("phase done", "session id", session.ID(), "helm state", st)
	writeRecordsNew(session, st)
}
