package deck

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
)

// 截断阈值
const (
	maxOutputLen    = 30_000 // 输出字符数上限
	keepLen         = 16_000 // HeadTail 模式保留的字符总数
	truncatedMarker = "... (truncated) ..."
)

// truncate 按 HeadTail 策略截断文本，尝试持久化原文
// store 失败不影响截断——LLM 仍拿到截断版本，仅不包含原文路径
func truncate(text string) (string, string) {
	key, err := store(text)
	if err != nil {
		// store 失败时仍返回截断文本，marker 不附带路径
		marker := "\n" + truncatedMarker + "\n"
		half := keepLen / 2
		return text[:half] + marker + text[len(text)-half:], ""
	}

	marker := fmt.Sprintf("\n... (truncated — full output at %s) ...\n", key)
	half := keepLen / 2
	return text[:half] + marker + text[len(text)-half:], key
}

// store 将完整原文写入临时文件，返回唯一键（文件路径）
func store(text string) (string, error) {
	dir := filepath.Join(os.TempDir(), "argo-truncation")

	// 0700：工具输出可能含敏感内容（密钥、令牌、日志），
	// 只允许当前用户读写。后续迭代再评估是否需要放开。
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("store failed: %w", err)
	}

	// 随机文件名，防止冲突
	name := make([]byte, 16)
	if _, err := rand.Read(name); err != nil {
		return "", fmt.Errorf("store failed: %w", err)
	}

	// 0644：父目录已 0700 限制了外部访问，此处的文件无需更严。
	// owner 可读写，便于用户直接 cat/less 查看完整输出。
	path := filepath.Join(dir, fmt.Sprintf("%x", name))
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		return "", fmt.Errorf("store failed: %w", err)
	}

	return path, nil
}

// postProcess 对工具输出做截断后处理，未超上限则原样返回
func postProcess(result ToolResult) ToolResult {
	if len(result.Output) <= maxOutputLen {
		return result
	}

	truncated, key := truncate(result.Output)

	if result.Metadata == nil {
		result.Metadata = make(map[string]any)
	}
	result.Metadata["truncated"] = true
	result.Output = truncated
	if key != "" {
		result.Metadata["truncation_key"] = key
	}

	return result
}
