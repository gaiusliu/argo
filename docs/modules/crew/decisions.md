# crew 技术决策记录

---

## DEC-C01：Skill 格式沿用 agentskills.io 标准

**日期**：2026-06-09
**状态**：已决定

**决策**：SKILL.md + YAML frontmatter，MVP 只要求两个字段：`name` + `description`。五家调研项目全部遵循此标准，不再重新发明。

**拒绝**：Hermes Agent 的丰富 YAML（platforms gating、config vars、fallback_for_toolsets）——MVP 不需要条件激活和配置变量注入。

---

## DEC-C02：渐进式三级披露

**日期**：2026-06-09
**状态**：已决定

**决策**：crew 按三级加载技能内容。

| 级 | 加载时机 | 内容 | 触发方式 |
|------|------|------|------|
| L1 元数据 | 启动时扫描 | 仅 name + description | Skills.List() |
| L2 指令 | LLM 决定激活时 | 完整 SKILL.md 正文 | Skills.Instructions(name) |
| L3 资源 | LLM 需要更多细节时 | skill 目录下 references/ 等文件 | Read 工具按需 |

**来源**：Hermes Agent 的渐进式披露 + OpenClaw 的 read 工具按需。

**拒绝**：Hermes Agent 的模板变量（`${HERMES_SKILL_DIR}`）+ 内联 shell（`` !`command` ``）——MVP 不做技能预处理。

---

## DEC-C03：不做代码扩展层

**日期**：2026-06-09
**状态**：已决定

**决策**：crew 不做 Plugin/Extension 代码扩展。Skill 是纯 Markdown 内容，crew 只做发现、加载和按需供给。代码级扩展（注册新工具、订阅事件钩子）由 deck 的 `tools/builtin` 包注册机制覆盖——第三方通过 Go 包 `init()` + `deck.Register()` 注册新工具，不需要插件运行时。

**拒绝**：pi 的 Extension API——需要模块加载器 + 沙箱 + 生命周期管理，MVP 不引入插件运行时。

---

## DEC-C04：按 Agent 配置加载，不做条件激活

**日期**：2026-06-09
**状态**：已决定

**决策**：Agent 配置中声明 `skills: [learn, select]`，crew 只加载列表中的技能。不做 paths 匹配文件自动激活（Claude-Code 模式）——通用 Agent 没有"文件操作触发技能"的场景。

**拒绝**：OpenClaw 的 `requires: { bins: [...] }` 声明式前置条件——MVP 不检查环境依赖。
