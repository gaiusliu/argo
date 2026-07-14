package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"argo/src/knot"

	diffmatchpatch "github.com/sergi/go-diff/diffmatchpatch"
)

// ANSI 颜色码，用于 diff 输出
const (
	red   = "\033[31m"
	green = "\033[32m"
	reset = "\033[0m"
)

// askVerdict 用户对工具执行请求的裁决结果。
type askVerdict int

const (
	verdictDeny      askVerdict = iota // 拒绝
	verdictApprove                     // 批准
	verdictInterrupt                   // 终止整个 session
)

// askCLI 通过 stdin 询问用户是否允许工具执行。
// 对 write/edit 工具额外展示彩色 diff 变更预览。
// 输入 /interrupt 返回 verdictInterrupt。
func askCLI(stdin *bufio.Reader, tu knot.ToolUse) askVerdict {
	switch tu.Name {
	case "write", "edit":
		showDiff(tu)
	}

	fmt.Fprintf(os.Stderr, "\n⚠️  %s\n", tu.VerdictReason)
	fmt.Fprintf(os.Stderr, "   工具: %s, 参数: %s\n", tu.Name, knot.GetRawParam(tu))
	fmt.Fprintf(os.Stderr, "   执行? [y/N/stop]: ")
	answer, _ := stdin.ReadString('\n')
	answer = strings.TrimSpace(answer)

	if strings.EqualFold(answer, "/interrupt") || strings.EqualFold(answer, "stop") {
		return verdictInterrupt
	}
	if strings.EqualFold(answer, "y") {
		return verdictApprove
	}
	return verdictDeny
}

// showDiff 读取目标文件当前内容，与工具参数中的新内容做 diff，彩色输出到 stderr。
func showDiff(tu knot.ToolUse) {
	path, _ := tu.Parameters[knot.ParamPath].(string)
	if path == "" {
		return
	}

	var oldText string
	if data, err := os.ReadFile(path); err == nil {
		oldText = string(data)
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "\n⚠️  无法读取原文件 %s: %v\n", path, err)
		return
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

	fmt.Fprintf(os.Stderr, "\n── 📄 %s ──", path)
	for _, d := range diffs {
		for _, line := range strings.Split(strings.TrimSuffix(d.Text, "\n"), "\n") {
			switch d.Type {
			case diffmatchpatch.DiffInsert:
				fmt.Fprintf(os.Stderr, "\n%s+ %s%s", green, line, reset)
			case diffmatchpatch.DiffDelete:
				fmt.Fprintf(os.Stderr, "\n%s- %s%s", red, line, reset)
			case diffmatchpatch.DiffEqual:
				fmt.Fprintf(os.Stderr, "\n  %s", line)
			}
		}
	}
	fmt.Fprintf(os.Stderr, "\n──%s──\n", strings.Repeat("─", 40))
}

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
