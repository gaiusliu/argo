# Argo 项目规则

## 仓库结构

- **`argo`**（`gaiusliu/argo`）：主代码仓库，`src/` + `src2/` 等
- **`argo-docs`**（`gaiusliu/argo-docs`）：`docs/` 目录为独立 Git 仓库，迭代文档、设计文档、handoff 等均在此仓库中
- 两个仓库各自独立提交和推送，互不影响

## 代码生成

- 一次只生成/修改一个文件，每步不超过 30 行
- 每步执行完后暂停，等待用户审核通过再继续下一步
- 审核不通过则修改本步代码，不得跳过进入下一步

## 架构规则

- 本项目必须遵守 `docs/rules.md` 中记录的所有规则，不可违反。具体规则内容在使用时按需加载。

## 参考项目

| 项目 | 本地路径 | 语言 | 说明 |
|------|----------|------|------|
| Reasonix | `code/DeepSeek-Reasonix/` | Go | AI coding agent，核心逻辑在 `internal/`、`desktop/`、`cmd/reasonix/` |
| pi | `code/pi/` | TypeScript | AI coding agent，核心逻辑在 `packages/coding-agent/src/core/` |
| opencode | `code/opencode/` | TypeScript | SST 开源的 AI coding agent（Effect monorepo），核心逻辑在 `packages/core/src/` |
| oh-my-pi | `code/oh-my-pi/` | Rust + TypeScript | AI coding agent，核心逻辑在 `packages/coding-agent/src/`、`crates/` |

可通过 `projectPath` 参数跨项目查询 CodeGraph。
