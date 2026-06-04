# deck 实现设计

## 概述

deck 是工具层，提供 `ToolRegistry`（注册、可用性判定、发现）和 `ToolExecutor`（使用、结果后处理）。对上暴露为 helm 可注入的服务接口，对下通过 `Executor` interface + 三个实现（BuiltinExecutor / CLIExecutor / MCPExecutor）屏蔽三种来源的差异。

## 内部结构

```
deck/
├── registry.go       // ToolRegistry：注册、可用性判定、发现
├── executor.go       // ToolExecutor：使用、结果后处理
├── executors.go      // Executor 接口 + BuiltinExecutor / CLIExecutor / MCPExecutor
├── config.go         // LoadConfig() 读 tools.json
├── availability.go   // 可用性判定
├── postprocess.go    // 结果后处理
├── tools/            // 内部自建工具包（每个工具一个子包）
│   ├── bash/
│   ├── read/
│   ├── write/
│   └── tools.go      // [barrel 文件](../glossary.md#barrel-触发)，import 所有工具子包触发 init()
└── types.go          // ToolDef, ToolResult, Condition, Executor 接口
```

## 核心类型

> 具体案例见 [examples.md](examples.md)

```go
// Executor — 统一执行接口，三个实现各自适配
type Executor interface {
    Execute(ctx context.Context, params map[string]any) (ToolResult, error)
}

type BuiltinExecutor struct {
    Fn func(ctx context.Context, params map[string]any) (ToolResult, error)
}
func (e *BuiltinExecutor) Execute(ctx context.Context, params map[string]any) (ToolResult, error) {
    return e.Fn(ctx, params)
}

type CLIExecutor struct {
    Binary string
}
func (e *CLIExecutor) Execute(ctx context.Context, params map[string]any) (ToolResult, error) {
    // 将 params 转为命令行参数，exec.Command(binary, args...)
}

type MCPExecutor struct {
    ServerName string
    ToolName   string
    Manager    *MCPManager
}
func (e *MCPExecutor) Execute(ctx context.Context, params map[string]any) (ToolResult, error) {
    return e.Manager.CallTool(ctx, e.ServerName, e.ToolName, params)
}
```

```go
// ToolDef — 存工厂函数，每次 Lookup 动态生成新 Executor
type ToolDef struct {
    Name         string
    Description  string
    Parameters   jsonschema.Schema  // MCP/内部工具提供完整 schema；外部 CLI 为空
    buildExecutor func() Executor    // 工厂，Lookup 时调用生成新实例
    Conditions  []Condition         // MVP: 空=永远可见
    Concurrency ConcurrencyMode     // concurrent | sequential
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
// ToolRegistry
func LoadRegistry(toolsCfg ToolsConfig, mcpManager *MCPManager) (*ToolRegistry, error)
func (r *ToolRegistry) EvaluateAvailability(ctx context.Context) error
func (r *ToolRegistry) List(agentCfg AgentConfig) []ToolDef
    // 三步流水线：① 滤除 hidden → ② agent 权限滤除 → ③ 按名称排序
    // 注：agentCfg 是预留占位符，多 Agent 场景下控制不同工具权限。MVP 只有一个 Agent，实际调用传入 nil
func (r *ToolRegistry) Lookup(name string) (ToolDef, bool)
    // 返回的 ToolDef.buildExecutor 已包含即时生成的新 Executor 实例

// ToolExecutor — 名称查找 → 权限检查 → 分区 → 执行调度 → 包装 ToolResult
// 不做独立参数校验步骤。所有错误统一走 ToolResult.Status="error" 返回给 LLM 自行修正
func NewToolExecutor() *ToolExecutor
func (e *ToolExecutor) Execute(ctx context.Context, exec Executor, params map[string]any) ToolResult
func (e *ToolExecutor) ExecuteBatch(ctx context.Context, calls []ToolCall) []ToolResult
    // 按 ConcurrencyMode 拆组：
    //   concurrent → goroutine + sync.WaitGroup（上限 N）
    //   sequential → for 逐个执行

// ToolHooks — 执行阶段钩子
type ToolHooks struct {
    PreToolUse  func(ctx context.Context, def ToolDef, params map[string]any) error // nil=放行, error=阻断
}

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

// 结果后处理 —— MVP: 单工具字符上限 + 头 40% + 尾 60% + 标记
// 比例硬编码，不做可配置。所有调研项目均硬编码
type PostProcessor struct {
    MaxOutputChars int  // 默认 50,000
}
func (p *PostProcessor) Process(result ToolResult) ToolResult
    // if len(result.Output) > MaxOutputChars:
    //   head = Output[:MaxOutputChars*4/10]
    //   tail = Output[len-keepTail:]
    //   Output = head + "\n\n[... 输出被截断，省略 X 字符 ...]\n\n" + tail

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
  List(agentCfg)        → ① 滤除 hidden → ② 权限滤除 → ③ 排序 → ToolDef[]
                            → helm 注入 system prompt

工具调用时:
  helm → Lookup(name) → ToolDef
       → Executor = ToolDef.buildExecutor()  // 工厂生成新实例
       → Execute(Executor, params)
           ├── Executor.Execute(ctx, params)  // interface 多态：builtin/cli/mcp
           └── PostProcessor.Process(rawResult)
                 ├── 超限：头 40% + 尾 60% + 截断标记
                 └── 异常：stderr 原样包装，交给 LLM 自行修正
       → ToolResult → helm 注入上下文
```
