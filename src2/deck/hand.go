package deck

// Hand 工具抽象。Haul 返回结果前必须经过 Clip 后处理。
type Hand interface {
	Name() string
	Description() string
	ParametersSchema() map[string]any
	Source() string
	// Haul 执行工具调用，返回捕获的结果。
	Haul(params map[string]any) (Catch, error)
}

// Catch 水手拉绳执行后捕获的结果。
type Catch struct {
	Output string
}
