package helm

import (
	"argo/src_old/knot"
	"argo/src_old/voyage"
	"context"
	"log/slog"
)

type PhaseRunnerDone struct {
}

func (pr *PhaseRunnerDone) Run(ctx context.Context,
	v *voyage.Voyage, emit func(knot.Event)) bool {
	slog.Info("phase done", "session id", v.ID())
	checkpoint(v)
	return true
}
