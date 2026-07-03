# Handoff — helm 模块开发

## 背景

Argo（极简 AI Agent 运行基座）。deck 模块 MVP 已全部完成，进入 helm 模块。

## 已完成

### deck 模块（001-deck-mvp）✅

5 个源文件，~695 行 Go 代码（零外部依赖）：

- `src/deck/tool.go` — Tool 接口、List/Lookup（RWMutex + toolsIdx O(1)）、RegisterTool（冲突检查 + builtin 优先）
- `src/deck/builtin_tool.go` — 8 个 handler（bash/read/write/edit/grep/glob/list/lookup）、resolveShell
- `src/deck/tool_registry.go` — 配置文件加载、校验（`~/.argo/config.json`）
- `src/deck/cli_tool.go` — CLI 工具委托 bashHandler + postProcess
- `src/deck/truncation.go` — HeadTail 截断（30K 上限 / 16K 保留）、原文持久化

3 条技术决策：DEC-001（grep 纯 Go）、DEC-002（--help 替代 Schema()）、DEC-003（HeadTail 截断）。

## 下一步

按模块依赖顺序进入 **helm 模块**（Agent 主循环）：

```
deck ✅ → helm → reef → vault → sail → crew
```

建议先阅读 [docs/modules/helm/](docs/modules/helm/) 和 [docs/how.md](docs/how.md)，然后创建 `docs/iterations/002-helm-mvp/` 迭代文档。

## 关键参考

| 文档 | 位置 |
|---|---|
| 技术架构 | [docs/how.md](docs/how.md) |
| deck 迭代（完成） | [docs/iterations/001-deck-mvp/main.md](docs/iterations/001-deck-mvp/main.md) |
| 技术决策 | [docs/decisions.md](docs/decisions.md) |
| helm 模块设计 | [docs/modules/helm/](docs/modules/helm/) |
| deck 源码 | `src/deck/`（5 文件，695 行） |

## 建议技能

- `grow-doc` — 新模块开发时记录决策和创建迭代文档
- `code-review` — 提交前审查代码变更
