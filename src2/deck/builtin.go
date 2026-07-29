package deck

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// BuiltinTools 返回编译期内置工具实例。
func BuiltinTools() []Hand {
	return []Hand{
		builtin("bash", "Execute shell commands", cmdSchema(), bashHandler),
		builtin("read", "Read file contents with line numbers", pathSchema(), readHandler),
		builtin("write", "Create or overwrite a file", pathContentSchema(), writeHandler),
		builtin("grep", "Search files with regex", patternPathSchema(), grepHandler),
		builtin("web_fetch", "Fetch URL content", urlSchema(), webFetchHandler),
	}
}

type builtinTool struct {
	name    string
	desc    string
	schema  map[string]any
	handler func(ctx context.Context, params map[string]any) (string, error)
}

func builtin(name, desc string, schema map[string]any,
	handler func(context.Context, map[string]any) (string, error)) *builtinTool {
	return &builtinTool{name: name, desc: desc, schema: schema, handler: handler}
}

func (b *builtinTool) Name() string                     { return b.name }
func (b *builtinTool) Description() string              { return b.desc }
func (b *builtinTool) ParametersSchema() map[string]any { return b.schema }
func (b *builtinTool) Source() string                   { return "builtin" }

// Haul 调用 handler 执行工具。
func (b *builtinTool) Haul(params map[string]any) (string, error) {
	if b.handler == nil {
		return "stub: " + b.name + " executed", nil
	}
	return b.handler(context.Background(), params)
}

// ── JSON Schema 辅助 ──

func cmdSchema() map[string]any {
	return toolSchema([]string{"cmd"}, map[string]string{"cmd": "string"})
}
func pathSchema() map[string]any {
	return toolSchema([]string{"path"}, map[string]string{"path": "string"})
}
func pathContentSchema() map[string]any {
	return toolSchema([]string{"path", "content"},
		map[string]string{"path": "string", "content": "string"})
}
func patternPathSchema() map[string]any {
	return toolSchema([]string{"pattern", "path"},
		map[string]string{"pattern": "string", "path": "string"})
}
func urlSchema() map[string]any {
	return toolSchema([]string{"url"}, map[string]string{"url": "string"})
}

func toolSchema(required []string, props map[string]string) map[string]any {
	pp := make(map[string]any, len(props))
	for k, v := range props {
		pp[k] = map[string]string{"type": v}
	}
	s := map[string]any{"type": "object", "properties": pp}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// ── Handler 实现 ──

// BuiltinToolTimeout 工具执行超时。
const BuiltinToolTimeout = 5 * time.Minute

// toInt 将 JSON 数字（float64/int/json.Number）转为 int。
func toInt(v any, defaultVal int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return defaultVal
	}
}
func resolveShell() (string, string) {
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("bash"); err == nil {
			return "bash", "-c"
		}
		shell := os.Getenv("COMSPEC")
		if shell == "" {
			shell = "cmd"
		}
		return shell, "/c"
	}
	return "/bin/sh", "-c"
}

// bashHandler 执行 shell 命令。
func bashHandler(ctx context.Context, params map[string]any) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, BuiltinToolTimeout)
	defer cancel()
	cmdStr, ok := params["cmd"].(string)
	if !ok || cmdStr == "" {
		return "empty command", fmt.Errorf("bash: empty command")
	}
	shell, shellFlag := resolveShell()
	cmd := exec.CommandContext(ctx, shell, shellFlag, cmdStr)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr.Error(), ctxErr
		}
		return string(output), err
	}
	return string(output), nil
}

// readHandler 读取文件内容，带行号和分页。
func readHandler(ctx context.Context, params map[string]any) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, BuiltinToolTimeout)
	defer cancel()
	path, _ := params["path"].(string)
	if path == "" {
		return "", fmt.Errorf("read: missing path")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	offset := toInt(params["offset"], 1)
	limit := toInt(params["limit"], 2000)
	scanner := bufio.NewScanner(file)
	var buf strings.Builder
	lineNum := 0
	read := 0
	for scanner.Scan() {
		lineNum++
		if lineNum < offset {
			continue
		}
		if read >= limit {
			buf.WriteString("... (truncated)\n")
			break
		}
		fmt.Fprintf(&buf, "%5d\t%s\n", lineNum, scanner.Text())
		read++
	}
	return buf.String(), scanner.Err()
}

// writeHandler 创建或覆写文件。
func writeHandler(ctx context.Context, params map[string]any) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, BuiltinToolTimeout)
	defer cancel()
	path, _ := params["path"].(string)
	content, _ := params["content"].(string)
	if path == "" || content == "" {
		return "", fmt.Errorf("write: missing path or content")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), path), nil
}

func isExcludedDir(name string) bool {
	return name == ".git" || name == "node_modules" || name == ".argo"
}

// grepHandler 正则搜索文件。
func grepHandler(ctx context.Context, params map[string]any) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, BuiltinToolTimeout)
	defer cancel()
	pattern, _ := params["pattern"].(string)
	path, _ := params["path"].(string)
	if pattern == "" || path == "" {
		return "", fmt.Errorf("grep: pattern and path required")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	limit := toInt(params["limit"], 200)
	var buf strings.Builder
	found := 0
	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if isExcludedDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() && found < limit {
			lineNum++
			line := strings.TrimRight(scanner.Text(), "\r")
			if re.MatchString(line) {
				fmt.Fprintf(&buf, "%s:%d:%s\n", p, lineNum, line)
				found++
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// webFetchHandler 抓取 URL 内容。
func webFetchHandler(ctx context.Context, params map[string]any) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, BuiltinToolTimeout)
	defer cancel()
	url, _ := params["url"].(string)
	lower := strings.ToLower(url)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return "", fmt.Errorf("web_fetch: invalid url: %s", url)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	const maxSize = 5 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSize))
	if err != nil {
		return "", err
	}
	return string(body), nil
}
