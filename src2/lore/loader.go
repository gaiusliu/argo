package lore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// loadNote 解析单个 SKILL.md，提取 name、description 和路径。
// name 缺失时用父目录名作为 fallback。
func loadNote(path string) (Note, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Note{}, err
	}
	name, desc, _, err := parseFrontmatter(string(data))
	if err != nil {
		return Note{}, err
	}
	if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}
	return Note{Name: name, Description: desc, Path: path}, nil
}

// parseFrontmatter 解析 YAML frontmatter，返回 name、description 和正文。
func parseFrontmatter(content string) (name, desc, body string, err error) {
	if !strings.HasPrefix(content, "---\n") {
		return "", "", "", errors.New("no frontmatter")
	}
	remaining := content[4:]
	idx := strings.Index(remaining, "\n---\n")
	if idx < 0 {
		idx = strings.Index(remaining, "\n---")
	}
	if idx < 0 {
		return "", "", "", errors.New("malformed frontmatter: no closing ---")
	}
	fmText, body := remaining[:idx], remaining[idx+4:]
	for _, line := range strings.Split(fmText, "\n") {
		line = strings.TrimSpace(line)
		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			continue
		}
		key, val := strings.TrimSpace(kv[0]), strings.Trim(strings.TrimSpace(kv[1]), `"'`)
		switch key {
		case "name":
			name = val
		case "description":
			desc = val
		}
	}
	return name, desc, body, nil
}
