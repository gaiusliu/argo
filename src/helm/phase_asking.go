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
	// 不落盘对话（无 checkpoint），只存控制流状态。
	// PendingMessages 记录本轮尚未持久化的 user + assistant(tool_calls)，
	// 中断时 state.json 被删 → 自然回滚。
	v.Phase = voyage.PhaseExecTools
	v.PendingMessages = v.Tale().Unsaved()
	err := v.SaveState()
	if err != nil {
		slog.Error("save state failed", "error", err)
	}

	return true
}
