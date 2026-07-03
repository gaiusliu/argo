package deck

import "context"

type cliTool struct {
	name        string
	description string
}

func (c cliTool) Name() string        { return c.name }
func (c cliTool) Description() string { return c.description }
func (c cliTool) Source() ToolSource  { return SourceCLI }

// Execute 将 binary + args 拼成 cmd 后委托给 bashHandler
func (c cliTool) Execute(ctx context.Context, params map[string]any) (ToolResult, error) {
	args, _ := params["args"].(string)
	cmdStr := c.name
	if args != "" {
		cmdStr += " " + args
	}
	params["cmd"] = cmdStr
	result, err := bashHandler(ctx, params)
	return postProcess(result), err
}
