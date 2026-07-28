package sight

// ── OpenAI Completions 协议类型，内部使用不导出 ──

// apiReq OpenAI chat completions 请求体。
type apiReq struct {
	Model    string
	Messages []apiMsg
	Hands    []apiTool
	Stream   bool
}

// apiMsg 单条消息。
type apiMsg struct {
	Role    string
	Content string
	// OmenID role=tool 时关联对应的工具调用
	OmenID string
}

// apiTool LLM function calling 工具定义。
type apiTool struct {
	Name        string
	Description string
	Parameters  map[string]any
}
