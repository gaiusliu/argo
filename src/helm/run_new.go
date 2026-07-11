package helm

import (
	"argo/src/knot"
	"argo/src/voyage"
	"context"
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
			} else if phase == HelmPhaseExecTools {
				pr := NewPhaseRunnerExecTools()
				pr.Run(ctx, st, emit, session)
			} else if phase == HelmPhaseAsking {
				pr := NewPhaseRunnerAsking()
				pr.Run(ctx, st, emit, session)
				// 此时需返回，等待客户端审批结果
				return
			} else {
				// phase done
				pr := NewPhaseRunnerDone()
				pr.Run(ctx, st, emit, session)
				return
			}

			phase = st.Phase
		}

	}()

	return out
}
