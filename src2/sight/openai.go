package sight

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"argo/src2/deck"
	"argo/src2/pact"
)

// openAI 实现 Sight，字段仅含长生命周期配置，每次 Take 内部构建请求。
type openAI struct {
	model   string
	key     string
	baseURL string
	hands   []deck.HandMeta
	client  *http.Client
}

func newOpenAI(model, key, baseURL string, hands []deck.HandMeta, client *http.Client) *openAI {
	return &openAI{
		model:   model,
		key:     key,
		baseURL: baseURL,
		hands:   hands,
		client:  client,
	}
}

func (o *openAI) Name() string { return o.model }

// Take 将 pact 消息+工具转为 API 请求，通过 HTTP 流式调用并推送 pact.Event。
func (o *openAI) Take(ctx context.Context, msgs []pact.Message) <-chan pact.Event {
	req := o.buildRequest(msgs)
	return o.doChat(ctx, req)
}

// buildRequest 将 pact.Message / deck.HandMeta 转为内部 API 请求结构。
func (o *openAI) buildRequest(msgs []pact.Message) apiReq {
	req := apiReq{
		Model:         o.model,
		Messages:      make([]apiMsg, 0, len(msgs)),
		Tools:         make([]apiTool, 0, len(o.hands)),
		Stream:        true,
		StreamOptions: apiStreamOpt{IncludeUsage: true},
	}
	for _, m := range msgs {
		req.Messages = append(req.Messages, apiMsg{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.OmenID,
		})
	}
	for _, t := range o.hands {
		req.Tools = append(req.Tools, apiTool{
			Type: "function",
			Function: apiToolFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return req
}

// ── 流状态管理 ──

type tcsState struct {
	id, name, args string
}

type streamState struct {
	textActive     bool
	thinkingActive bool
	toolCalls      map[int]tcsState
	lastUsage      pact.TokenUsage
}

func newStreamState() *streamState {
	return &streamState{toolCalls: make(map[int]tcsState)}
}

// streamSSE 读取 respBody 的 SSE 流，解析 chunk 并推送 Event。
func (o *openAI) streamSSE(ctx context.Context, respBody io.Reader, out chan<- pact.Event) {
	ss := newStreamState()
	scanner := bufio.NewScanner(respBody)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:]
		if data == "[DONE]" {
			ss.flush(out)
			return
		}
		events, err := ss.handleChunk([]byte(data))
		if err != nil {
			out <- pact.Event{Type: pact.EventTypeError, Err: err}
			return
		}
		for _, ev := range events {
			out <- ev
		}
	}
}

// handleChunk 解析单个 SSE chunk 为 Event 列表。
func (s *streamState) handleChunk(raw []byte) ([]pact.Event, error) {
	var chunk apiChunk
	if err := json.Unmarshal(raw, &chunk); err != nil {
		return nil, err
	}
	s.lastUsage = pact.TokenUsage{
		InputTokens:  chunk.Usage.PromptTokens,
		OutputTokens: chunk.Usage.CompletionTokens,
	}
	if len(chunk.Choices) == 0 {
		return nil, nil
	}
	d := chunk.Choices[0].Delta
	reasoning := d.ReasoningContent
	if reasoning == "" {
		reasoning = d.ReasoningText
	}
	var events []pact.Event
	if reasoning != "" {
		if !s.thinkingActive {
			events = append(events, pact.Event{Type: pact.EventTypeThinkingStart})
			s.thinkingActive = true
		}
		events = append(events, pact.Event{Type: pact.EventTypeThinkingDelta, Delta: reasoning})
	}
	if d.Content != "" {
		if s.thinkingActive {
			events = append(events, pact.Event{Type: pact.EventTypeThinkingDone})
			s.thinkingActive = false
		}
		if !s.textActive {
			events = append(events, pact.Event{Type: pact.EventTypeTextStart})
			s.textActive = true
		}
		events = append(events, pact.Event{Type: pact.EventTypeTextDelta, Delta: d.Content})
	}
	for _, tc := range d.ToolCalls {
		if s.thinkingActive {
			events = append(events, pact.Event{Type: pact.EventTypeThinkingDone})
			s.thinkingActive = false
		}
		if _, seen := s.toolCalls[tc.Index]; !seen {
			s.toolCalls[tc.Index] = tcsState{id: tc.ID, name: tc.Function.Name}
		}
		t := s.toolCalls[tc.Index]
		t.args += tc.Function.Arguments
		s.toolCalls[tc.Index] = t
	}
	return events, nil
}

// flush 关闭所有活跃 block 并发送 Done 事件。
func (s *streamState) flush(out chan<- pact.Event) {
	if s.textActive {
		out <- pact.Event{Type: pact.EventTypeTextDone, Usage: s.lastUsage}
		s.textActive = false
	}
	if s.thinkingActive {
		out <- pact.Event{Type: pact.EventTypeThinkingDone}
		s.thinkingActive = false
	}
	for _, tcs := range s.toolCalls {
		var params map[string]any
		if err := json.Unmarshal([]byte(tcs.args), &params); err != nil {
			params = map[string]any{"args": tcs.args}
		}
		out <- pact.Event{
			Type:  pact.EventTypeToolUseDone,
			Omens: []pact.Omen{{Name: tcs.name, Arguments: tcs.args}},
		}
	}
	out <- pact.Event{Type: pact.EventTypeDone, Usage: s.lastUsage}
}
func (o *openAI) doChat(ctx context.Context, req apiReq) <-chan pact.Event {
	out := make(chan pact.Event, 16)
	go func() {
		defer close(out)
		body, err := json.Marshal(req)
		if err != nil {
			out <- pact.Event{Type: pact.EventTypeError, Err: err}
			return
		}
		httpReq, err := http.NewRequestWithContext(ctx, "POST",
			o.baseURL+"/v1/chat/completions", bytes.NewReader(body))
		if err != nil {
			out <- pact.Event{Type: pact.EventTypeError, Err: err}
			return
		}
		httpReq.Header.Set("Authorization", "Bearer "+o.key)
		httpReq.Header.Set("Content-Type", "application/json")
		resp, err := o.client.Do(httpReq)
		if err != nil {
			out <- pact.Event{Type: pact.EventTypeError, Err: err}
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			out <- pact.Event{Type: pact.EventTypeError,
				Err: fmt.Errorf("HTTP %d", resp.StatusCode)}
			return
		}
		o.streamSSE(ctx, resp.Body, out)
	}()
	return out
}
