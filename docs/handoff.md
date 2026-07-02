# Handoff — deck 模块开发

## 背景

正在开发 Argo（一个极简 AI Agent 运行基座）的 deck 模块。deck 负责工具的注册、发现和调用。

## 已完成

- [x] 核心类型定义：`Tool` 接口、`ToolResult` 结构、`ToolSource` 枚举、`builtinTool`、`cliTool`
- [x] 六个内置工具的 handler 实现：`bashHandler`、`readHandler`、`writeHandler`、`editHandler`、`grepHandler`、`globHandler`
- [x] 关键决策：grep 使用纯 Go 实现（[DEC-001](docs/decisions.md)）；MVP 不做 MCP、不做批量执行
- [x] 工具内建 `ctx` 超时/取消协作、`\r` 跨平台正则兼容

## 下一步

按 [迭代文档](docs/iterations/001-deck-mvp/main.md) 的顺序继续：

1. **启动注册流程** — `RegisterBuiltins()` 显式注册六个内置工具 + List/Lookup；读取外部工具配置文件加载 CLI 工具
2. **工具选择** — `List()` / `Lookup()` 包级函数
3. **工具调用** — `Execute` 统一接口，内置工具调 handler，CLI 工具 `exec.CommandContext` + `paramsToArgs`
4. **工具结果处理** — `postProcess` 截断（头/尾/头尾策略），`truncation` 包完成原文持久化与过期清理

## 关键参考

| 文档 | 位置 |
|---|---|
| 模块设计 | [docs/modules/deck/design.md](docs/modules/deck/design.md) |
| 迭代进度 | [docs/iterations/001-deck-mvp/main.md](docs/iterations/001-deck-mvp/main.md) |
| 技术决策 | [docs/decisions.md](docs/decisions.md) |
| 核心代码 | `src/deck/builtin_tool.go`（六个 handler + 类型定义） |

## 建议技能

- `grow-doc` — 开发过程中记录新决策或更新迭代文档
- `code-review` — 提交前审查代码变更
