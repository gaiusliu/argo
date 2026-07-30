package lore

import (
	"os"
	"path/filepath"
)

// scanDir 递归扫描 root 目录，返回所有 SKILL.md 绝对路径。
// 跳过以 . 开头的隐藏目录和 node_modules。
func scanDir(root string) ([]string, error) {
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name != "." && (name[0] == '.' || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Name() == "SKILL.md" {
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			paths = append(paths, abs)
		}
		return nil
	})
	return paths, err
}
