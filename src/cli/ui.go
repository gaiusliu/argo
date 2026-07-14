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
