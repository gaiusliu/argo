package deck

import "context"

// stub — 待 gen-code 实现
func (t *builtinTool) Name() string {
	return ""
}

// stub — 待 gen-code 实现
func (t *builtinTool) Description() string {
	return ""
}

// stub — 待 gen-code 实现
func (t *builtinTool) Source() ToolSource {
	return ""
}

// stub — 待 gen-code 实现
func (t *builtinTool) Execute(ctx context.Context, params map[string]any) (ToolResult, error) {
	return ToolResult{}, nil
}

// stub — 待 gen-code 实现
func (t *builtinTool) Concurrency() ConcurrencyMode {
	return ""
}
