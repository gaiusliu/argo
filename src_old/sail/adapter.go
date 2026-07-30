package sail

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"argo/src_old/knot"
)

var ChatDefaultTimeout = 5 * time.Minute

// openAIStreamOptions 流式选项，启用 usage 返回
type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// openAIReq OpenAI /v1/chat/completions 请求体
type openAIReq struct {
	Model         string               `json:"model"`
	Messages      []openAIMsg          `json:"messages"`
	Stream        bool                 `json:"stream"`
	Tools         []openAITool         `json:"tools,omitempty"`
	StreamOptions openAIStreamOptions  `json:"stream_options"`
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

// openAIToolCall assistant 消息中的工具调用条目
type openAIToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function openAIToolCallFn `json:"function"`
}

// openAIToolCallFn 工具调用的函数元数据
type openAIToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// openAIMsg 单条消息
type openAIMsg struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

// openAIUsage stream_options 开启后，最后一块 chunk 携带的 token 统计
type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
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
	Usage openAIUsage `json:"usage"`
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
	lastUsage      knot.TokenUsage // 本次 Chat 调用的 token 消耗（API 实际返回）
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
	// 每块都可能带 usage（最后一块才有真实数据），直接存
	s.lastUsage = knot.TokenUsage{
		InputTokens:  chunk.Usage.PromptTokens,
		OutputTokens: chunk.Usage.CompletionTokens,
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
			s.toolCalls[tc.Index] = toolCallState{id: tc.ID, name: tc.Function.Name}
		}
		// 累积 arguments 片段，flush 时统一发出完整 Delta
		tcs := s.toolCalls[tc.Index]
		tcs.args += tc.Function.Arguments
		s.toolCalls[tc.Index] = tcs
	}

	return events, nil
}

// flush 返回关闭所有活跃 block 的事件（textDone / thinkingDone / toolUseDelta）。
// EventTextDone 附带本次 Chat 调用的 token 统计（来自 API usage 块）。
func (s *streamState) flush() []knot.Event {
	var events []knot.Event
	if s.textActive {
		events = append(events, knot.Event{
			Type:  knot.EventTextDone,
			Usage: s.lastUsage,
		})
		s.textActive = false
	}
	if s.thinkingActive {
		events = append(events, knot.Event{Type: knot.EventThinkingDone})
		s.thinkingActive = false
	}
	for _, tcs := range s.toolCalls {
		// 解析累积的 JSON arguments 为扁平 map，下游不需要感知 API 的嵌套格式
		var params map[string]any
		if err := json.Unmarshal([]byte(tcs.args), &params); err != nil {
			// 解析失败时保留原始数据供排查
			params = map[string]any{knot.ParamArgs: tcs.args}
		}
		events = append(events, knot.Event{
			Type: knot.EventToolUseDelta,
			ToolUse: knot.ToolUse{
				ID:         tcs.id,
				Name:       tcs.name,
				Parameters: params,
			},
		})
	}
	return events
}

// parseSSE 逐行读取 SSE 流，解析后推入 channel。ctx 取消时退出，不泄漏 goroutine。
func parseSSE(ctx context.Context, respBody io.ReadCloser, out chan<- knot.Event) {
	// 不能忘了关channel和body
	defer close(out)
	defer respBody.Close()

	// ctx 取消 → 关闭 body 中断阻塞的 scanner；不泄漏 goroutine
	stopAfter := context.AfterFunc(ctx, func() {
		respBody.Close()
	})
	defer stopAfter()

	state := newStreamState()
	scanner := bufio.NewScanner(respBody)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		slog.Debug("SSE chunk", "raw", data)

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
		out <- knot.Event{Type: knot.EventError, Err: fmt.Errorf("sail: sse scanner: %w", err)}
	}

	if err := ctx.Err(); err != nil {
		out <- knot.Event{Type: knot.EventError, Err: fmt.Errorf("sail: sse ctx: %w", err)}
	}
}
