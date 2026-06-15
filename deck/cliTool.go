package deck

import "context"

// stub — 待 gen-code 实现
func (t *cliTool) Name() string {
	return ""
}

// stub — 待 gen-code 实现
func (t *cliTool) Description() string {
	return ""
}

// stub — 待 gen-code 实现
func (t *cliTool) Source() ToolSource {
	return ""
}

// stub — 待 gen-code 实现
func (t *cliTool) Execute(ctx context.Context, params map[string]any) (ToolResult, error) {
	return ToolResult{}, nil
}

// stub — 待 gen-code 实现
func (t *cliTool) Concurrency() ConcurrencyMode {
	return ""
}
