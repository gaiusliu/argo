package crew

import (
	"os"
	"path/filepath"
	"strings"
)

// scanDir 递归扫描 root 目录，返回所有 SKILL.md 文件的绝对路径。
// 跳过以 "." 开头的隐藏目录和 node_modules。
func scanDir(root string) ([]string, error) {
	var paths []string

	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			// 跳过无法访问的目录，不中断扫描
			return nil
		}

		if d.IsDir() && p != root {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" {
				return filepath.SkipDir
			}
		}

		// 收集 SKILL.md 文件的绝对路径
		if !d.IsDir() && d.Name() == "SKILL.md" {
			abs, err := filepath.Abs(p)
			if err != nil {
				return nil
			}
			paths = append(paths, abs)
		}

		return nil
	})

	return paths, err
}
