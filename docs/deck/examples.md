# deck 配置案例

## tools.json

```json
{
  "cli": [
    { "name": "gh", "description": "GitHub CLI — 管理 issue、PR、仓库" },
    { "name": "docker", "description": "Docker 容器管理" },
    { "name": "git", "description": "Git 版本控制" }
  ],
  "mcp": [
    { "name": "github", "command": ["npx", "-y", "@anthropic/mcp-server-github"] }
  ]
}
```

---

## 内部工具 ToolDef

```go
// deck/tools/read/read.go
func init() {
    registry.Register(ToolDef{
        Name:        "Read",
        Description: "读取文件内容，支持指定行范围",
        Parameters: jsonschema.Object(
            jsonschema.String("file_path", "文件路径"),
            jsonschema.Int("offset", "起始行，不提供则从头开始"),
            jsonschema.Int("limit", "读取行数"),
        ),
        buildExecutor: func() Executor {
            return &BuiltinExecutor{Fn: readExecute}
        },
        Concurrency: "concurrent",
        Timeout:     30 * time.Second,
        // Conditions 为空 → 永远可见
    })
}
```

---

## 外部 CLI 工具 ToolDef（Argo 自动生成）

用户只写了 `{ "name": "gh", "description": "GitHub CLI" }`，Argo 的 `LoadConfig()` 自动构造：

```go
ToolDef{
    Name:        "gh",
    Description: "GitHub CLI — 管理 issue、PR、仓库",
    Parameters:  nil,  // 外部 CLI 不预写 schema，LLM 用 --help 现场获取
    buildExecutor: func() Executor {
        return &CLIExecutor{Binary: "gh"}
    },
    Conditions: []Condition{
        {Kind: "binary", Key: "gh"},  // 自动生成
    },
    Concurrency: "sequential",
    Timeout:     60 * time.Second,
}
```

---

## MCP 工具 ToolDef

用户只写了 `{ "name": "github", "command": ["npx", "-y", "@anthropic/mcp-server-github"] }`。

Argo spawn 该进程后通过 `tools/list` 获取清单，例如服务器返回了两个工具 `search_issues` 和 `list_prs`，每个被包装为：

```go
ToolDef{
    Name:        "mcp__github__search_issues",
    Description: "搜索 GitHub issue",
    Parameters:  schemaFromMCP,  // 服务器提供的 JSON Schema
    buildExecutor: func() Executor {
        return &MCPExecutor{
            ServerName: "github",
            ToolName:   "search_issues",
            Manager:    mcpManager,
        }
    },
    // Conditions 为空 → 可用性由 MCPManager.IsAlive("github") 判定
    Concurrency: "concurrent",
    Timeout:     30 * time.Second,
}
```

---

## ToolResult

```go
// 成功
ToolResult{
    Status:   "success",
    Output:   "Read 500 lines from main.go\n\npackage main\n\nimport (...",
    Metadata: map[string]any{"exit_code": 0, "duration_ms": 120},
}

// 失败
ToolResult{
    Status:   "error",
    Output:   "gh: command not found: 'list_prs'. Did you mean 'pr list'?\n\nRun 'gh --help' for usage.",
    Metadata: map[string]any{"exit_code": 1, "duration_ms": 230},
}

// 超时
ToolResult{
    Status:   "timeout",
    Output:   "command timed out after 60s",
    Metadata: map[string]any{"duration_ms": 60000},
}
```

---

## 可用性判定案例

```go
// 外部 gh 工具的 Conditions
[]Condition{{Kind: "binary", Key: "gh"}}

// exec.LookPath("gh") → 找到 → visible
// exec.LookPath("gh") → 未找到 → hidden
//   诊断信息: "binary 'gh' not in PATH"
//   提示: "Install: https://cli.github.com/manual/installation"

// 少数内部工具需要额外条件（如 Docker 工具需要 daemon）
[]Condition{
    {Kind: "binary", Key: "docker"},
    {Kind: "env",    Key: "DOCKER_HOST"},
}
```
