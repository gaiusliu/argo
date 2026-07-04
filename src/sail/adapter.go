package sail

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"argo/src/knot"
)

// openAIReq OpenAI /v1/chat/completions 请求体
type openAIReq struct {
	Model    string      `json:"model"`
	Messages []openAIMsg `json:"messages"`
	Stream   bool        `json:"stream"`
}

// openAIMsg 单条消息
type openAIMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// buildOpenAIBody 将 knot.ChatRequest 转为 OpenAI JSON 请求体
func buildOpenAIBody(req knot.ChatRequest, modelName string) ([]byte, error) {
	msgs := make([]openAIMsg, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = openAIMsg{Role: string(m.Role), Content: m.Content}
	}
	body := openAIReq{
		Model:    modelName,
		Messages: msgs,
		Stream:   true,
	}
	return json.Marshal(body)
}

// openAIChunk OpenAI Chat Completions SSE chunk 的 JSON 映射
type openAIChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ReasoningText    string `json:"reasoning_text"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

// streamState 跟踪流式响应的块状态，决定每个 chunk 应发的 EventType
type streamState struct {
	textActive     bool
	thinkingActive bool
	toolCalls      map[int]string // index → tool call ID
}

func newStreamState() *streamState {
	return &streamState{toolCalls: make(map[int]string)}
}

// handleChunk 解析 SSE chunk 并返回对应的事件列表（含 start/delta 判断）
func (s *streamState) handleChunk(raw []byte) ([]knot.Event, error) {
	var chunk openAIChunk
	if err := json.Unmarshal(raw, &chunk); err != nil {
		return nil, fmt.Errorf("sail: parse chunk: %w", err)
	}
	if len(chunk.Choices) == 0 {
		return nil, nil
	}
	d := chunk.Choices[0].Delta

	// reasoning 兼容三家命名
	reasoning := d.ReasoningContent
	if reasoning == "" {
		reasoning = d.ReasoningText
	}

	var events []knot.Event

	// --- 推理 ---
	if reasoning != "" {
		if !s.thinkingActive {
			events = append(events, knot.Event{Type: knot.EventThinkingStart})
			s.thinkingActive = true
		}
		events = append(events, knot.Event{Type: knot.EventThinkingDelta, Delta: reasoning})
	}

	// --- 文本 ---
	if d.Content != "" {
		if s.thinkingActive {
			events = append(events, knot.Event{Type: knot.EventThinkingDone})
			s.thinkingActive = false
		}
		if !s.textActive {
			events = append(events, knot.Event{Type: knot.EventTextStart})
			s.textActive = true
		}
		events = append(events, knot.Event{Type: knot.EventTextDelta, Delta: d.Content})
	}

	// --- 工具调用 ---
	for _, tc := range d.ToolCalls {
		if s.thinkingActive {
			events = append(events, knot.Event{Type: knot.EventThinkingDone})
			s.thinkingActive = false
		}
		if _, seen := s.toolCalls[tc.Index]; !seen {
			events = append(events, knot.Event{
				Type:    knot.EventToolUseStart,
				ToolUse: &knot.ToolUse{ID: tc.ID, Name: tc.Function.Name},
			})
			s.toolCalls[tc.Index] = tc.ID
		}
		events = append(events, knot.Event{
			Type: knot.EventToolUseDelta,
			ToolUse: &knot.ToolUse{
				ID:         tc.ID,
				Name:       tc.Function.Name,
				Parameters: map[string]any{"args": tc.Function.Arguments},
			},
		})
	}

	return events, nil
}

// flush 返回关闭所有活跃 block 的事件（textDone / thinkingDone / toolUseDone）
func (s *streamState) flush() []knot.Event {
	var events []knot.Event
	if s.textActive {
		events = append(events, knot.Event{Type: knot.EventTextDone})
		s.textActive = false
	}
	if s.thinkingActive {
		events = append(events, knot.Event{Type: knot.EventThinkingDone})
		s.thinkingActive = false
	}
	for _, id := range s.toolCalls {
		events = append(events, knot.Event{
			Type:    knot.EventToolUseDone,
			ToolUse: &knot.ToolUse{ID: id},
		})
	}
	return events
}

// parseSSE 逐行读取 SSE 流，解析后推入 channel。ctx 取消时退出，不泄漏 goroutine。
func parseSSE(ctx context.Context, body io.ReadCloser, out chan<- knot.Event) {
	defer close(out)
	defer body.Close()

	// ctx 取消 → 关闭 body 中断阻塞的 scanner；不泄漏 goroutine
	stopAfter := context.AfterFunc(ctx, func() {
		body.Close()
	})
	defer stopAfter()

	state := newStreamState()
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		// [DONE] → flush 所有活跃 block 后关闭 channel
		if data == "[DONE]" {
			for _, evt := range state.flush() {
				out <- evt
			}
			return
		}

		events, err := state.handleChunk([]byte(data))
		if err != nil {
			out <- knot.Event{Type: knot.EventError, Err: err}
			return
		}
		for _, evt := range events {
			out <- evt
		}
	}

	// scanner 自然结束（无 [DONE]），flush + 错误
	for _, evt := range state.flush() {
		out <- evt
	}
	if err := scanner.Err(); err != nil {
		out <- knot.Event{Type: knot.EventError, Err: fmt.Errorf("sail: sse: %w", err)}
	}

	if err := ctx.Err(); err != nil {
		out <- knot.Event{Type: knot.EventError, Err: fmt.Errorf("sail: sse: %w", err)}
	}
}

// doChat 构造请求体 → HTTP POST → goroutine 解析 SSE → 返回事件 channel
func doChat(ctx context.Context, apiKey, baseURL, modelName string, req knot.ChatRequest) (<-chan knot.Event, error) {
	// 构造 OpenAI JSON 请求体
	body, err := buildOpenAIBody(req, modelName)
	if err != nil {
		return nil, fmt.Errorf("sail: build body: %w", err)
	}

	// 构造 HTTP POST 请求，超时由 ctx 传导
	url := baseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("sail: new request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sail: http: %w", err)
	}

	// 非 200 → 返回错误事件
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		out := make(chan knot.Event, 1)
		out <- knot.Event{Type: knot.EventError, Err: fmt.Errorf("sail: http %d", resp.StatusCode)}
		close(out)
		return out, nil
	}

	// 启动 goroutine 解析 SSE 流
	out := make(chan knot.Event, 8)
	go parseSSE(ctx, resp.Body, out)
	return out, nil
}
