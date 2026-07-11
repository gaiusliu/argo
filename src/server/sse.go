// Package server 提供 Argo 的 HTTP 后端入口。
package server

import (
	"encoding/json"
	"fmt"
	"io"

	"argo/src/knot"
)

// writeSSE 将 Event 转为 JSON 并以 SSE 格式写入 w。
func writeSSE(w io.Writer, ev knot.Event) error {
	data, err := json.Marshal(ToEventJSON(ev))
	if err != nil {
		return fmt.Errorf("sse marshal: %w", err)
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return fmt.Errorf("sse write: %w", err)
	}
	return nil
}
