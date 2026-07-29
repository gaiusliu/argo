package deck

// Clip 对工具输出做后处理。失败时原样返回，不中断工具执行流。
type Clip interface {
	Clip(result string) string
}

// NewClip 按名称构造 Clip。name 为空时默认 head-tail。
func NewClip(cfg ClipCfg) Clip {
	name := cfg.Strategy
	if name == "" {
		name = "head-tail"
	}
	switch name {
	case "head-tail":
		return HeadTailClip{}
	default:
		return HeadTailClip{}
	}
}

// ClipCfg 工具输出截断配置。
type ClipCfg struct {
	// Strategy 截断策略名，默认 "head-tail"
	Strategy string `json:"strategy,omitempty"`
}
