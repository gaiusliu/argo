package deck

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	doublestar "github.com/bmatcuk/doublestar/v4"
	"time"

	"argo/src/knot"
)

type builtinTool struct {
	name         string
	description  string
	paramsSchema map[string]any
	handler      func(ctx context.Context, params map[string]any) (knot.ToolResult, error)
}

func (bt *builtinTool) Name() string                 { return bt.name }
func (bt *builtinTool) Description() string          { return bt.description }
func (bt *builtinTool) Source() knot.ToolSource      { return knot.SourceBuiltin }
func (bt *builtinTool) ParametersSchema() map[string]any { return bt.paramsSchema }
func (bt *builtinTool) Execute(ctx context.Context, params map[string]any) (knot.ToolResult, error) {
	result, err := bt.handler(ctx, params)
	return postProcess(result), err
}

// prop 构造单个 JSON Schema property 定义
func prop(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}

// toolSchema 构造工具参数 JSON Schema 的快捷函数
func toolSchema(required []string, props map[string]map[string]any) map[string]any {
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// mcpRPCParams 是 JSON-RPC 2.0 tools/call 请求的参数字段
type mcpRPCParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// mcpRPCRequest 是 JSON-RPC 2.0 tools/call 请求体
type mcpRPCRequest struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      int          `json:"id"`
	Method  string       `json:"method"`
	Params  mcpRPCParams `json:"params"`
}

// mcpRPCResult 解析 JSON-RPC 响应的 result.content 部分
type mcpRPCResult struct {
	Result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"result"`
}

var builtinTools = []*builtinTool{
	{
		name: "bash", description: "Executes shell commands and returns combined output",
		paramsSchema: toolSchema([]string{"cmd"}, map[string]map[string]any{
			"cmd": prop("string", "shell 命令字符串"),
		}),
		handler: bashHandler,
	},
	{
		name: "read", description: "Reads file contents with line numbers, supports offset/limit paging",
		paramsSchema: toolSchema([]string{"path"}, map[string]map[string]any{
			"path":   prop("string", "文件路径"),
			"offset": prop("integer", "起始行号，默认 1"),
			"limit":  prop("integer", "读取行数上限，默认 2000"),
		}),
		handler: readHandler,
	},
	{
		name: "write", description: "Creates or overwrites a file at the given path",
		paramsSchema: toolSchema([]string{"path", "content"}, map[string]map[string]any{
			"path":    prop("string", "文件路径"),
			"content": prop("string", "写入内容"),
		}),
		handler: writeHandler,
	},
	{
		name: "edit", description: "Makes precise string replacements in a file, supports replace_all",
		paramsSchema: toolSchema([]string{"path", "old_string", "new_string"}, map[string]map[string]any{
			"path":        prop("string", "文件路径"),
			"old_string":  prop("string", "待替换的原始文本"),
			"new_string":  prop("string", "替换后的新文本"),
			"replace_all": prop("boolean", "是否替换全部匹配，默认 false"),
		}),
		handler: editHandler,
	},
	{
		name: "grep", description: "Searches file contents with regex, returns file:line:content matches",
		paramsSchema: toolSchema([]string{"pattern", "path"}, map[string]map[string]any{
			"pattern": prop("string", "正则表达式"),
			"path":    prop("string", "搜索目录"),
			"limit":   prop("integer", "匹配上限，默认 200"),
		}),
		handler: grepHandler,
	},
	{
		name: "glob", description: "Finds files matching a glob pattern, returns matching paths",
		paramsSchema: toolSchema([]string{"pattern", "path"}, map[string]map[string]any{
			"pattern": prop("string", "glob 匹配模式"),
			"path":    prop("string", "搜索目录"),
			"limit":   prop("integer", "匹配上限，默认 200"),
		}),
		handler: globHandler,
	},
	{
		name: "list", description: "Lists all registered tools with names and descriptions",
		paramsSchema: toolSchema(nil, map[string]map[string]any{}),
		handler: listHandler,
	},
	{
		name: "lookup", description: "Gets detailed metadata for a tool by name",
		paramsSchema: toolSchema([]string{"name"}, map[string]map[string]any{
			"name": prop("string", "要查询的工具名"),
		}),
		handler: lookupHandler,
	},
	{
		name: "web_fetch", description: "Fetches URL content and returns plain text",
		paramsSchema: toolSchema([]string{"url"}, map[string]map[string]any{
			"url": prop("string", "请求 URL，仅支持 http/https"),
		}),
		handler: webFetchHandler,
	},
	{
		name: "web_search", description: "Searches the web via free MCP endpoints and returns results as markdown",
		paramsSchema: toolSchema([]string{"query"}, map[string]map[string]any{
			"query": prop("string", "搜索词"),
		}),
		handler: webSearchHandler,
	},
}

// webFetchMaxSize WebFetch 响应体上限（字节）
const webFetchMaxSize = 5 * 1024 * 1024

// webFetchTimeout WebFetch HTTP 请求超时
const webFetchTimeout = 30 * time.Second

// searchRPCTimeout WebSearch MCP 调用超时
const searchRPCTimeout = 25 * time.Second

var excludedDirs = []string{".git", "node_modules", ".argo"}

// resolveShell 返回当前平台的 shell 路径和 flag
// Windows 优先 Git Bash / WSL 的 bash，找不到则回退到 cmd
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

func bashHandler(ctx context.Context, params map[string]any) (knot.ToolResult, error) {
	cmdStr, ok := params[knot.ParamCmd].(string)
	if !ok || cmdStr == "" {
		return knot.ToolResult{Status: knot.StatusError, Output: "empty command"}, fmt.Errorf("bash: empty command")
	}

	// 选择 shell
	shell, shellFlag := resolveShell()

	// 执行
	cmd := exec.CommandContext(ctx, shell, shellFlag, cmdStr)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 区分超时和非零退出码
		if ctxErr := ctx.Err(); ctxErr != nil {
			return knot.ToolResult{Status: knot.StatusError, Output: "context error:" + ctxErr.Error()}, fmt.Errorf("context error: %w", ctxErr)
		}
		return knot.ToolResult{Status: knot.StatusError, Output: "bash command error:" + err.Error()}, fmt.Errorf("bash command error: %w", err)
	}

	return knot.ToolResult{Status: knot.StatusSuccess, Output: string(output)}, nil
}

func readHandler(ctx context.Context, params map[string]any) (knot.ToolResult, error) {
	path, ok := params[knot.ParamPath].(string)
	if !ok || path == "" {
		return knot.ToolResult{Status: knot.StatusError, Output: "path is required"}, fmt.Errorf("path is required")
	}

	offset := 1
	if v, ok := params[knot.ParamOffset].(int); ok {
		offset = v
	}

	limit := 2000
	if v, ok := params[knot.ParamLimit].(int); ok {
		limit = v
	}

	// 打开文件
	file, err := os.Open(path)
	if err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "read failed: " + err.Error()}, fmt.Errorf("read failed: %w", err)
	}
	defer file.Close()

	// 流式读取，逐行编号
	var buf strings.Builder
	scanner := bufio.NewScanner(file)
	lineNum := 0
	read := 0

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return knot.ToolResult{Status: knot.StatusError, Output: "read failed: " + err.Error()}, fmt.Errorf("read failed: %w", err)
		}

		lineNum++
		// 跳过 offset 之前的行
		if lineNum < offset {
			continue
		}
		// 读到 limit 行就停
		if read >= limit {
			buf.WriteString(truncatedMarker + "\n")
			break
		}
		fmt.Fprintf(&buf, "%5d\t%s\n", lineNum, scanner.Text())
		read++
	}

	if err := scanner.Err(); err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "read failed: " + err.Error()}, fmt.Errorf("read failed: %w", err)
	}

	return knot.ToolResult{Status: knot.StatusSuccess, Output: buf.String()}, nil
}

func writeHandler(ctx context.Context, params map[string]any) (knot.ToolResult, error) {
	// 提取参数
	path, ok := params[knot.ParamPath].(string)
	if !ok || path == "" {
		return knot.ToolResult{Status: knot.StatusError, Output: "write failed: path is required"}, fmt.Errorf("write failed: path is required")
	}

	content, ok := params[knot.ParamContent].(string)
	if !ok {
		return knot.ToolResult{Status: knot.StatusError, Output: "write failed: content is required"}, fmt.Errorf("write failed: content is required")
	}

	// 检查 ctx
	if err := ctx.Err(); err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "write failed: " + err.Error()}, fmt.Errorf("write failed: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "write failed: " + err.Error()}, fmt.Errorf("write failed: %w", err)
	}

	return knot.ToolResult{Status: knot.StatusSuccess, Output: fmt.Sprintf("wrote %d bytes to %s", len(content), path)}, nil
}

func editHandler(ctx context.Context, params map[string]any) (knot.ToolResult, error) {
	// 提取参数
	path, ok := params[knot.ParamPath].(string)
	if !ok || path == "" {
		return knot.ToolResult{Status: knot.StatusError, Output: "edit failed: path is required"}, fmt.Errorf("edit failed: path is required")
	}

	oldStr, ok := params[knot.ParamOldStr].(string)
	if !ok || oldStr == "" {
		return knot.ToolResult{Status: knot.StatusError, Output: "edit failed: old_string is required"}, fmt.Errorf("edit failed: old_string is required")
	}
	newStr, _ := params[knot.ParamNewStr].(string)

	// 拒绝空编辑
	if oldStr == newStr {
		return knot.ToolResult{Status: knot.StatusError, Output: "edit failed: old_string and new_string are identical"}, fmt.Errorf("edit failed: old_string and new_string are identical")
	}

	replaceAll := false
	if v, ok := params[knot.ParamReplaceAll].(bool); ok {
		replaceAll = v
	}

	// 检查 ctx
	if err := ctx.Err(); err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "edit failed: " + err.Error()}, fmt.Errorf("edit failed: %w", err)
	}

	// 读取原文件
	data, err := os.ReadFile(path)
	if err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "edit failed: " + err.Error()}, fmt.Errorf("edit failed: %w", err)
	}

	// 定位并替换——old_string 必须精确匹配
	content := string(data)
	count := strings.Count(content, oldStr)
	if count == 0 {
		return knot.ToolResult{Status: knot.StatusError, Output: "edit failed: old_string not found"}, fmt.Errorf("edit failed: old_string not found")
	}

	// 有多个匹配结果且replaceAll没有设置，冲突
	if count > 1 && !replaceAll {
		return knot.ToolResult{Status: knot.StatusError, Output: fmt.Sprintf("edit failed: old_string matches %d times, must be unique", count)}, fmt.Errorf("edit failed: old_string matches %d times, must be unique", count)
	}

	var result string
	if replaceAll {
		result = strings.ReplaceAll(content, oldStr, newStr)
	} else {
		result = strings.Replace(content, oldStr, newStr, 1)
	}

	if err := os.WriteFile(path, []byte(result), 0644); err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "edit failed: " + err.Error()}, fmt.Errorf("edit failed: %w", err)
	}

	if replaceAll {
		return knot.ToolResult{Status: knot.StatusSuccess, Output: fmt.Sprintf("replaced %d occurrences in %s", count, path)}, nil
	}
	return knot.ToolResult{Status: knot.StatusSuccess, Output: fmt.Sprintf("replaced 1 occurrence in %s", path)}, nil
}

func grepHandler(ctx context.Context, params map[string]any) (knot.ToolResult, error) {
	pattern, ok := params[knot.ParamPattern].(string)
	if !ok || pattern == "" {
		return knot.ToolResult{Status: knot.StatusError, Output: "grep failed: pattern is required"}, fmt.Errorf("grep failed: pattern is required")
	}

	path, ok := params[knot.ParamPath].(string)
	if !ok || path == "" {
		return knot.ToolResult{Status: knot.StatusError, Output: "grep failed: path is required"}, fmt.Errorf("grep failed: path is required")
	}

	// 编译正则
	re, err := regexp.Compile(pattern)
	if err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "grep failed: " + err.Error()}, fmt.Errorf("grep failed: %w", err)
	}

	limit := 200
	if v, ok := params[knot.ParamLimit].(int); ok && v > 0 {
		limit = v
	}

	// 检查 ctx
	if err := ctx.Err(); err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "grep failed: " + err.Error()}, fmt.Errorf("grep failed: %w", err)
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
		return knot.ToolResult{Status: knot.StatusError, Output: "grep failed: " + err.Error()}, fmt.Errorf("grep failed: %w", err)
	}
	if found >= limit {
		buf.WriteString(truncatedMarker + "\n")
	}

	return knot.ToolResult{Status: knot.StatusSuccess, Output: buf.String()}, nil
}

func globHandler(ctx context.Context, params map[string]any) (knot.ToolResult, error) {
	// 提取参数
	path, ok := params[knot.ParamPath].(string)
	if !ok || path == "" {
		return knot.ToolResult{Status: knot.StatusError, Output: "glob failed: path is required"}, fmt.Errorf("glob failed: path is required")
	}

	pattern, ok := params[knot.ParamPattern].(string)
	if !ok || pattern == "" {
		return knot.ToolResult{Status: knot.StatusError, Output: "glob failed: pattern is required"}, fmt.Errorf("glob failed: pattern is required")
	}

	limit := 200
	if v, ok := params[knot.ParamLimit].(int); ok && v > 0 {
		limit = v
	}

	// 提前校验 doublestar pattern（支持 ** 递归匹配）
	if _, err := doublestar.Match(pattern, "x"); err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "glob failed: " + err.Error()}, fmt.Errorf("glob failed: %w", err)
	}

	// 检查 ctx
	if err := ctx.Err(); err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "glob failed: " + err.Error()}, fmt.Errorf("glob failed: %w", err)
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

		// 用相对路径 + doublestar 匹配（支持 ** 递归匹配）
		rel, err := filepath.Rel(path, p)
		if err != nil {
			slog.Error("glob: rel 失败", "path", path, "p", p, "error", err)
			return nil
		}
		rel = filepath.ToSlash(rel)
		match, err := doublestar.Match(pattern, rel)
		if err != nil {
			slog.Error("glob: match 失败", "pattern", pattern, "rel", rel, "error", err)
			return nil
		}
		if !match {
			return nil
		}

		fmt.Fprintf(&buf, "%s\n", p)
		found++
		return nil
	})

	if err := ctx.Err(); err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "glob failed: " + err.Error()}, fmt.Errorf("glob failed: %w", err)
	}

	if found >= limit {
		buf.WriteString(truncatedMarker + "\n")
	}

	if found == 0 {
		return knot.ToolResult{Status: knot.StatusSuccess, Output: "no files found"}, nil
	}

	return knot.ToolResult{Status: knot.StatusSuccess, Output: buf.String()}, nil
}

func isExcludedDir(name string) bool {
	return slices.Contains(excludedDirs, name)
}

// listHandler 返回注册表中全部工具的名称和描述
func listHandler(ctx context.Context, params map[string]any) (knot.ToolResult, error) {
	tools := List()
	var buf strings.Builder
	for _, t := range tools {
		fmt.Fprintf(&buf, "%s: %s\n", t.Name(), t.Description())
	}
	return knot.ToolResult{Status: knot.StatusSuccess, Output: buf.String()}, nil
}

// lookupHandler 按名称查找工具，返回详细元数据
func lookupHandler(ctx context.Context, params map[string]any) (knot.ToolResult, error) {
	name, ok := params[knot.ParamName].(string)
	if !ok || name == "" {
		return knot.ToolResult{Status: knot.StatusError, Output: "lookup: name is required"}, fmt.Errorf("lookup: name is required")
	}
	t, ok := Lookup(name)
	if !ok {
		return knot.ToolResult{Status: knot.StatusError, Output: "tool not found: " + name}, fmt.Errorf("lookup: tool not found: %s", name)
	}
	out := fmt.Sprintf("name: %s\ndescription: %s\n", t.Name(), t.Description())
	return knot.ToolResult{Status: knot.StatusSuccess, Output: out}, nil
}

// stripHTML 将 HTML 转换为 Markdown，保留段落、标题、列表等语义结构
func stripHTML(s string) string {
	conv := md.NewConverter("", true, nil)
	// 移除不可见内容块
	conv.Remove("script")
	conv.Remove("style")
	conv.Remove("noscript")
	conv.Remove("iframe")
	conv.Remove("object")
	conv.Remove("embed")
	markdown, err := conv.ConvertString(s)
	if err != nil {
		return s
	}
	return strings.TrimSpace(markdown)
}

// webFetchHandler 获取 URL 内容并返回纯文本
// 只允许 http/https 协议，响应上限 5MB，超时 30s
func webFetchHandler(ctx context.Context, params map[string]any) (knot.ToolResult, error) {
	// 提取并校验 url 参数
	url, ok := params[knot.ParamURL].(string)
	if !ok || url == "" {
		return knot.ToolResult{Status: knot.StatusError, Output: "web_fetch: url is required"},
			fmt.Errorf("web_fetch: url is required")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return knot.ToolResult{Status: knot.StatusError, Output: "web_fetch: only http/https allowed"},
			fmt.Errorf("web_fetch: unsupported scheme")
	}

	// 检查上下文
	if err := ctx.Err(); err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "web_fetch: " + err.Error()},
			fmt.Errorf("web_fetch: %w", err)
	}

	// 发起 GET 请求，30s 超时
	client := &http.Client{Timeout: webFetchTimeout}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "web_fetch: " + err.Error()},
			fmt.Errorf("web_fetch: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "web_fetch: " + err.Error()},
			fmt.Errorf("web_fetch: %w", err)
	}
	defer resp.Body.Close()

	// 先看 content-length 头，超限直接拒绝，避免下载浪费
	if cl := resp.ContentLength; cl > webFetchMaxSize {
		return knot.ToolResult{Status: knot.StatusError, Output: "web_fetch: response too large"},
			fmt.Errorf("web_fetch: content-length %d exceeds limit", cl)
	}

	// 限制读取实际大小
	body, err := io.ReadAll(io.LimitReader(resp.Body, webFetchMaxSize))
	if err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "web_fetch: " + err.Error()},
			fmt.Errorf("web_fetch: %w", err)
	}

	// MIME 分发：按 content-type 决定处理方式
	contentType := resp.Header.Get("Content-Type")
	mime, _, _ := strings.Cut(contentType, ";")
	mime = strings.TrimSpace(mime)

	switch {
	case strings.HasPrefix(mime, "image/"):
		// 图片转 base64 Data URL，供视觉模型消费
		b64 := base64.StdEncoding.EncodeToString(body)
		return knot.ToolResult{Status: knot.StatusSuccess, Output: fmt.Sprintf("data:%s;base64,%s", mime, b64)}, nil
	case mime == "text/html":
		return knot.ToolResult{Status: knot.StatusSuccess, Output: stripHTML(string(body))}, nil
	default:
		// 纯文本、未知类型一律直接返回
		return knot.ToolResult{Status: knot.StatusSuccess, Output: string(body)}, nil
	}
}

// callSearchMCP 向 MCP 端点发送 JSON-RPC tools/call 请求并返回结果文本
func callSearchMCP(ctx context.Context, baseURL, toolName string, args map[string]any) (string, error) {
	// 构造 JSON-RPC 请求体
	reqBody := mcpRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: mcpRPCParams{
			Name:      toolName,
			Arguments: args,
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	// POST 请求，25s 超时
	client := &http.Client{Timeout: searchRPCTimeout}
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("post request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	// 按 Content-Type 提取响应体：SSE 格式（text/event-stream）需剥 data: 壳
	body := respBytes
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		// 按双换行分割事件，取第一个事件的 data 字段拼接
		events := bytes.Split(respBytes, []byte("\n\n"))
		var b strings.Builder
		for _, line := range bytes.Split(events[0], []byte("\n")) {
			if bytes.HasPrefix(line, []byte("data: ")) {
				b.Write(line[6:]) // 去掉 "data: " 前缀
			}
		}
		body = []byte(b.String())
	}

	// 解析 JSON-RPC 响应，提取 result.content[0].text
	var rpcResult mcpRPCResult
	if err := json.Unmarshal(body, &rpcResult); err != nil {
		slog.Error("web_search 响应解析失败", "endpoint", baseURL, "status", resp.StatusCode, "raw", string(respBytes))
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(rpcResult.Result.Content) == 0 {
		return "", fmt.Errorf("empty search result")
	}
	return rpcResult.Result.Content[0].Text, nil
}

// fnvHash 对字符串做 FNV-1a 哈希，返回 uint32 用于确定性分流
func fnvHash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

// webSearchHandler 通过网络搜索获取结果
// 在 Parallel 和 Exa 两个免费 MCP 端点间随机选择一个
func webSearchHandler(ctx context.Context, params map[string]any) (knot.ToolResult, error) {
	// 提取并校验 query 参数
	query, ok := params[knot.ParamQuery].(string)
	if !ok || query == "" {
		return knot.ToolResult{Status: knot.StatusError, Output: "web_search: query is required"},
			fmt.Errorf("web_search: query is required")
	}

	if err := ctx.Err(); err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "web_search: " + err.Error()},
			fmt.Errorf("web_search: %w", err)
	}

	// 对 query 取 hash 做确定性分流，均衡两个免费端点的负载
	// 0 → Parallel, 1 → Exa
	h := fnvHash(query)
	if h%2 == 0 {
		text, err := callSearchMCP(ctx, "https://search.parallel.ai/mcp", "web_search", map[string]any{
			"objective":      query,
			"search_queries": []string{query},
		})
		if err != nil {
			return knot.ToolResult{Status: knot.StatusError, Output: "web_search: " + err.Error()},
				fmt.Errorf("web_search: %w", err)
		}
		return knot.ToolResult{Status: knot.StatusSuccess, Output: text}, nil
	}

	text, err := callSearchMCP(ctx, "https://mcp.exa.ai/mcp", "web_search_exa", map[string]any{
		"query":      query,
		"type":       "auto",
		"numResults": 8,
		"livecrawl":  "fallback",
	})
	if err != nil {
		return knot.ToolResult{Status: knot.StatusError, Output: "web_search: " + err.Error()},
			fmt.Errorf("web_search: %w", err)
	}
	return knot.ToolResult{Status: knot.StatusSuccess, Output: text}, nil
}
