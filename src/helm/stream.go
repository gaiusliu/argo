package helm

import "argo/src/knot"

// newEventChannel 创建带缓冲的事件 channel
func newEventChannel() chan knot.Event {
	return make(chan knot.Event, 16)
}
