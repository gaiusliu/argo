package deck

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"argo/src/knot"
)

// 截断阈值
const (
	maxOutputLen    = 30_000 // 输出字符数上限
	keepLen         = 16_000 // HeadTail 模式保留的字符总数
	truncationMarker = "... (truncated) ..."
)

// HeadTailTruncator 对超长工具结果做首尾保留截断。
// 超过 maxOutputLen 时保留首尾各约 8K 字符，中间插入截断标记。
// 原文写入临时文件，TTL 7 天。
type HeadTailTruncator struct{}

func (HeadTailTruncator) Truncate(result knot.ToolResult) knot.ToolResult {
	if len(result.Output) <= maxOutputLen {
		return result
	}

	truncated, originalPath := headTailTruncate(result.Output)

	if result.Metadata == nil {
		result.Metadata = make(map[string]any)
	}
	result.Output = truncated
	result.Metadata["truncation"] = map[string]any{
		"truncated":    true,
		"originalPath": originalPath,
	}
	return result
}

// headTailTruncate 按 HeadTail 策略截断文本，尝试持久化原文。
// store 失败不影响截断——LLM 仍拿到截断版本，仅不包含原文路径。
func headTailTruncate(text string) (string, string) {
	originalPath, err := headTailStore(text)
	if err != nil {
		// store 失败时仍返回截断文本，marker 不附带路径
		marker := "\n" + truncationMarker + "\n"
		half := keepLen / 2
		return text[:half] + marker + text[len(text)-half:], ""
	}

	marker := fmt.Sprintf("\n... (truncated — full output at %s) ...\n", originalPath)
	half := keepLen / 2
	return text[:half] + marker + text[len(text)-half:], originalPath
}

// headTailStore 将完整原文写入临时文件，返回文件路径
func headTailStore(text string) (string, error) {
	dir := filepath.Join(os.TempDir(), "argo-truncation")

	// 0700：工具输出可能含敏感内容，只允许当前用户读写
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("store failed: %w", err)
	}

	// 写入前顺手清理过期文件
	cleanupOldFiles(dir)

	// 文件名格式: argo-<timestamp_ms_hex>-<random_hex>
	// 时间戳内嵌在文件名中，比依赖 mtime 更可靠的过期判断
	ts := time.Now().UnixMilli()
	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		return "", fmt.Errorf("store failed: %w", err)
	}

	// 0644：父目录已 0700 限制访问，文件无需更严
	path := filepath.Join(dir, fmt.Sprintf("argo-%x-%x", ts, randBytes))
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		return "", fmt.Errorf("store failed: %w", err)
	}

	return path, nil
}

// cleanupOldFiles 清理过期的截断原文文件（TTL：7 天）
func cleanupOldFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "argo-") {
			continue
		}
		ts, err := decodeFileTimestamp(name)
		if err != nil {
			continue
		}
		if ts.Before(cutoff) {
			os.Remove(filepath.Join(dir, name))
		}
	}
}

// decodeFileTimestamp 从文件名中解码毫秒时间戳
func decodeFileTimestamp(name string) (time.Time, error) {
	hex := strings.TrimPrefix(name, "argo-")
	idx := strings.IndexByte(hex, '-')
	if idx < 0 {
		return time.Time{}, fmt.Errorf("invalid filename")
	}
	ms, err := strconv.ParseInt(hex[:idx], 16, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(ms), nil
}

func init() {
	// 启动时清理过期截断原文文件
	cleanupOldFiles(filepath.Join(os.TempDir(), "argo-truncation"))

	RegisterTruncator("head-tail", func(opts map[string]any) (Truncator, error) {
		return HeadTailTruncator{}, nil
	})
}
