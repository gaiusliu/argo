package helm

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"argo/src/knot"
)

// 行为核心：coding 场景（默认）
const codingBehaviorCore = `You are an expert software engineering assistant.
Complete tasks by reading files, executing commands, editing code, and creating files.
Be concise. If you don't know something, say so — don't make things up.`

// buildSystemPrompt 向各模块索取上下文片段，拼装完整 system prompt
func buildSystemPrompt(tools []knot.Tool) string {
	var sb strings.Builder

	// 1. 行为核心
	sb.WriteString(codingBehaviorCore)
	sb.WriteString("\n\n")

	// 2. 环境信息（session 级不变项）
	argoDir := ArgoDir()
	sb.WriteString("<env>\n")
	fmt.Fprintf(&sb, "  Platform: %s\n", runtime.GOOS)
	fmt.Fprintf(&sb, "  Today's date: %s\n", time.Now().Format("2006-01-02"))
	if argoDir != "" {
		fmt.Fprintf(&sb, "  Global config: %s\n", argoDir)
	}
	sb.WriteString("</env>\n\n")

	// 3. 工具列表
	sb.WriteString("## Available Tools\n")
	for _, t := range tools {
		fmt.Fprintf(&sb, "- %s: %s\n", t.Name(), t.Description())
	}

	// 4. AGENTS.md 用户指令（~/.argo/AGENTS.md，不存在则跳过）
	if argoDir != "" {
		if content, err := os.ReadFile(filepath.Join(argoDir, "AGENTS.md")); err == nil {
			sb.WriteString("\n")
			sb.Write(content)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
