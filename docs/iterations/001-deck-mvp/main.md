# 001-deck-mvp

## 目标

搭建 deck 工具管理模块的 MVP 版本。

## 范围

- [x] 核心结构/接口定义（Tool、ToolResult、ToolSource、builtinTool、cliTool）
- [x] 内置工具 handler（bash、read、write、edit、grep、glob、list、lookup、web_fetch、web_search，共 10 个）
- [x] 启动注册流程（RegisterTools → RegisterTool + toolsIdx 冲突检查 + builtin 优先）
- [x] 外部 CLI 工具配置（ConfigPath + LoadConfig + ValidateConfig，map 结构）
- [x] 工具选择（List / Lookup + RWMutex + toolsIdx O(1) 查找）
- [x] 工具调用（cliTool 实现 Tool 接口，委托 bashHandler；resolveShell 支持 Git Bash > cmd）
- [x] 工具结果处理（postProcess + HeadTail 截断，30K 上限 + 16K 保留，原文持持久化到临时文件）

## 关键决策

- [DEC-001](../../decisions.md#dec-001grep-工具-mvp-阶段使用纯-go-实现)：grep 纯 Go 实现
- [DEC-002](../../decisions.md#dec-002cli-工具参数发现不通过-schema-方法由-llm-自行---help)：CLI 工具参数由 LLM 通过 `--help` 自行发现
- [DEC-003](../../decisions.md#dec-003工具输出截断使用-headtail-策略后续支持可配置)：HeadTail 截断策略，后续支持可配置
- [DEC-012~021](../../decisions.md)：WebFetch/WebSearch 配套决策（重定向、UA、输出格式、SSE、权限、参数、MCP 端点、分流策略、MIME 分发）

## 文件

| 文件 | 内容 |
|------|------|
| `tool.go` | Tool 接口、类型定义、List/Lookup（RWMutex + toolsIdx）、RegisterTool（冲突检查）、RegisterTools |
| `builtin_tool.go` | 10 个 handler、resolveShell、builtinTools 清单 |
| `tool_registry.go` | ToolsConfig、CLIToolEntry、ConfigPath、LoadConfig、validateCLIEntry、ValidateConfig |
| `cli_tool.go` | CLI 工具，委托 bashHandler + postProcess |
| `truncation.go` | truncatedMarker 常量、truncate（HeadTail）、store、postProcess |

## 状态

✅ 完成
