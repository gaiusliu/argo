package crew

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// loadSkill 解析单个 SKILL.md 文件，返回 SkillInfo。
// 从 YAML frontmatter 提取 name 和 description，
// name 缺失时用父目录名 fallback，frontmatter 不存在则报错。
func loadSkill(path string) (*SkillInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load skill: %w", err)
	}

	content := string(data)

	name, desc, _, err := parseFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("load skill %s: %w", path, err)
	}

	// name 缺失则用父目录名
	if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}

	// description 允许为空
	if desc == "" {
		slog.Warn("skill description empty", "path", path)
	}

	return &SkillInfo{
		Name:        name,
		Description: desc,
		Path:        path,
	}, nil
}

// parseFrontmatter 从 SKILL.md 内容中提取 YAML frontmatter。
// 返回 name、description、body（frontmatter 之后的部分）。
func parseFrontmatter(content string) (name, desc, body string, err error) {
	if !strings.HasPrefix(content, "---\n") {
		return "", "", "", fmt.Errorf("no frontmatter")
	}

	// 找到结束的 "---"
	remaining := content[4:]
	idx := strings.Index(remaining, "\n---\n")
	if idx < 0 {
		// 也尝试 "---\n" (末尾无空行)
		if idx = strings.Index(remaining, "\n---"); idx < 0 {
			return "", "", "", fmt.Errorf("malformed frontmatter: no closing ---")
		}
	}

	fmText := remaining[:idx]
	body = remaining[idx+4:] // 跳过 "\n---\n" 或 "\n---"

	// 逐行解析 key: value
	for _, line := range strings.Split(fmText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		// 去除引号包围
		val = strings.Trim(val, "\"'")
		switch key {
		case "name":
			name = val
		case "description":
			desc = val
		}
	}

	return name, desc, body, nil
}
