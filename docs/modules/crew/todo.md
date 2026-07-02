# crew 开发任务

## 1. 核心类型定义
- [ ] ▶ 1.1 定义 SkillInfo（L1 元数据）、SkillEntry（L2 指令内容）、ScannerConfig

## 2. 文件扫描
- [ ] 2.1 实现 scanDir — 递归扫描 ~/.argo/skills/，定位所有 SKILL.md 文件路径

## 3. SKILL.md 解析
- [ ] 3.1 实现 loadSkill — 解析 YAML frontmatter + 提取 Markdown 正文
- [ ] 3.2 校验：name 缺失视为无效跳过 + description 缺失允许加载 + frontmatter 不存在跳过

## 4. Skills 接口实现
- [ ] 4.1 实现 Crew.New（初始化扫描 + 全量解析，项目级覆盖用户级同名技能）
- [ ] 4.2 实现 List — 返回所有可用技能的 L1 元数据列表
- [ ] 4.3 实现 Instructions — 按 name 返回 L2 指令正文，不存在时返回明确错误

## 5. 测试
- [ ] 5.1 scanDir 目录递归/空目录边界测试
- [ ] 5.2 loadSkill YAML 解析/frontmatter 缺失容错测试
- [ ] 5.3 List / Instructions 接口测试 + 同名覆盖优先级测试
