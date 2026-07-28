package deck

// Clip 对 Catch 做后处理（截断等）。失败时原样返回，不中断工具执行流。
type Clip interface {
	Clip(result Catch) Catch
}

// NewClip 按名称构造 Clip。name 为空时默认 head-tail。
func NewClip(name string) Clip {
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
