package deck

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	maxOutputLen      = 30_000
	keepLen           = 16_000
	truncationMarker  = "... (truncated) ..."
	truncationWithPath = "... (truncated — full output at %s) ..."
)

// HeadTailClip 超长结果首尾保留截断。
type HeadTailClip struct{}

// Clip 对超长输出做首尾截断，原文暂存临时文件。
// 存储失败时使用无路径截断标记，不影响结果返回。
func (HeadTailClip) Clip(result string) string {
	if len(result) <= maxOutputLen {
		return result
	}
	half := keepLen / 2
	path, err := headTailStore(result)
	if err != nil {
		marker := "\n" + truncationMarker + "\n"
		return result[:half] + marker + result[len(result)-half:]
	}
	marker := fmt.Sprintf("\n"+truncationWithPath+"\n", path)
	return result[:half] + marker + result[len(result)-half:]
}

// headTailStore 将完整原文写入临时文件，返回文件路径。
// 文件名内嵌毫秒时间戳，便于过期判断。
func headTailStore(text string) (string, error) {
	dir := filepath.Join(os.TempDir(), "argo-truncation")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("store failed: %w", err)
	}
	cleanupOldFiles(dir)
	ts := time.Now().UnixMilli()
	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		return "", fmt.Errorf("store failed: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("argo-%x-%x", ts, randBytes))
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		return "", fmt.Errorf("store failed: %w", err)
	}
	return path, nil
}

// cleanupOldFiles 清理过期的截断原文文件，TTL 7 天。
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

// decodeFileTimestamp 从文件名中解码毫秒时间戳。
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
