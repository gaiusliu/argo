# deck 实现设计

## 概述

deck 提供 `Tool` 接口作为工具的统一抽象。CLI / MCP / builtin 三种来源分别实现此接口，helm 通过包级函数 `List` / `Lookup` 获取工具，调用 `Execute` 执行。

## 内部结构

```
deck/
├── tool.go              // Tool 接口 + ToolResult 类型定义
├── types.go             // ToolSource / ConcurrencyMode / TruncationStrategy / ToolCall / ToolsConfig / CLIToolEntry / MCPToolEntry
├── builtin.go           // builtinTool 实现（handler 函数指针）
├── cli.go               // cliTool 实现
├── exec_shell.go        // execShell 内部函数
├── params_to_args.go    // paramsToArgs 内部函数
├── post_process.go       // postProcess 内部函数（按 TruncationConfig 截断，写 metadata）
├── truncation.go        // writeTruncation / cleanupOldTruncations
├── register.go          // Register + unexported tools map + ToolError
├── list.go              // List 函数
├── lookup.go            // Lookup 函数
├── mcp.go               // mcpTool 实现 + MCPManager（待 Layer 1 落地）
├── batch.go             // ExecuteBatch（待创建）
├── config.go            // LoadConfig（待创建）
└── tools/
    └── builtin/          // 六个内置工具（待 Layer 2 落地，每个在 init() 声明 TruncationStrategy）
        ├── bash.go
        ├── read.go
        ├── write.go
        ├── edit.go
        ├── grep.go
        └── glob.go
```

## 核心类型

### Tool 接口（5 方法）

| 方法 | 返回值 | 职责 |
|------|--------|------|
| `Name()` | `string` | 工具唯一标识名 |
| `Description()` | `string` | 自然语言描述，供 LLM 理解工具用途 |
| `Source()` | `ToolSource` | 工具来源：`ToolSourceBuiltin` / `ToolSourceCLI` / `ToolSourceMCP` |
| `Execute(ctx, params)` | `(ToolResult, error)` | 执行工具调用，错误通过 Status 字段返回，不预校验参数 |
| `Concurrency()` | `ConcurrencyMode` | `Concurrent`（可并行）或 `Sequential`（串行） |

### ToolSource 枚举

| 常量 | 含义 |
|------|------|
| `ToolSourceBuiltin` | 编译时注册的内置工具，同名冲突时优先 |
| `ToolSourceCLI` | 通过 `~/.argo/tools.json` cli 列表配置的外部 CLI 工具 |
| `ToolSourceMCP` | 通过 `~/.argo/tools.json` mcp 列表配置的 MCP 协议工具 |

### 其他基础类型

| 类型 | 字段 | 说明 |
|------|------|------|
| `ConcurrencyMode` | — | 字符串类型，值 `"concurrent"` / `"sequential"` |
| `ToolResult` | `Output string`, `Status string`, `Metadata map` | 工具执行结果。Status 取值：`"success"` / `"error"` / `"timeout"` |
| `ToolCall` | `Name string`, `Params map` | 一次工具调用请求 |
| `TruncationStrategy` | — | iota 整型枚举。`TruncationHead` / `TruncationTail` / `TruncationHeadAndTail`，各带 `String()` 方法返回 `"head"` / `"tail"` / `"HeadAndTail"` |
| `TruncationConfig` | `Strategy TruncationStrategy`, `Ratio int`, `TailRatio int` | 截断配置。Ratio 主保留百分点 1~80（默认 20 = 20%），TailRatio 仅 HeadAndTail 启用（默认 20 = 20%）。`Validate()` 方法校验 Ratio > 0 且 ≤ 80。详见 [DEC-010](decisions.md#dec-010truncationconfig-ratiotailratio-改-int-百分点) |
| `TruncationConfigJSON` | `Strategy string`, `Ratio *int`, `TailRatio *int` | JSON 反序列化用，用于 `tools.json` 中 CLI/MCP 条目的可选 `truncation` 字段。值含义与 `TruncationConfig` 相同（百分点整数）|

## 三种 Tool 实现

三个 struct 均为 unexported，由 Register / LoadConfig 构造，helm 只通过 Tool 接口交互。

### builtinTool

通过 handler 函数指针注册。参考 Claude Code SDK 的 SimpleTool 模式。

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | `string` | 工具名，编译时指定 |
| `description` | `string` | 工具描述 |
| `handler` | `func(ctx, params) (ToolResult, error)` | 具体执行逻辑 |
| `concurrency` | `ConcurrencyMode` | 并发模式 |
| `truncation` | `TruncationConfig` | 截断配置，`NewBuiltinTool` 时传入默认值 |

方法逻辑：
- `Name()` / `Description()` / `Source()` / `Concurrency()` → 直接返回对应字段
- `Execute(ctx, params)` → 调用 `handler(ctx, params)`，结果经 `postProcess(result, t.truncation)` 截断后返回

### cliTool

外部 CLI 命令的 Tool 适配器。

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | `string` | 工具名，由配置文件指定 |
| `description` | `string` | 工具描述 |
| `binary` | `string` | 可执行文件名或路径 |
| `concurrency` | `ConcurrencyMode` | 并发模式 |
| `truncation` | `TruncationConfig` | 截断配置，默认 tail(20)，可由 tools.json 覆盖 |

方法逻辑：
- `Name()` / `Description()` / `Source()` / `Concurrency()` → 直接返回对应字段
- `Execute(ctx, params)` → `paramsToArgs(params)` 转换参数 → `execShell(ctx, binary, args)` 执行 → `postProcess(result, t.truncation)` 截断

### mcpTool

MCP 协议工具的 Tool 适配器。Name 由 `"mcp__" + serverName + "__" + toolName` 推导，避免不同 MCP 服务器间工具名冲突。

| 字段 | 类型 | 说明 |
|------|------|------|
| `description` | `string` | 工具描述，由 MCP `tools/list` 返回 |
| `serverName` | `string` | 所属 MCP 服务器名 |
| `toolName` | `string` | MCP 工具原始名称 |
| `inputSchema` | `json.RawMessage` | 参数 JSON Schema |
| `manager` | `*MCPManager` | 关联的 MCP 管理器 |
| `concurrency` | `ConcurrencyMode` | 并发模式 |
| `truncation` | `TruncationConfig` | 截断配置，默认 tail(20)，可由 tools.json 覆盖 |

方法逻辑：
- `Name()` → 拼接 `"mcp__" + serverName + "__" + toolName`
- `Description()` / `Source()` / `Concurrency()` → 直接返回对应字段
- `Execute(ctx, params)` → 委托 `manager.CallTool(ctx, serverName, toolName, params)`，结果经 `postProcess(result, t.truncation)` 截断

## 包级 API

| 函数 | 签名 | 职责 |
|------|------|------|
| `Register` | `func(t Tool) error` | 注册工具到进程唯一 map。同名冲突返回 error（builtin > 外部，CLI/MCP 同名报错） |
| `LoadConfig` | `func(cfg ToolsConfig, mcp *MCPManager) error` | 解析 tools.json：CLI 条目 → 构造 cliTool → Register；MCP 条目 → Start → ListTools → 构造 mcpTool → Register。CLI/MCP 条目内嵌可选 `truncation` 字段，省略时用默认 tail(20) |
| `List` | `func() []Tool` | 返回所有已注册工具列表，排序稳定 |
| `Lookup` | `func(name string) (Tool, bool)` | 精确查找，命中返回 (Tool, true)，未命中 (nil, false) |
| `ExecuteBatch` | `func(ctx, calls []ToolCall) []ToolResult` | 按 ConcurrencyMode 分区：Concurrent 组 goroutine 并行，Sequential 组串行执行，合并结果 |

## MCPManager

MCP 子进程生命周期管理。

| 方法 | 职责 |
|------|------|
| `NewMCPManager(entries)` | 根据 MCPToolEntry 列表创建管理器 |
| `Start(ctx)` | spawn 子进程，建立 stdio 握手，每个进程起 goroutine 监听退出 |
| `ListTools(ctx)` | 调用 MCP `tools/list`，返回 mcpTool 列表 |
| `CallTool(ctx, server, tool, params)` | 向指定 MCP 服务器发起工具调用，返回 ToolResult |
| `IsAlive(server)` | 返回子进程是否运行中（读内存状态，不主动 ping） |
| `Shutdown()` | 关闭所有子进程，释放资源 |

## 配置文件

`~/.argo/tools.json` 结构：

```
{
  "cli":  [{"name": "...", "description": "..."}],
  "mcp":  [{"name": "...", "command": ["...", "..."]}]
}
```

对应类型：

| 类型 | 字段 | JSON 字段 |
|------|------|-----------|
| `ToolsConfig` | `CLI []CLIToolEntry`, `MCP []MCPToolEntry` | `cli`, `mcp` |
| `CLIToolEntry` | `Name`, `Description`, `Truncation *TruncationConfigJSON` | `name`, `description`, `truncation`（可选） |
| `MCPToolEntry` | `Name`, `Command []string`, `Truncation *TruncationConfigJSON` | `name`, `command`, `truncation`（可选） |

## 内部辅助函数

| 函数 | 签名 | 职责 |
|------|------|------|
| `postProcess` | `func(result ToolResult, cfg TruncationConfig) ToolResult` | 输出 > MaxOutputChars 时按 cfg.Strategy 截断（head / tail / HeadAndTail），自动写 metadata。详见 [DEC-007](decisions.md#dec-007工具结果后处理方案重写supersedes-原-dec-007-和-dec-009) |
| `execShell` | `func(ctx, binary, ...args) ToolResult` | 执行 shell 命令，合并 stdout+stderr |
| `paramsToArgs` | `func(params map[string]any) ([]string, error)` | `{key: val}` → `["--key=val"]`，按 key 排序。nil value / 不支持类型返回 error |
| `writeTruncation` | `func(content string) (string, error)` | 把完整输出写到 `~/.argo/truncation/tool-<ts>-<rand>.log`，返回路径 |
| `cleanupOldTruncations` | `func(retention time.Duration)` | 清理超出保留期的 truncation 文件（默认 7 天）|

## 数据流

```
启动时:
  builtin init() → deck.Register() → tools[name]
  LoadConfig() → CLI 条目（含可选 truncation）→ 构造 cliTool → deck.Register()
              → MCP 条目（含可选 truncation）→ spawn 子进程 → tools/list → 构造 mcpTool → deck.Register()

每轮对话前:
  deck.List() → 滤除不可用工具 → []Tool → helm 取 Name()+Description() → 注入 system prompt

运行时（单工具）:
  helm → deck.Lookup("gh") → Tool
       → LLM 决定参数 → Tool.Execute(ctx, params) → ToolResult
       → 参数错误时 stderr 原样返回给 LLM，LLM 自行修正

运行时（批量）:
  helm → deck.ExecuteBatch(ctx, []ToolCall{...})
       → Lookup 获取每个 Tool → 按 Concurrency() 分区 → 并发/串行执行
```
