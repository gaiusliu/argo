package main

import "github.com/charmbracelet/lipgloss"

// 预定义的 lipgloss 样式，替换旧的原始 ANSI 转义序列。
var (
	// statusBarStyle 底部 token 统计状态行
	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("8")).
			Foreground(lipgloss.Color("15")).
			Padding(0, 1)

	// inputStyle 输入区域背景（替换旧 inputBg = "\033[48;5;60m"）
	inputStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("60"))

	// thinkingStyle 思考指示文本
	thinkingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			Italic(true)

	// toolStartStyle 工具调用开始
	toolStartStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3"))

	// toolDoneStyle 工具执行成功
	toolDoneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("2"))

	// toolErrorStyle 工具执行失败
	toolErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("1"))

	// diffAddStyle diff 新增行（替换旧 green）
	diffAddStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("2"))

	// diffDelStyle diff 删除行（替换旧 red）
	diffDelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("1"))

	// askPromptStyle Ask 权限确认提示
	askPromptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3")).
			Bold(true)

	// errorStyle 错误信息
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")).
			Bold(true)
)
