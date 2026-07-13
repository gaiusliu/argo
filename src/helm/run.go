package helm

import (
	"argo/src/knot"
	"argo/src/voyage"
	"context"
)

func Run(ctx context.Context, v *voyage.Voyage) <-chan knot.Event {
	out := make(chan knot.Event, 16)

	go func() {
		defer close(out)
		emit := func(ev knot.Event) { out <- ev }
		for {
			// 检测中断信号（客户端断开或 interrupt 端点触发）
			select {
			case <-ctx.Done():
				return
			default:
			}

			pr := newRunner(v.Phase)
			done := pr.Run(ctx, v, emit)

			if done {
				return
			}
		}
	}()

	return out
}
