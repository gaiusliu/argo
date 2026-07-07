package deck

import (
	"context"

	"argo/src/knot"
)

type cliTool struct {
	name        string
	description string
}

func (c *cliTool) Name() string                     { return c.name }
func (c *cliTool) Description() string              { return c.description }
func (c *cliTool) Source() knot.ToolSource          { return knot.SourceCLI }
func (c *cliTool) ParametersSchema() map[string]any { return nil }

// Execute 将 binary + args 拼成 cmd 后委托给 bashHandler
func (c *cliTool) Execute(ctx context.Context, params map[string]any) (knot.ToolResult, error) {
	args, _ := params[knot.ParamArgs].(string)
	cmdStr := c.name
	if args != "" {
		cmdStr += " " + args
	}
	params[knot.ParamCmd] = cmdStr
	result, err := bashHandler(ctx, params)
	return postProcess(result), err
}
