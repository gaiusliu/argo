# 012：crew 平坦模式

**日期**：2026-07-14 ~ 2026-07-14
**状态**：✅ 已完成
**涉及模块**：crew（新增）、helm、deck、server

## 目标

实现 crew 技能管理模块的平坦模式。crew 只做三件事——发现、读取、按需供给。技能按渐进式披露：L1 元数据注入 system prompt，L2 全文通过 skill 工具或 /skill-name slash command 激活，L3 资源文件通过现有 read 工具按需访问。

## 范围

### 包含

- [1. crew 核心包](#1-crew-核心包) — SkillInfo 类型、Skills 接口、目录扫描、SKILL.md 解析、注册表
- [2. System Prompt 注入](#2-system-prompt-注入) — L1 技能列表注入 `<available_skills>` XML 块
- [3. skill 工具](#3-skill-工具) — LLM 可调用 `skill(name)` 获取 L2 全文
- [4. /skill-name 服务端检测](#4-skill-name-服务端检测) — server 层拦截 `/` 前缀消息，注入技能全文
- [5. 全局初始化](#5-全局初始化) — server 启动时 `crew.Init()`

### 不含

- 技能树/分类模式（`Tree()` 接口、`browse` 工具）
- L3 资源文件的主动发现（references/ 文件通过现有 read 工具访问，不做额外索引）
- 条件激活（bin/env/OS 检测）
- 远程技能拉取/安装
- 安全扫描
- 热更新

## 公共设计约束

1. **渐进式披露** — L1 永驻 system prompt（仅 name + description），L2 按需加载（tool call 或 /command）
2. **双激活路径** — skill 工具（LLM 自主） + /skill-name（用户显式），互补不互斥
3. **轻量定位** — crew 不做生命周期管理、安全扫描、条件激活。技能是纯 Markdown 内容

## 整体开发状态

| 功能 | 状态 | 关联文件 |
|------|------|---------|
| 1. crew 核心包 | ✅ 完成 | `src/crew/types.go` `scanner.go` `loader.go` `registry.go` |
| 2. System Prompt 注入 | ✅ 完成 | `src/helm/prompt.go` `src/helm/phase_model.go` |
| 3. skill 工具 | ✅ 完成 | `src/deck/builtin_tool.go` |
| 4. /skill-name 服务端检测 | ✅ 完成 | `src/server/prompt.go` |
| 5. 全局初始化 | ✅ 完成 | `src/argo-server/main.go` |

### 关联测试

| 测试 | 状态 | 关联文件 |
|------|------|---------|
| crew 单元测试 | ✅ 完成 | `src/crew/crew_test.go` (5 tests, all PASS) |

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-07-14 | 迭代闭环。生成 `docs/modules/crew/design.md`，更新 `docs/architecture.md`，清理旧 spec 文档 |
| 2026-07-14 | 全部 5 个功能实现完成。crew 核心包、system prompt 注入、skill 工具、/skill-name 检测、全局初始化 |
| 2026-07-14 | 迭代开始。基于五项目横向调研结论，确定轻量定位 + 平坦模式优先 |
