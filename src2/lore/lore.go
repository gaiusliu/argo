// Package lore 航海经验知识库——发现、读取、按需供给技能。
package lore

import (
	"fmt"
	"os"
	"sort"
)

// Note 技能索引条目，对应一个 SKILL.md 文件。
type Note struct {
	// Name 技能名
	Name string
	// Description 一句话描述
	Description string
	// Path SKILL.md 绝对路径
	Path string
}

// Lore 航海经验知识库，每次访问从磁盘重新扫描，不缓存。
type Lore struct {
	userDir    string
	projectDir string
}

// New 创建知识库。userDir 为用户级技能目录，projectDir 为项目级技能目录。
// 项目级同名技能覆盖用户级。
func New(userDir, projectDir string) *Lore {
	return &Lore{
		userDir:    userDir,
		projectDir: projectDir,
	}
}

// List 返回所有已发现技能元数据，按名称排序。
func (l *Lore) List() []Note {
	notes, err := l.scanAll()
	if err != nil {
		return nil
	}
	result := make([]Note, 0, len(notes))
	for _, n := range notes {
		result = append(result, n)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Instructions 返回指定技能的完整指令正文。
func (l *Lore) Instructions(name string) (string, error) {
	notes, err := l.scanAll()
	if err != nil {
		return "", err
	}
	note, ok := notes[name]
	if !ok {
		return "", fmt.Errorf("lore: note %q not found", name)
	}
	data, err := os.ReadFile(note.Path)
	if err != nil {
		return "", err
	}
	_, _, body, err := parseFrontmatter(string(data))
	if err != nil {
		return "", err
	}
	return body, nil
}

// scanAll 扫描用户级和项目级目录，项目级同名技能覆盖用户级。
func (l *Lore) scanAll() (map[string]Note, error) {
	notes := make(map[string]Note)
	if err := l.loadDir(notes, l.userDir); err != nil {
		return nil, err
	}
	// 后执行的 loadDir 覆盖先执行的同名 key，实现项目级覆盖用户级。
	if err := l.loadDir(notes, l.projectDir); err != nil {
		return nil, err
	}
	return notes, nil
}

// loadDir 扫描单个目录，将技能加载到 notes 中。
// 目录不存在时静默跳过，解析失败时跳过单条并继续。
func (l *Lore) loadDir(notes map[string]Note, dir string) error {
	paths, err := scanDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, p := range paths {
		note, err := loadNote(p)
		if err != nil {
			// TODO：记录日志
			continue
		}
		notes[note.Name] = note
	}
	return nil
}
