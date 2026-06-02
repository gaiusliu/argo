# deck 实现设计

## 概述

deck 是工具层，提供 `ToolRegistry`（注册、可用性判定、发现）和 `ToolExecutor`（使用、结果后处理）。对上暴露为 helm 可注入的服务接口，对下通过 `ToolAdapter` 屏蔽内部工具/外部 CLI/MCP 三种来源的差异。

## 内部结构

```
deck/
├── registry.go       // ToolRegistry：注册、可用性判定、发现
├── executor.go       // ToolExecutor：使用、结果后处理
├── adapter.go        // ToolAdapter 接口：
│                     //   builtinAdapter  (Go 函数调用)
│                     //   cliAdapter      (exec.Command)
│                     //   mcpAdapter      (MCP tools/call)
├── config.go         // LoadConfig() 读 tools.json
├── availability.go   // 可用性判定
├── postprocess.go    // 结果后处理
├── tools/            // 内部自建工具包（每个工具一个子包）
│   ├── bash/
│   ├── read/
│   ├── write/
│   └── tools.go      // barrel 文件，import 所有工具子包触发 init()
└── types.go          // ToolDef, ToolResult, ToolExecutorRef
```

## 核心类型

```go
type ToolDef struct {
    Name        string
    Description string
    Parameters  jsonschema.Schema  // MCP/内部工具提供完整 schema；外部 CLI 为空
    Ref         ToolExecutorRef
    Conditions  []Condition        // MVP: 空=永远可见
    Concurrency ConcurrencyMode    // concurrent | sequential
    Timeout     time.Duration
}

type Condition struct {
    Kind string // "binary" | "env"
    Key  string // binary 名 / 环境变量名
}

type ToolResult struct {
    Output   string         // 文本输出
    Status   string         // success | error | timeout
    Metadata map[string]any
}

type ToolExecutorRef struct {
    Source string // "builtin" | "cli" | "mcp"
    Builtin func(ctx context.Context, params map[string]any) (ToolResult, error)
    CLI     struct{ Binary string }
    MCP     struct {
        ServerName string
        ToolName   string
    }
}
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
    CLI []CLIToolEntry  `json:"cli"`
    MCP []MCPToolEntry  `json:"mcp"`
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

## 函数签名

```go
// ToolAdapter — 屏蔽三种来源的执行差异
type ToolAdapter interface {
    Execute(ctx context.Context, ref ToolExecutorRef, params map[string]any) (ToolResult, error)
}

// ToolRegistry
func LoadRegistry(toolsCfg ToolsConfig, mcpManager *MCPManager) (*ToolRegistry, error)
func (r *ToolRegistry) EvaluateAvailability(ctx context.Context) error
func (r *ToolRegistry) List(agentCfg AgentConfig) []ToolDef
func (r *ToolRegistry) Lookup(name string) (ToolDef, bool)

// ToolExecutor
func NewToolExecutor(adapters map[string]ToolAdapter) *ToolExecutor
func (e *ToolExecutor) Execute(ctx context.Context, def ToolDef, params map[string]any) ToolResult
func (e *ToolExecutor) ExecuteBatch(ctx context.Context, calls []ToolCall) []ToolResult

// Availability — 可用性判定
func EvaluateAvailability(ctx context.Context, entries []ToolDef, mcp *MCPManager) []ToolDef

// MCPManager — MCP 子进程生命周期管理
// Start: spawn 子进程，拉起 stdio 管道，握手；每个子进程起一个 goroutine 跑 cmd.Wait()
// IsAlive: 读内存状态（connected/disconnected），不做主动 ping
type MCPManager struct { ... }
func NewMCPManager(entries []MCPToolEntry) *MCPManager
func (m *MCPManager) Start(ctx context.Context) error
func (m *MCPManager) ListTools(ctx context.Context) ([]ToolDef, error)
func (m *MCPManager) CallTool(ctx context.Context, serverName, toolName string, params map[string]any) (ToolResult, error)
func (m *MCPManager) IsAlive(serverName string) bool
func (m *MCPManager) Shutdown()

// 结果后处理
type PostProcessor struct {
    MaxOutputTokens int
}
func (p *PostProcessor) Process(result ToolResult) ToolResult

// 配置加载
func LoadConfig(path string) (ToolsConfig, error)
```

## 数据流

```
启动时:
  internal tools   → init() 自注册 → ToolRegistry.entries[]
  tools.json CLI   → LoadConfig() → 构造 ToolDef → Register()
  tools.json MCP   → LoadConfig() → spawn 子进程 → tools/list → 构造 ToolDef → Register()

每轮对话前:
  EvaluateAvailability() → visible/hidden 标记
  List(agent)            → ToolDef[] → helm 的 system prompt

工具调用时:
  helm → Lookup(name) → ToolDef
       → Execute(def, params)
           ├── adapter.Execute(ref, params)
           │     ├── builtin: ref.Builtin(ctx, params)
           │     ├── cli:     exec.Command(binary, translate(params)...)
           │     └── mcp:     mcpManager.CallTool(server, tool, params)
           └── PostProcessor.Process(rawResult)
                 ├── 截断
                 ├── 格式化
                 └── 错误转换
       → ToolResult → helm 注入上下文
```
