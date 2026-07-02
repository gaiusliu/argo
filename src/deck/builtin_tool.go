package deck

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

type builtinTool struct {
	name        string
	description string
	handler     func(ctx context.Context, params map[string]any) (ToolResult, error)
}

var excludedDirs = []string{".git", "node_modules", ".argo"}

func bashHandler(ctx context.Context, params map[string]any) (ToolResult, error) {
	cmdStr, ok := params["cmd"].(string)
	if !ok || cmdStr == "" {
		return ToolResult{Status: StatusError, Output: "empty command"}, fmt.Errorf("bash: empty command")
	}

	// 选择 shell
	shell := "/bin/sh"
	shellFlag := "-c"
	if runtime.GOOS == "windows" {
		// COMSPEC 指向系统 cmd.exe 的绝对路径（如 C:\Windows\System32\cmd.exe），
		// 是所有 Windows 版本必定存在的环境变量，比直接写 "cmd" 更可靠
		shell = os.Getenv("COMSPEC")
		if shell == "" {
			shell = "cmd"
		}
	}

	// 执行
	cmd := exec.CommandContext(ctx, shell, shellFlag, cmdStr)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 区分超时和非零退出码
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ToolResult{Status: StatusError, Output: "context error:" + ctxErr.Error()}, fmt.Errorf("context error: %w", ctxErr)
		}
		return ToolResult{Status: StatusError, Output: "bash command error:" + err.Error()}, fmt.Errorf("bash command error: %w", err)
	}

	return ToolResult{Status: StatusSuccess, Output: string(output)}, nil
}

func readHandler(ctx context.Context, params map[string]any) (ToolResult, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return ToolResult{Status: StatusError, Output: "path is required"}, fmt.Errorf("path is required")
	}

	offset := 1
	if v, ok := params["offset"].(int); ok {
		offset = v
	}

	limit := 2000
	if v, ok := params["limit"].(int); ok {
		limit = v
	}

	// 打开文件
	file, err := os.Open(path)
	if err != nil {
		return ToolResult{Status: StatusError, Output: "read failed: " + err.Error()}, fmt.Errorf("read failed: %w", err)
	}
	defer file.Close()

	// 流式读取，逐行编号
	var buf strings.Builder
	scanner := bufio.NewScanner(file)
	lineNum := 0
	read := 0

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return ToolResult{Status: StatusError, Output: "read failed: " + err.Error()}, fmt.Errorf("read failed: %w", err)
		}

		lineNum++
		// 跳过 offset 之前的行
		if lineNum < offset {
			continue
		}
		// 读到 limit 行就停
		if read >= limit {
			buf.WriteString("... (truncated)\n")
			break
		}
		fmt.Fprintf(&buf, "%5d\t%s\n", lineNum, scanner.Text())
		read++
	}

	if err := scanner.Err(); err != nil {
		return ToolResult{Status: StatusError, Output: "read failed: " + err.Error()}, fmt.Errorf("read failed: %w", err)
	}

	return ToolResult{Status: StatusSuccess, Output: buf.String()}, nil
}

func writeHandler(ctx context.Context, params map[string]any) (ToolResult, error) {
	// 提取参数
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return ToolResult{Status: StatusError, Output: "write failed: path is required"}, fmt.Errorf("write failed: path is required")
	}

	content, ok := params["content"].(string)
	if !ok {
		return ToolResult{Status: StatusError, Output: "write failed: content is required"}, fmt.Errorf("write failed: content is required")
	}

	// 检查 ctx
	if err := ctx.Err(); err != nil {
		return ToolResult{Status: StatusError, Output: "write failed: " + err.Error()}, fmt.Errorf("write failed: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return ToolResult{Status: StatusError, Output: "write failed: " + err.Error()}, fmt.Errorf("write failed: %w", err)
	}

	return ToolResult{Status: StatusSuccess, Output: fmt.Sprintf("wrote %d bytes to %s", len(content), path)}, nil
}

func editHandler(ctx context.Context, params map[string]any) (ToolResult, error) {
	// 提取参数
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return ToolResult{Status: StatusError, Output: "edit failed: path is required"}, fmt.Errorf("edit failed: path is required")
	}

	oldStr, ok := params["old_string"].(string)
	if !ok || oldStr == "" {
		return ToolResult{Status: StatusError, Output: "edit failed: old_string is required"}, fmt.Errorf("edit failed: old_string is required")
	}
	newStr, _ := params["new_string"].(string)

	// 拒绝空编辑
	if oldStr == newStr {
		return ToolResult{Status: StatusError, Output: "edit failed: old_string and new_string are identical"}, fmt.Errorf("edit failed: old_string and new_string are identical")
	}

	replaceAll := false
	if v, ok := params["replace_all"].(bool); ok {
		replaceAll = v
	}

	// 检查 ctx
	if err := ctx.Err(); err != nil {
		return ToolResult{Status: StatusError, Output: "edit failed: " + err.Error()}, fmt.Errorf("edit failed: %w", err)
	}

	// 读取原文件
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{Status: StatusError, Output: "edit failed: " + err.Error()}, fmt.Errorf("edit failed: %w", err)
	}

	// 定位并替换——old_string 必须精确匹配
	content := string(data)
	count := strings.Count(content, oldStr)
	if count == 0 {
		return ToolResult{Status: StatusError, Output: "edit failed: old_string not found"}, fmt.Errorf("edit failed: old_string not found")
	}

	// 有多个匹配结果且replaceAll没有设置，冲突
	if count > 1 && !replaceAll {
		return ToolResult{Status: StatusError, Output: fmt.Sprintf("edit failed: old_string matches %d times, must be unique", count)}, fmt.Errorf("edit failed: old_string matches %d times, must be unique", count)
	}

	var result string
	if replaceAll {
		result = strings.ReplaceAll(content, oldStr, newStr)
	} else {
		result = strings.Replace(content, oldStr, newStr, 1)
	}

	if err := os.WriteFile(path, []byte(result), 0644); err != nil {
		return ToolResult{Status: StatusError, Output: "edit failed: " + err.Error()}, fmt.Errorf("edit failed: %w", err)
	}

	if replaceAll {
		return ToolResult{Status: StatusSuccess, Output: fmt.Sprintf("replaced %d occurrences in %s", count, path)}, nil
	}
	return ToolResult{Status: StatusSuccess, Output: fmt.Sprintf("replaced 1 occurrence in %s", path)}, nil
}

func grepHandler(ctx context.Context, params map[string]any) (ToolResult, error) {
	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return ToolResult{Status: StatusError, Output: "grep failed: pattern is required"}, fmt.Errorf("grep failed: pattern is required")
	}

	path, ok := params["path"].(string)
	if !ok || path == "" {
		return ToolResult{Status: StatusError, Output: "grep failed: path is required"}, fmt.Errorf("grep failed: path is required")
	}

	// 编译正则
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ToolResult{Status: StatusError, Output: "grep failed: " + err.Error()}, fmt.Errorf("grep failed: %w", err)
	}

	limit := 200
	if v, ok := params["limit"].(int); ok && v > 0 {
		limit = v
	}

	// 检查 ctx
	if err := ctx.Err(); err != nil {
		return ToolResult{Status: StatusError, Output: "grep failed: " + err.Error()}, fmt.Errorf("grep failed: %w", err)
	}

	var buf strings.Builder
	found := 0

	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		// 访问出错：跳过该条目，继续遍历
		if err != nil {
			return nil
		}

		// 目录：遇到排除目录则跳过，否则进入
		if info.IsDir() {
			if isExcludedDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		// 检查 ctx
		if ctx.Err() != nil {
			return ctx.Err()
		}

		file, err := os.Open(p)
		if err != nil {
			return nil // 跳过读不了的文件
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNum := 0
		for scanner.Scan() && found < limit {
			lineNum++
			// 去掉 Windows \r\n 残留的 \r，保证跨平台正则匹配一致
			line := strings.TrimRight(scanner.Text(), "\r")
			if re.MatchString(line) {
				fmt.Fprintf(&buf, "%s:%d:%s\n", p, lineNum, line)
				found++
			}
		}
		return nil
	})

	if err := ctx.Err(); err != nil {
		return ToolResult{Status: StatusError, Output: "grep failed: " + err.Error()}, fmt.Errorf("grep failed: %w", err)
	}
	if found >= limit {
		buf.WriteString("... (truncated)\n")
	}

	return ToolResult{Status: StatusSuccess, Output: buf.String()}, nil
}

func isExcludedDir(name string) bool {
	return slices.Contains(excludedDirs, name)
}

func globHandler(ctx context.Context, params map[string]any) (ToolResult, error) {
	// 提取参数
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return ToolResult{Status: StatusError, Output: "glob failed: path is required"}, fmt.Errorf("glob failed: path is required")
	}

	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return ToolResult{Status: StatusError, Output: "glob failed: pattern is required"}, fmt.Errorf("glob failed: pattern is required")
	}

	limit := 200
	if v, ok := params["limit"].(int); ok && v > 0 {
		limit = v
	}

	// 提前校验 pattern 语法（Go 1.26）：filepath.Match 只会在 pattern 非法时
	// 报 ErrBadPattern，合法 pattern 对所有文件名都不会 err。提前校验后 Walk
	// 内即可忽略 Match 的 error。若未来 Go 版本放宽或变更此行为，需重新评估。
	if _, err := filepath.Match(pattern, ""); err != nil {
		return ToolResult{Status: StatusError, Output: "glob failed: " + err.Error()}, fmt.Errorf("glob failed: %w", err)
	}

	// 检查 ctx
	if err := ctx.Err(); err != nil {
		return ToolResult{Status: StatusError, Output: "glob failed: " + err.Error()}, fmt.Errorf("glob failed: %w", err)
	}

	var buf strings.Builder
	found := 0

	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		// 访问出错：跳过
		if err != nil {
			return nil
		}

		// 目录：遇到排除目录则跳过
		if info.IsDir() {
			if isExcludedDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		// 检查 ctx
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// 达上限：快速退出
		if found >= limit {
			return filepath.SkipAll
		}

		// pattern 已在 Walk 前校验，此处不会再 err
		match, _ := filepath.Match(pattern, info.Name())
		if !match {
			return nil
		}

		fmt.Fprintf(&buf, "%s\n", p)
		found++
		return nil
	})

	if err := ctx.Err(); err != nil {
		return ToolResult{Status: StatusError, Output: "glob failed: " + err.Error()}, fmt.Errorf("glob failed: %w", err)
	}

	if found >= limit {
		buf.WriteString("... (truncated)\n")
	}

	if found == 0 {
		return ToolResult{Status: StatusSuccess, Output: "no files found"}, nil
	}

	return ToolResult{Status: StatusSuccess, Output: buf.String()}, nil
}
