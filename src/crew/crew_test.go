package crew

import (
	"os"
	"path/filepath"
	"testing"
)

// 辅助函数：在指定目录创建 SKILL.md 文件
func writeSkillFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0700); err != nil {
		t.Fatalf("创建技能目录失败: %v", err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("写入 SKILL.md 失败: %v", err)
	}
	return path
}

// TestScanDir 验证目录扫描能正确发现所有 SKILL.md 文件，
// 并跳过 . 开头目录和 node_modules。
// 方法：创建含有效技能、隐藏目录、node_modules 的临时目录结构。
// 预期：只返回有效技能的路径，跳过隐藏目录和 node_modules。
func TestScanDir(t *testing.T) {
	root := t.TempDir()

	// 创建有效技能目录
	writeSkillFile(t, root, "skill-a", "---\nname: skill-a\ndescription: A\n---\n# A")
	writeSkillFile(t, root, "skill-b", "---\nname: skill-b\ndescription: B\n---\n# B")

	// 创建应被跳过的目录
	hiddenDir := filepath.Join(root, ".hidden-skill")
	os.MkdirAll(hiddenDir, 0700)
	os.WriteFile(filepath.Join(hiddenDir, "SKILL.md"), []byte("---\nname: hidden\n---\n# nope"), 0644)

	nodeModDir := filepath.Join(root, "node_modules", "some-pkg")
	os.MkdirAll(nodeModDir, 0700)
	os.WriteFile(filepath.Join(nodeModDir, "SKILL.md"), []byte("---\nname: pkg\n---\n# nope"), 0644)

	paths, err := scanDir(root)
	if err != nil {
		t.Fatalf("scanDir 返回错误: %v", err)
	}

	if len(paths) != 2 {
		t.Errorf("期望 2 个技能路径, 实际得到 %d 个: %v", len(paths), paths)
	}
}

// TestScanDirEmpty 验证扫描空目录不报错。
// 方法：扫描不存在的路径。
// 预期：返回空列表，无错误。
func TestScanDirEmpty(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nonexistent")
	paths, err := scanDir(root)
	if err != nil {
		t.Fatalf("scanDir 空目录不应报错: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("期望 0 个路径, 实际得到 %d", len(paths))
	}
}

// TestLoadSkill 验证 YAML frontmatter 解析的四种情况。
// 方法：分别测试正常、缺 name、缺 description、缺 frontmatter。
// 预期：
//
//	正常 → name+desc 解析成功
//	缺 name → 用父目录名 fallback
//	缺 description → 允许，desc 为空
//	缺 frontmatter → 返回 error
func TestLoadSkill(t *testing.T) {
	root := t.TempDir()

	// 正常情况
	normalPath := writeSkillFile(t, root, "normal", "---\nname: my-skill\ndescription: 我的技能\n---\n# 正文内容")
	info, err := loadSkill(normalPath)
	if err != nil {
		t.Fatalf("正常技能解析失败: %v", err)
	}
	if info.Name != "my-skill" {
		t.Errorf("期望 name=my-skill, 实际=%s", info.Name)
	}
	if info.Description != "我的技能" {
		t.Errorf("期望 description=我的技能, 实际=%s", info.Description)
	}

	// 缺 name → fallback 到目录名
	noNamePath := writeSkillFile(t, root, "fallback-name", "---\ndescription: 忘了写 name\n---\n# 正文")
	info, err = loadSkill(noNamePath)
	if err != nil {
		t.Fatalf("缺 name 的技能应仍可加载: %v", err)
	}
	if info.Name != "fallback-name" {
		t.Errorf("期望 name 用目录名 fallback=fallback-name, 实际=%s", info.Name)
	}

	// 缺 description → 允许
	noDescPath := writeSkillFile(t, root, "no-desc", "---\nname: no-desc\n---\n# 正文")
	info, err = loadSkill(noDescPath)
	if err != nil {
		t.Fatalf("缺 description 的技能应仍可加载: %v", err)
	}
	if info.Description != "" {
		t.Errorf("期望 description 为空, 实际=%s", info.Description)
	}

	// 缺 frontmatter → 报错
	noFmPath := writeSkillFile(t, root, "no-fm", "# 直接就是正文，没有 frontmatter")
	_, err = loadSkill(noFmPath)
	if err == nil {
		t.Error("缺 frontmatter 应返回 error")
	}
}

// TestNewWithOverride 验证项目级覆盖用户级同名技能。
// 方法：在 userDir 和 projectDir 创建同名技能，用 New 加载。
// 预期：projectDir 中的版本覆盖 userDir 中的版本。
func TestNewWithOverride(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()

	writeSkillFile(t, userDir, "dupe", "---\nname: dupe\ndescription: 用户版本\n---\n# 用户")
	writeSkillFile(t, projectDir, "dupe", "---\nname: dupe\ndescription: 项目版本\n---\n# 项目")

	c, err := New(userDir, projectDir)
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}

	list := c.List()
	if len(list) != 1 {
		t.Fatalf("期望 1 个技能, 实际 %d", len(list))
	}
	if list[0].Description != "项目版本" {
		t.Errorf("期望描述=项目版本, 实际=%s", list[0].Description)
	}
}

// TestListAndInstructions 验证 List 返回排序列表，Instructions 返回正文。
// 方法：创建两个技能，检查 List 排序和 Instructions 返回的 body。
// 预期：List 按名称排序，Instructions 返回去 frontmatter 的正文。
func TestListAndInstructions(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "skill-b", "---\nname: b-skill\ndescription: B\n---\n# B 的正文\nB 内容")
	writeSkillFile(t, dir, "skill-a", "---\nname: a-skill\ndescription: A\n---\n# A 的正文\nA 内容")

	c, err := New(dir, "")
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}

	// 验证排序
	list := c.List()
	if len(list) != 2 {
		t.Fatalf("期望 2 个技能, 实际 %d", len(list))
	}
	if list[0].Name != "a-skill" || list[1].Name != "b-skill" {
		t.Errorf("List 排序错误: %v", list)
	}

	// 验证 Instructions
	body, err := c.Instructions("a-skill")
	if err != nil {
		t.Fatalf("Instructions 失败: %v", err)
	}
	expected := "# A 的正文\nA 内容"
	if body != expected {
		t.Errorf("期望正文=%q, 实际=%q", expected, body)
	}

	// 验证不存在的技能
	_, err = c.Instructions("nonexistent")
	if err == nil {
		t.Error("不存在的技能应返回 error")
	}
}

// TestDiskAlwaysFresh 验证技能从磁盘实时读取，无需显式 reload。
// 方法：构造 Crew 后新建技能目录，再调用 List 和 Instructions。
// 预期：新技能无需任何刷新操作，立即被 List 和 Instructions 返回。
func TestDiskAlwaysFresh(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "skill-a", "---\nname: skill-a\ndescription: A\n---\n# 初始技能")

	c, err := New(dir, "")
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}

	// 构造后新增一个技能
	writeSkillFile(t, dir, "skill-b", "---\nname: skill-b\ndescription: B\n---\n# 后续安装的技能")

	// List 应立即包含新技能
	list := c.List()
	if len(list) != 2 {
		t.Fatalf("期望 2 个技能（新安装的 skill-b 应立即可见）, 实际 %d", len(list))
	}

	// Instructions 应能读取新技能
	body, err := c.Instructions("skill-b")
	if err != nil {
		t.Fatalf("Instructions skill-b 失败: %v", err)
	}
	if body != "# 后续安装的技能" {
		t.Errorf("期望正文='# 后续安装的技能', 实际=%q", body)
	}
}
