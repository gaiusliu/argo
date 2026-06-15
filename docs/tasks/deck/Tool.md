---
module: deck
interface: Tool
layer: 0
status: code_done
implemented_by:
  - docs/tasks/deck/builtinTool.md
  - docs/tasks/deck/cliTool.md
  - docs/tasks/deck/mcpTool.md
---

# deck::Tool

## 规格

```go
type Tool interface {
    Name() string
    Description() string
    Source() ToolSource
    Execute(ctx context.Context, params map[string]any) (ToolResult, error)
    Concurrency() ConcurrencyMode
}
```

## 方法约定

- `Name()`: 返回工具唯一标识名。builtin 编译时指定，CLI 由配置文件指定，MCP 由 `"mcp__" + serverName + "__" + toolName` 推导。
- `Description()`: 返回自然语言描述，供 LLM 理解工具用途。
- `Source()`: 返回工具来源（`ToolSourceBuiltin` / `ToolSourceCLI` / `ToolSourceMCP`），供 Register 处理同名冲突优先级。
- `Execute(ctx, params)`: 执行工具调用。参数错误不预先校验，直接执行，错误通过 ToolResult.Status="error" 返回给 LLM。内部统一调用 postProcess 做结果截断。
- `Concurrency()`: 返回 Concurrent 或 Sequential，供 ExecuteBatch 分区调度。读操作为 Concurrent，写操作为 Sequential。
