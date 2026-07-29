package sight

// ── OpenAI Completions 协议类型，内部使用不导出 ──

// apiReq OpenAI chat completions 请求体。
type apiReq struct {
	Model         string       `json:"model"`
	Messages      []apiMsg     `json:"messages"`
	Tools         []apiTool    `json:"tools,omitempty"`
	Stream        bool         `json:"stream"`
	StreamOptions apiStreamOpt `json:"stream_options"`
}

type apiStreamOpt struct {
	IncludeUsage bool `json:"include_usage"`
}

// apiMsg 单条消息。
type apiMsg struct {
	Role       string        `json:"role"`
	Content    string        `json:"content"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	ToolCalls  []apiToolCall `json:"tool_calls,omitempty"`
}

// apiToolCall assistant 消息中的工具调用。
type apiToolCall struct {
	ID       string        `json:"id"`
	Type     string        `json:"type"`
	Function apiToolCallFn `json:"function"`
}

type apiToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// apiTool LLM function calling 工具定义。
type apiTool struct {
	Type     string      `json:"type"`
	Function apiToolFunc `json:"function"`
}

type apiToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ── SSE 解析 ──

// apiChunk OpenAI SSE chunk。
type apiChunk struct {
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
	Usage apiUsage `json:"usage"`
}

// apiUsage SSE chunk 的 token 统计。
type apiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}
