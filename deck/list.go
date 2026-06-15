package deck

// List 返回当前所有已注册工具的列表，排序稳定。
// 供 helm 在每轮对话前获取工具清单并注入 system prompt。
func List() []Tool {
	// 按注册顺序遍历，跳过已删除或为 nil 的条目
	result := make([]Tool, 0, len(tools))
	for _, name := range toolsOrder {
		if t, ok := tools[name]; ok && t != nil {
			result = append(result, t)
		}
	}
	return result
}
