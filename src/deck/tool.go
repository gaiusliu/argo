package deck

import "context"

type ToolSource int

const (
	SourceBuiltin ToolSource = iota
	SourceCLI
)

type Tool interface {
	Name() string
	Description() string
	Source() ToolSource
	Execute(ctx context.Context, params map[string]any) (ToolResult, error)
}

type ResultStatus int

const (
	StatusSuccess ResultStatus = iota
	StatusError
)

type ToolResult struct {
	Output   string
	Status   ResultStatus
	Metadata map[string]any
}

func List() []Tool
func Lookup(name string) (Tool, bool)
