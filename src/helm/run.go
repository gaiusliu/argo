package helm

import (
	"argo/src/knot"
	"argo/src/voyage"
	"context"
)

func Run(ctx context.Context, st *HelmState, session voyage.Voyage) <-chan knot.Event {
	out := newEventChannel()

	go func() {
		defer close(out)
		emit := func(ev knot.Event) { out <- ev }
		for {
			pr := newRunner(st.Phase)
			pr.Run(ctx, st, emit, session)

			// Asking 或 Done 是断点，goroutine 退出等待外部驱动
			if st.Phase == HelmPhaseAsking || st.Phase == HelmPhaseDone {
				return
			}
		}
	}()

	return out
}
