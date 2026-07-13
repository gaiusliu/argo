package helm

import (
	"argo/src/knot"
	"argo/src/voyage"
	"context"
	"log/slog"
)

type PhaseRunnerAsking struct {
}

func (pr *PhaseRunnerAsking) Run(ctx context.Context,
	v *voyage.Voyage, emit func(knot.Event)) bool {
	slog.Info("phase asking", "session id", v.ID())
	// 断点前将本轮状态快照落盘
	v.Phase = voyage.PhaseExecTools
	checkpoint(v)
	err := v.SaveState()
	if err != nil {
		slog.Error("save state failed", "error", err)
	}

	return true
}
