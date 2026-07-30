package sail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"argo/src_old/knot"
)

const openAIBaseURL = "https://api.openai.com/v1/chat/completions"

func NewProviderOpenAI(model, provider, apiKey, baseURL string,
	messages []knot.Message, tools []knot.Tool, httpClient *http.Client) *ProviderOpenAI {
	return &ProviderOpenAI{
		model:      model,
		Provider:   provider,
		APIKey:     apiKey,
		BaseURL:    baseURL,
		Messages:   messages,
		Tools:      tools,
		HTTPClient: httpClient,
	}
}

type ProviderOpenAI struct {
	model      string // 模型名（unexported，通过 Model() 访问）
	Provider   string // 供应商名（通过 Name() 访问）
	APIKey     string
	BaseURL    string
	Messages   []knot.Message
	Tools      []knot.Tool
	HTTPClient *http.Client // nil 时回退 http.DefaultClient
	Req        openAIReq
}

// Name 实现 Provider 接口。
func (s *ProviderOpenAI) Name() string { return s.Provider }

// Model 实现 Provider 接口。
func (s *ProviderOpenAI) Model() string { return s.model }

// client 返回注入的 HTTP client，nil 时回退 DefaultClient
func (s *ProviderOpenAI) client() *http.Client {
	if s.HTTPClient != nil {
		return s.HTTPClient
	}
	return http.DefaultClient
}

func (s *ProviderOpenAI) buildRequestBody() {
	openAIMsgs := make([]openAIMsg, 0, len(s.Messages))

	// 写入历史消息
	for _, m := range s.Messages {
		msg := openAIMsg{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}

		if len(m.ToolCalls) > 0 {
			toolCalls := make([]openAIToolCall, 0, len(m.ToolCalls))

			for _, tc := range m.ToolCalls {
				toolCalls = append(toolCalls, openAIToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: openAIToolCallFn{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				})
			}

			msg.ToolCalls = toolCalls

		}

		openAIMsgs = append(openAIMsgs, msg)
	}

	// 转换 knot.Tool → openAITool
	tools := make([]openAITool, 0, len(s.Tools))
	for _, t := range s.Tools {
		tools = append(tools,
			openAITool{
				Type: "function",
				Function: openAIToolFunc{
					Name:        t.Name(),
					Description: t.Description(),
					Parameters:  t.ParametersSchema(),
				},
			})
	}

	s.Req = openAIReq{
		Model:    s.model,
		Messages: openAIMsgs,
		Stream:   true,
		Tools:    tools,
		StreamOptions: openAIStreamOptions{IncludeUsage: true},
	}
}

// Chat 实现 Provider 接口。
func (s *ProviderOpenAI) Chat(ctx context.Context, messages []knot.Message, tools []knot.Tool) <-chan knot.Event {
	s.Messages = messages
	s.Tools = tools
	s.buildRequestBody()
	return s.doChat(ctx)
}

func (s *ProviderOpenAI) doChat(ctx context.Context) <-chan knot.Event {
	const maxRetries = 3
	var retryDelay time.Duration // 下次重试前的等待时间

	for attempt := 0; attempt < maxRetries; attempt++ {
		// 统一退避：429 Retry-After 优先，否则指数退避 1s/2s/4s
		if retryDelay > 0 {
			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				slog.Error("context done", "error", ctx.Err().Error(), "req", s.Req, "base url", s.BaseURL)
				out := make(chan knot.Event, 1)
				out <- knot.Event{Type: knot.EventError, Err: &APIError{Code: "network", Message: ctx.Err().Error()}}
				close(out)
				return out
			}
		}

		reqBody, err := json.Marshal(s.Req)
		if err != nil {
			slog.Error("json marshal req body failed", "error", ctx.Err().Error(), "req", s.Req, "base url", s.BaseURL)
			out := make(chan knot.Event, 1)
			out <- knot.Event{Type: knot.EventError, Err: fmt.Errorf("json marshal req body failed: %w", err)}
			close(out)
			return out
		}
		httpReq, err := http.NewRequestWithContext(ctx, "POST", s.BaseURL, bytes.NewReader(reqBody))
		if err != nil {
			slog.Error("new request with context failed", "error", err.Error(), "req", s.Req, "base url", s.BaseURL)
			out := make(chan knot.Event, 1)
			out <- knot.Event{Type: knot.EventError, Err: &APIError{Code: "server_error", Message: "new request with context failed: " + err.Error()}}
			close(out)
			return out
		}
		httpReq.Header.Set("Authorization", "Bearer "+s.APIKey)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := s.client().Do(httpReq)
		if err != nil {
			// 网络错误可重试
			if attempt < maxRetries-1 {
				retryDelay = time.Duration(1<<attempt) * time.Second
				continue
			}
			ae := &APIError{Code: "network", Retryable: true, Message: err.Error()}
			slog.Error("http client do failed", "error", err.Error(), "req", s.Req, "base url", s.BaseURL)
			out := make(chan knot.Event, 1)
			out <- knot.Event{Type: knot.EventError, Err: ae}
			close(out)
			return out
		}

		// 200 → 解析 SSE 流
		if resp.StatusCode == http.StatusOK {
			out := make(chan knot.Event, 16)
			go parseSSE(ctx, resp.Body, out)
			return out
		}

		// 非 200 → 读 body + 分类
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			slog.Error("read rsp body failed", "error", err.Error(), "req", s.Req, "base url", s.BaseURL)

			// 网络错误可重试
			if attempt < maxRetries-1 {
				retryDelay = time.Duration(1<<attempt) * time.Second
				continue
			}

			out := make(chan knot.Event, 1)
			out <- knot.Event{Type: knot.EventError, Err: err}
			close(out)
			return out
		}

		ae := classifyError(resp.StatusCode, respBody, resp.Header)
		slog.Error("do chat failed", "error", ae, "req", s.Req, "base url", s.BaseURL)

		if !ae.Retryable || attempt >= maxRetries-1 {
			out := make(chan knot.Event, 1)
			out <- knot.Event{Type: knot.EventError, Err: ae}
			close(out)
			return out
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
	return out
}

// Window 返回模型的 context window token 上限。
// 三级查找：用户配置 > models.dev 缓存 > 保守默认 128000。
func Window(modelName string) knot.TokenLimit {
	// 用户配置优先
	if mc, ok := knot.LookupModel(modelName); ok && mc.Limit != nil && mc.Limit.Context != nil {
		return knot.TokenLimit{MaxTokens: *mc.Limit.Context}
	}
	// 内置数据表（models.dev 缓存）
	if w := knot.LookupWindow(modelName); w > 0 {
		return knot.TokenLimit{MaxTokens: w}
	}
	// 保守默认值
	return knot.TokenLimit{MaxTokens: 128000}
}
