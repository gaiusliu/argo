package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"argo/src/knot"
)

// readMultiLine 处理多行输入：行尾 \ 表示续行，\\ 表示字面反斜杠。
func readMultiLine(stdin *bufio.Reader, firstLine string) string {
	var buf strings.Builder
	line := firstLine
	for {
		if strings.HasSuffix(line, "\\\\") {
			buf.WriteString(line[:len(line)-1])
			break
		}
		if strings.HasSuffix(line, "\\") {
			buf.WriteString(line[:len(line)-1])
			buf.WriteByte('\n')
			fmt.Fprintf(os.Stderr, "%s%s\r%s  → ", inputBg, clrEOL, inputBg)
			raw, err := stdin.ReadString('\n')
			if err != nil {
				break
			}
			line = strings.TrimRight(raw, " \t\r\n")
			continue
		}
		buf.WriteString(line)
		break
	}
	return buf.String()
}

// showToolStart 展示工具调用开始信息。
func showToolStart(ev knot.Event) {
	fmt.Fprintf(os.Stderr, "\n🔧 正在使用工具: %s - %s\n",
		ev.ToolUse.Name, knot.GetRawParam(ev.ToolUse))
}

// showToolDone 展示工具调用结束信息。
func showToolDone(ev knot.Event) {
	if ev.ToolUse.Result.Status == knot.StatusError {
		fmt.Fprintf(os.Stderr, "❌ %s 失败: %s\n",
			ev.ToolUse.Name, ev.ToolUse.Result.Output)
	} else {
		fmt.Fprintf(os.Stderr, "✅ %s 完成\n", ev.ToolUse.Name)
	}
}
