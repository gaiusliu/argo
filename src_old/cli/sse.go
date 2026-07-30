package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"argo/src_old/knot"
	"argo/src_old/server"
)

// streamEvents 发送 POST 请求，将 SSE 响应流转为 knot.Event 通道。
// cancel 关闭底层连接，可用于主动终止流。
func (c *argoClient) streamEvents(path string, body any) (<-chan knot.Event, func(), error) {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		r = strings.NewReader(string(data))
	}
	resp, err := c.client.Post("http://"+c.addr+path, "application/json", r)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, nil, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	out := make(chan knot.Event)
	go func() {
		defer resp.Body.Close()
		defer close(out)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var ej server.EventJSON
			if err := json.Unmarshal([]byte(line[6:]), &ej); err != nil {
				continue
			}
			out <- toKnotEvent(ej)
		}
	}()
	return out, func() { resp.Body.Close() }, nil
}

// sendPrompt 发送用户消息并返回 SSE 事件流和取消函数。
func (c *argoClient) sendPrompt(sessionID, message string) (<-chan knot.Event, func(), error) {
	return c.streamEvents("/session/prompt", server.NewPromptRequest{
		SessionID: sessionID, Message: message,
	})
}

// sendAskReply 发送用户对 EventAsk 的回复并返回 SSE 事件流和取消函数。
func (c *argoClient) sendAskReply(sessionID string, tus []knot.ToolUse) (<-chan knot.Event, func(), error) {
	return c.streamEvents("/session/prompt", server.NewPromptRequest{
		SessionID:       sessionID,
		ToolUseVerdicts: tus,
	})
}
