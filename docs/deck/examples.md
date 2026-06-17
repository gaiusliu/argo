# deck 配置案例

## tools.json

```json
{
  "cli": [
    { "name": "gh", "description": "GitHub CLI" },
    { "name": "docker", "description": "Docker 容器管理" }
  ],
  "mcp": [
    { "name": "github", "command": ["npx", "-y", "@anthropic/mcp-server-github"] }
  ]
}
```

---

## builtin 工具注册

```go
// deck/tools/builtin/read.go
package builtin

import (
    "context"
    "os"

    "argo/deck"
)

func init() {
    deck.Register(deck.NewBuiltinTool(
        "read",
        "读取文件内容，支持指定行范围",
        func(ctx context.Context, params map[string]any) (deck.ToolResult, error) {
            filePath, _ := params["filePath"].(string)
            data, err := os.ReadFile(filePath)
            if err != nil {
                return deck.ToolResult{Status: "error", Output: err.Error()}, nil
            }
            return deck.ToolResult{Status: "success", Output: string(data)}, nil
        },
        deck.Concurrent,
        deck.TruncationConfig{Strategy: deck.TruncationHead, Ratio: 20},
    ))
}
```

---

## CLI 工具（LoadConfig 自动构造）

用户只写了 `{ "name": "gh", "description": "GitHub CLI" }`，Argo 自动构造：

```go
&cliTool{
    name:        "gh",
    description: "GitHub CLI",
    binary:      "gh",
    concurrency: Sequential,
}
```

---

## MCP 工具（MCPManager 自动构造）

用户只写了 `{ "name": "github", "command": [...] }`。Argo spawn 进程后通过 `tools/list` 获取清单，例如服务器返回了 `search_issues`，被包装为：

```go
&mcpTool{
    description: "搜索 GitHub issue",
    serverName:  "github",
    toolName:    "search_issues",
    inputSchema: schemaFromMCP,  // 服务器提供的 JSON Schema
    manager:     mcpManager,
    concurrency: Concurrent,
}
// Name() → "mcp__github__search_issues"
```

---

## ToolResult

```go
// 成功
ToolResult{
    Status:   "success",
    Output:   "package main\n\nimport \"fmt\"\n...",
    Metadata: map[string]any{"exit_code": 0, "duration_ms": 120},
}

// 失败
ToolResult{
    Status: "error",
    Output: "gh: command not found",
}

// 超时
ToolResult{
    Status: "timeout",
    Output: "command timed out after 60s",
}
```

---

## helm 调用示例

```go
// 注入 system prompt
tools := deck.List()

// 执行工具
tool, ok := deck.Lookup("gh")
result, _ := tool.Execute(ctx, params)

// 批量执行
results := deck.ExecuteBatch(ctx, []deck.ToolCall{
    {Name: "read", Params: map[string]any{"filePath": "main.go"}},
    {Name: "grep", Params: map[string]any{"pattern": "context", "path": "."}},
})
```
