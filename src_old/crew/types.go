// Package crew 实现技能（Skill）管理：发现、读取、按需供给。
// 一门技能就是一个目录下的 SKILL.md 文件。
package crew

// SkillInfo 技能元数据（L1 披露级别）。
type SkillInfo struct {
	Name        string // 技能名（来自 frontmatter 或目录名 fallback）
	Description string // 技能描述（来自 frontmatter，允许为空）
	Path        string // SKILL.md 文件绝对路径
}

// Skills 技能注册表接口。
type Skills interface {
	// List 返回所有已发现技能的元数据列表，按名称排序。
	List() []SkillInfo

	// Instructions 返回指定技能的完整指令正文（去除 frontmatter）。
	// 技能不存在时返回 error。
	Instructions(name string) (string, error)
}
