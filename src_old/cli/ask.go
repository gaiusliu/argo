package main

import (
	"fmt"
	"os"
	"strings"

	"argo/src_old/knot"

	diffmatchpatch "github.com/sergi/go-diff/diffmatchpatch"
)

// ANSI 复位码，formatDiff 使用
const reset = "\033[0m"

// formatDiff 与 showDiff 逻辑相同，但返回字符串供 viewport 展示。
func formatDiff(tu knot.ToolUse) string {
	path, _ := tu.Parameters[knot.ParamPath].(string)
	if path == "" {
		return ""
	}

	var oldText string
	if data, err := os.ReadFile(path); err == nil {
		oldText = string(data)
	} else if !os.IsNotExist(err) {
		return fmt.Sprintf("\n⚠️  无法读取原文件 %s: %v\n", path, err)
	}

	var newText string
	switch tu.Name {
	case "write":
		newText, _ = tu.Parameters[knot.ParamContent].(string)
	case "edit":
		oldStr, _ := tu.Parameters[knot.ParamOldStr].(string)
		newStr, _ := tu.Parameters[knot.ParamNewStr].(string)
		if replaceAll, _ := tu.Parameters[knot.ParamReplaceAll].(bool); replaceAll {
			newText = strings.ReplaceAll(oldText, oldStr, newStr)
		} else {
			newText = strings.Replace(oldText, oldStr, newStr, 1)
		}
	}

	dmp := diffmatchpatch.New()
	a, b, lines := dmp.DiffLinesToChars(oldText, newText)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, lines)

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("\n── 📄 %s ──", path))
	for _, d := range diffs {
		for _, line := range strings.Split(strings.TrimSuffix(d.Text, "\n"), "\n") {
			switch d.Type {
			case diffmatchpatch.DiffInsert:
				buf.WriteString(fmt.Sprintf("\n%s+ %s%s",
					diffAddStyle.Render(""), line, reset))
			case diffmatchpatch.DiffDelete:
				buf.WriteString(fmt.Sprintf("\n%s- %s%s",
					diffDelStyle.Render(""), line, reset))
			case diffmatchpatch.DiffEqual:
				buf.WriteString(fmt.Sprintf("\n  %s", line))
			}
		}
	}
	buf.WriteString(fmt.Sprintf("\n──%s──\n", strings.Repeat("─", 40)))
	return buf.String()
}

// formatAskPrompt 生成 Ask 权限确认的提示文本（含 diff 预览）。
func formatAskPrompt(tu knot.ToolUse) string {
	var buf strings.Builder
	if tu.Name == "write" || tu.Name == "edit" {
		buf.WriteString(formatDiff(tu))
	}
	buf.WriteString(fmt.Sprintf("\n%s  %s\n", askPromptStyle.Render("⚠️"), tu.VerdictReason))
	buf.WriteString(fmt.Sprintf("   Tool: %s, Param: %s\n", tu.Name, knot.GetRawParam(tu)))
	buf.WriteString("   Execute? [y/N/stop]")
	return buf.String()
}
