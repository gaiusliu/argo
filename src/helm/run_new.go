package helm

import (
	"argo/src/knot"
	"argo/src/voyage"
	"context"
	"log/slog"
)

func RunNew(ctx context.Context, st *HelmState, session voyage.Voyage) <-chan knot.Event {
	out := newEventChannel()

	go func() {
		defer close(out)
		emit := func(ev knot.Event) { out <- ev }
		phase := st.Phase
		for {
			if phase == HelmPhaseModel {
				pr := NewPhaseRunnerModel()
				pr.Run(ctx, st, emit, session)
			} else if phase == HelmPhaseExecTools ||
				phase == HelmPhaseAsking {
				pr := NewPhaseRunnerExecTools()
				pr.Run(ctx, st, emit, session)
			} else {
				// phase done
				slog.Info("event done", "session id", session.ID)
				return
			}

			phase = st.Phase
		}

	}()

	return out
}
