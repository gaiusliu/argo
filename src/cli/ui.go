package main

import (
	"fmt"

	"argo/src/knot"
)

// formatToolStart 格式化工具调用开始信息，返回字符串供 viewport 展示。
func formatToolStart(ev knot.Event) string {
	return fmt.Sprintf("\n%s  Using tool: %s - %s\n",
		toolStartStyle.Render("🔧"),
		ev.ToolUse.Name, knot.GetRawParam(ev.ToolUse))
}

// formatToolDone 格式化工具调用结束信息，返回字符串供 viewport 展示。
func formatToolDone(ev knot.Event) string {
	if ev.ToolUse.Result.Status == knot.StatusError {
		return fmt.Sprintf("%s %s failed: %s\n",
			toolErrorStyle.Render("❌"),
			ev.ToolUse.Name, ev.ToolUse.Result.Output)
	}
	return fmt.Sprintf("%s %s completed\n",
		toolDoneStyle.Render("✅"),
		ev.ToolUse.Name)
}

// formatHistoryToolStart 格式化历史记录中的工具调用开始行。
func formatHistoryToolStart(tc knot.ToolCall) string {
	return fmt.Sprintf("\n%s  Using tool: %s - %s\n",
		toolStartStyle.Render("🔧"), tc.Name, knot.DisplayParam(tc.Name, tc.Arguments))
}

// formatHistoryToolDone 格式化历史记录中的工具执行结果。
// 注意：历史消息不保存 success/error 状态，统一按完成处理。
func formatHistoryToolDone(name, content string) string {
	if name == "" {
		return ""
	}
	if content != "" {
		return fmt.Sprintf("%s %s completed:\n%s\n",
			toolDoneStyle.Render("✅"), name, content)
	}
	return fmt.Sprintf("%s %s completed\n",
		toolDoneStyle.Render("✅"), name)
}
