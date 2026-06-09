# deck 实现设计

## 概述

deck 提供 `Tool` 接口作为工具的统一抽象。CLI / MCP / builtin 三种来源分别实现此接口，helm 通过包级函数 `List` / `Lookup` 获取工具，调用 `Execute` 执行。工具的描述和参数信息在注册时即已确定，LLM 传错参数时从 stderr 自行修正——无需额外 Help 步骤。

## 内部结构

```
deck/
├── tool.go         // Tool 接口 + ToolResult 类型定义
├── builtin.go      // builtinTool 实现
├── cli.go          // cliTool 实现
├── mcp.go          // mcpTool 实现 + MCPManager
├── registry.go     // Register / LoadFromConfig / List / Lookup + unexported map
├── config.go       // LoadConfig / ToolsConfig
├── batch.go        // ExecuteBatch + ConcurrencyMode
├── postprocess.go  // postProcess 内部函数
├── tools/
│   └── builtin/     // 六个内置工具（bash.go, read.go, write.go, edit.go, grep.go, glob.go）
│                    // 各文件 init() 调用 deck.Register()，只需 import 此包即可触发
└── types.go        // ToolsConfig, ToolCall, ConcurrencyMode
```

## 核心类型

```go
// Tool — 每个工具实现此接口。helm 只和此接口交互
type Tool interface {
    Name() string
    Description() string
    Execute(ctx context.Context, params map[string]any) (ToolResult, error)
    Concurrency() ConcurrencyMode // concurrent | sequential
}

type ConcurrencyMode string

const (
    Concurrent ConcurrencyMode = "concurrent"
    Sequential ConcurrencyMode = "sequential"
)

type ToolResult struct {
    Output   string // 文本输出
    Status   string // success | error | timeout
    Metadata map[string]any
}

type ToolCall struct {
    Name   string
    Params map[string]any
}
```

三种工具实现：

```go
// builtinTool — 内置工具通过 Register + Handler 函数指针注册。参考 Claude Code SDK 的 SimpleTool 模式
type builtinTool struct {
    name        string
    description string
    handler     func(ctx context.Context, params map[string]any) (ToolResult, error)
    concurrency  ConcurrencyMode
}
func (t *builtinTool) Name() string        { return t.name }
func (t *builtinTool) Description() string { return t.description }
func (t *builtinTool) Execute(ctx context.Context, params map[string]any) (ToolResult, error) {
    result, err := t.handler(ctx, params)
    return postProcess(result), err
}
func (t *builtinTool) Concurrency() ConcurrencyMode { return t.concurrency }

// cliTool — 外部 CLI 命令适配为 Tool
type cliTool struct {
    name        string
    description string
    binary      string
    concurrency  ConcurrencyMode
}
func (t *cliTool) Name() string        { return t.name }
func (t *cliTool) Description() string { return t.description }
func (t *cliTool) Execute(ctx context.Context, params map[string]any) (ToolResult, error) {
    args := paramsToArgs(params)
    result := execShell(ctx, t.binary, args...)
    return postProcess(result), nil
}
func (t *cliTool) Concurrency() ConcurrencyMode { return t.concurrency }

// mcpTool — MCP 协议工具适配为 Tool。name 由 serverName + toolName 推导，避免同名冲突
type mcpTool struct {
    description  string
    serverName   string
    toolName     string
    inputSchema  json.RawMessage // tools/list 获取的 JSON Schema
    manager      *MCPManager
    concurrency   ConcurrencyMode
}
func (t *mcpTool) Name() string        { return "mcp__" + t.serverName + "__" + t.toolName }
func (t *mcpTool) Description() string { return t.description }
func (t *mcpTool) Execute(ctx context.Context, params map[string]any) (ToolResult, error) {
    result := t.manager.CallTool(ctx, t.serverName, t.toolName, params)
    return postProcess(result), nil
}
func (t *mcpTool) Concurrency() ConcurrencyMode { return t.concurrency }
```

六个内置工具通过 `init()` + `Register` + `builtinTool` 注册：

| 工具 | 描述 | 底层调用 |
|------|------|---------|
| bash | 执行 shell 命令 | `exec.Command("bash", "-c", cmd)` |
| read | 读取文件内容（带行号） | `os.ReadFile` |
| write | 创建或覆写文件 | `os.WriteFile` |
| edit | 精确字符串替换 | 读文件 → 替换 → 写文件 |
| grep | 正则搜索文件内容 | `grep -rn` |
| glob | 文件名模式匹配 | `filepath.Glob` |

示例（bash）：

```go
// tools/bash/bash.go
func init() {
    deck.Register(&builtinTool{
        name:        "bash",
        description: "执行 shell 命令",
        concurrency: Concurrent,
        handler: func(ctx context.Context, params map[string]any) (ToolResult, error) {
            cmd := params["command"].(string)
            return execShell(ctx, "bash", "-c", cmd), nil
        },
    })
}
```

## 包级 API

```go
// registry.go — 进程内唯一工具注册表，unexported map 管理
var tools = make(map[string]Tool)

// Register — init() 自注册 builtin 工具。同名冲突返回 error
func Register(t Tool) error

// LoadFromConfig — 启动时加载 tools.json 中的 CLI/MCP 工具
func LoadFromConfig(cfg ToolsConfig, mcp *MCPManager) error

// List — 返回当前所有可用工具的列表（供 helm 注入 system prompt）
func List() []Tool

// Lookup — 精确查找工具
func Lookup(name string) (Tool, bool)

// ExecuteBatch — 批量执行工具调用，按 ConcurrencyMode 分区调度
func ExecuteBatch(ctx context.Context, calls []ToolCall) []ToolResult {
    // 1. 按 Tool.Concurrency() 拆组：concurrent 组 + sequential 组
    // 2. concurrent 组 → goroutine + sync.WaitGroup
    // 3. sequential 组 → for 逐个执行
    // 4. 合并结果
}
```

## MCPManager

```go
// MCPManager — MCP 子进程生命周期管理
// Start: spawn 子进程，拉起 stdio 管道，握手；每个子进程起 goroutine 跑 cmd.Wait()
// IsAlive: 读内存状态，不做主动 ping
type MCPManager struct { ... }
func NewMCPManager(entries []MCPToolEntry) *MCPManager
func (m *MCPManager) Start(ctx context.Context) error
func (m *MCPManager) ListTools(ctx context.Context) ([]mcpTool, error)
func (m *MCPManager) CallTool(ctx context.Context, serverName, toolName string, params map[string]any) (ToolResult, error)
func (m *MCPManager) IsAlive(serverName string) bool
func (m *MCPManager) Shutdown()
```

## 配置文件

`~/.argo/tools.json`:

```json
{
  "cli": [
    { "name": "gh", "description": "GitHub CLI" }
  ],
  "mcp": [
    { "name": "github", "command": ["npx", "-y", "@anthropic/mcp-server-github"] }
  ]
}
```

对应 Go 类型：

```go
type ToolsConfig struct {
    CLI []CLIToolEntry `json:"cli"`
    MCP []MCPToolEntry `json:"mcp"`
}

type CLIToolEntry struct {
    Name        string `json:"name"`
    Description string `json:"description"`
}

type MCPToolEntry struct {
    Name    string   `json:"name"`
    Command []string `json:"command"`
}
```

## 内部辅助函数

```go
// postProcess — 结果截断（头 40% + 尾 60% + 截断标记）
// MaxOutputChars 默认 50,000
func postProcess(result ToolResult) ToolResult

// execShell — 执行 shell 命令，返回 ToolResult
func execShell(ctx context.Context, binary string, args ...string) ToolResult

// paramsToArgs — 将 map[string]any 转为 []string 命令行参数
func paramsToArgs(params map[string]any) []string
```

## 数据流

```
启动时:
  builtin init() → deck.Register() → tools[name]
  LoadConfig() → CLI 条目 → 构造 cliTool → deck.Register()
              → MCP 条目 → spawn 子进程 → tools/list → 构造 mcpTool → deck.Register()

每轮对话前:
  deck.List() → 滤除不可用工具 → []Tool → helm 取 Name() + Description() → 注入 system prompt

运行时（单工具）:
  helm → deck.Lookup("gh") → Tool
       → LLM 决定参数 → Tool.Execute(ctx, params) → ToolResult
       → 参数错误时 stderr 原样返回给 LLM，LLM 自行修正

运行时（批量）:
  helm → deck.ExecuteBatch(ctx, []ToolCall{...})
       → Lookup 获取每个 Tool → 按 Concurrency() 分区 → 并发/串行执行
```
