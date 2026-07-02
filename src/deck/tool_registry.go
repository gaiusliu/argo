package deck

type ToolsConfig struct {
	CLI []CLIToolEntry `json:"cli"`
}

type CLIToolEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Binary      string `json:"binary"`
}

func Register(t Tool) error {
	return nil
}
