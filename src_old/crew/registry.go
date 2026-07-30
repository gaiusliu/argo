package crew

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// defaultCrew 全局技能注册表实例。
var defaultCrew *Crew

// Init 初始化全局技能注册表。
// userDir 为用户配置目录（~/.argo），workDir 为当前工作目录。
// 项目目录 .argo/skills 下的技能覆盖用户级同名技能。
func Init(userDir, workDir string) error {
	userSkillsDir := filepath.Join(userDir, "skills")
	projectSkillsDir := filepath.Join(workDir, ".argo", "skills")
	c, err := New(userSkillsDir, projectSkillsDir)
	if err != nil {
		return fmt.Errorf("crew init: %w", err)
	}
	defaultCrew = c
	return nil
}

// List 返回全局注册表中所有技能的元数据列表。
func List() []SkillInfo {
	if defaultCrew == nil {
		return nil
	}
	return defaultCrew.List()
}

// Instructions 返回全局注册表中指定技能的完整指令正文。
func Instructions(name string) (string, error) {
	if defaultCrew == nil {
		return "", fmt.Errorf("crew: 未初始化")
	}
	return defaultCrew.Instructions(name)
}

// Crew 实现 Skills 接口。
// 不缓存技能数据——每次 List/Instructions 都重新扫描磁盘。
type Crew struct {
	userDir    string // 用户技能目录
	projectDir string // 项目技能目录
}

// New 返回一个新的 Crew 实例。projectDir 为空时只扫描 userDir。
func New(userDir, projectDir string) (*Crew, error) {
	return &Crew{userDir: userDir, projectDir: projectDir}, nil
}

// scanAll 重新扫描全部源目录，返回技能 map。
// 先扫 userDir 再扫 projectDir，同名技能项目级覆盖用户级。
func (c *Crew) scanAll() (map[string]*SkillInfo, error) {
	skills := make(map[string]*SkillInfo)
	if err := c.loadDir(skills, c.userDir); err != nil {
		return nil, err
	}
	if err := c.loadDir(skills, c.projectDir); err != nil {
		return nil, err
	}
	return skills, nil
}

// loadDir 扫描指定目录，将发现的技能写入 skills map。
// 同名技能直接覆盖（实现项目级优先）。
func (c *Crew) loadDir(skills map[string]*SkillInfo, dir string) error {
	if dir == "" {
		return nil
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}

	paths, err := scanDir(dir)
	if err != nil {
		return err
	}

	for _, p := range paths {
		info, err := loadSkill(p)
		if err != nil {
			continue
		}
		skills[info.Name] = info
	}
	return nil
}

// List 每次从磁盘重新扫描，返回所有已发现技能的元数据列表（按名称排序）。
func (c *Crew) List() []SkillInfo {
	skills, err := c.scanAll()
	if err != nil {
		return nil
	}
	result := make([]SkillInfo, 0, len(skills))
	for _, info := range skills {
		result = append(result, *info)
	}
	slices.SortFunc(result, func(a, b SkillInfo) int {
		return strings.Compare(a.Name, b.Name)
	})
	return result
}

// Instructions 每次从磁盘重新扫描后，返回指定技能的完整指令正文（去除 frontmatter）。
// 技能不存在时返回 error。
func (c *Crew) Instructions(name string) (string, error) {
	skills, err := c.scanAll()
	if err != nil {
		return "", fmt.Errorf("crew: 扫描技能失败: %w", err)
	}

	info, ok := skills[name]
	if !ok {
		return "", fmt.Errorf("crew: 未知技能 %q", name)
	}

	data, err := os.ReadFile(info.Path)
	if err != nil {
		return "", fmt.Errorf("crew: 读取技能 %q: %w", name, err)
	}

	_, _, body, err := parseFrontmatter(string(data))
	if err != nil {
		return "", fmt.Errorf("crew: 解析技能 %q: %w", name, err)
	}

	return strings.TrimSpace(body), nil
}
