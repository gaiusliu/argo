package deck

// truncatedTool 装饰 Hand，Haul 执行后自动过 Clip 后处理。
type truncatedTool struct {
	Hand
	clip Clip
}

func (t *truncatedTool) Haul(params map[string]any) (Catch, error) {
	result, err := t.Hand.Haul(params)
	if err != nil {
		return result, err
	}
	return t.clip.Clip(result), nil
}
