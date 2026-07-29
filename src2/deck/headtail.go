package deck

import (
	"fmt"
)

const (
	maxOutputLen     = 30_000
	keepLen          = 16_000
	truncationMarker = "... (truncated — full output at %s) ..."
)

// HeadTailClip 超长结果首尾保留截断。
type HeadTailClip struct{}

// Clip 对超长输出做首尾截断，原文暂存临时文件。
func (HeadTailClip) Clip(result string) string {
	if len(result) <= maxOutputLen {
		return result
	}
	half := keepLen / 2
	path := storeOriginal(result)
	marker := fmt.Sprintf("\n"+truncationMarker+"\n", path)
	result = result[:half] + marker + result[len(result)-half:]
	return result
}

// storeOriginal 暂存原文到临时文件，返回路径。
// TODO：从 src/deck/ 搬运真实 temp file 实现。
func storeOriginal(text string) string {
	_ = text
	return "/tmp/argo-truncation/stub"
}
