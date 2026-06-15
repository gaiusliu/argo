package deck

// Lookup 按名精确查找工具。命中返回 (Tool, true)，未命中返回 (nil, false)。
func Lookup(name string) (Tool, bool) {
	t, ok := tools[name]
	if !ok || t == nil {
		return nil, false
	}
	return t, true
}
