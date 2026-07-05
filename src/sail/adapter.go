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
	"time"

	"argo/src/knot"
)

// openAIReq OpenAI /v1/chat/completions 请求体
type openAIReq struct {
	Model    string       `json:"model"`
	Messages []openAIMsg  `json:"messages"`
	Stream   bool         `json:"stream"`
	Tools    []openAITool `json:"tools,omitempty"`
}

// openAITool OpenAI function calling 工具定义
type openAITool struct {
	Type     string         `json:"type"`
	Function openAIToolFunc `json:"function"`
}

// openAIToolFunc 工具函数元数据
type openAIToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
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

// toolCallState 记录单个 tool call 流式推送中的累积状态
type toolCallState struct {
	id   string
	name string
	args string
}

// streamState 跟踪流式响应的块状态，决定每个 chunk 应发的 EventType
type streamState struct {
	textActive     bool
	thinkingActive bool
	toolCalls      map[int]toolCallState
}

func newStreamState() *streamState {
	return &streamState{
		toolCalls: make(map[int]toolCallState),
	}
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
			s.toolCalls[tc.Index] = toolCallState{id: tc.ID, name: tc.Function.Name}
		}
		// 累积 arguments 片段，flush 时统一发出完整 Delta
		tcs := s.toolCalls[tc.Index]
		tcs.args += tc.Function.Arguments
		s.toolCalls[tc.Index] = tcs
	}

	return events, nil
}

// flush 返回关闭所有活跃 block 的事件（textDone / thinkingDone / toolUseDelta）
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
	for _, tcs := range s.toolCalls {
		events = append(events, knot.Event{
			Type: knot.EventToolUseDelta,
			ToolUse: &knot.ToolUse{
				ID:         tcs.id,
				Name:       tcs.name,
				Parameters: map[string]any{knot.ParamArgs: tcs.args},
			},
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

// doChat 构造请求体 → HTTP POST（含重试） → goroutine 解析 SSE → 返回事件 channel
func doChat(ctx context.Context, apiKey, baseURL, modelName string, req knot.ChatRequest) (<-chan knot.Event, error) {
	body, err := buildOpenAIBody(req, modelName)
	if err != nil {
		return nil, fmt.Errorf("sail: build body: %w", err)
	}

	url := baseURL + "/v1/chat/completions"
	const maxRetries = 3
	var retryDelay time.Duration // 下次重试前的等待时间

	for attempt := 0; attempt < maxRetries; attempt++ {
		// 统一退避：429 Retry-After 优先，否则指数退避 1s/2s/4s
		if retryDelay > 0 {
			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				out := make(chan knot.Event, 1)
				out <- knot.Event{Type: knot.EventError, Err: &APIError{Code: "network", Message: ctx.Err().Error()}}
				close(out)
				return out, nil
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("sail: new request: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			// 网络错误可重试
			if attempt < maxRetries-1 {
				retryDelay = time.Duration(1<<attempt) * time.Second
				continue
			}
			ae := &APIError{Code: "network", Retryable: true, Message: err.Error()}
			out := make(chan knot.Event, 1)
			out <- knot.Event{Type: knot.EventError, Err: ae}
			close(out)
			return out, nil
		}

		// 200 → 解析 SSE 流
		if resp.StatusCode == http.StatusOK {
			out := make(chan knot.Event, 8)
			go parseSSE(ctx, resp.Body, out)
			return out, nil
		}

		// 非 200 → 读 body + 分类
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		ae := classifyError(resp.StatusCode, respBody, resp.Header)

		if !ae.Retryable || attempt >= maxRetries-1 {
			out := make(chan knot.Event, 1)
			out <- knot.Event{Type: knot.EventError, Err: ae}
			close(out)
			return out, nil
		}
		// 设置下次退避：429 用 Retry-After，否则指数退避
		if ae.RetryAfterSeconds > 0 {
			retryDelay = time.Duration(ae.RetryAfterSeconds) * time.Second
		} else {
			retryDelay = time.Duration(1<<attempt) * time.Second
		}
	}

	out := make(chan knot.Event, 1)
	out <- knot.Event{Type: knot.EventError, Err: &APIError{Code: "server_error", Retryable: false, Message: "重试次数已耗尽"}}
	close(out)
	return out, nil
}
