package helm

import (
	"os"
	"path/filepath"

	"argo/src/knot"
)

// ArgoDir 返回 ~/.argo/ 全局配置目录，home 取不到返回空字符串
func ArgoDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".argo")
}

// ProjectDir 返回 <root>/.argo/ 项目配置目录
func ProjectDir(root string) string {
	return filepath.Join(root, ".argo")
}

// TurnState 调用方持有的会话状态，跨 runLoop 轮次传递
type TurnState struct {
	Messages  []knot.Message
	TurnCount int
	// ModelName 当前使用的模型名，空字符串表示使用 sail 默认模型
	ModelName string
}

// SkillInfo 技能元数据（L1 级别，启动时扫描）
type SkillInfo struct {
	Name        string
	Description string
}

// MemoryEntry 长期记忆条目
type MemoryEntry struct {
	Content string
}
