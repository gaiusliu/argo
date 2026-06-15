# deck 实现设计

## 概述

deck 提供 `Tool` 接口作为工具的统一抽象。CLI / MCP / builtin 三种来源分别实现此接口，helm 通过包级函数 `List` / `Lookup` 获取工具，调用 `Execute` 执行。

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

## 三种 Tool 实现

三个 struct 均为 unexported，由 Register / LoadFromConfig 构造，helm 只通过 Tool 接口交互。

### builtinTool

通过 handler 函数指针注册。参考 Claude Code SDK 的 SimpleTool 模式。

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | `string` | 工具名，编译时指定 |
| `description` | `string` | 工具描述 |
| `handler` | `func(ctx, params) (ToolResult, error)` | 具体执行逻辑 |
| `concurrency` | `ConcurrencyMode` | 并发模式 |

方法逻辑：
- `Name()` / `Description()` / `Source()` / `Concurrency()` → 直接返回对应字段
- `Execute(ctx, params)` → 调用 `handler(ctx, params)`，结果经 `postProcess` 截断后返回

### cliTool

外部 CLI 命令的 Tool 适配器。

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | `string` | 工具名，由配置文件指定 |
| `description` | `string` | 工具描述 |
| `binary` | `string` | 可执行文件名或路径 |
| `concurrency` | `ConcurrencyMode` | 并发模式 |

方法逻辑：
- `Name()` / `Description()` / `Source()` / `Concurrency()` → 直接返回对应字段
- `Execute(ctx, params)` → `paramsToArgs(params)` 转换参数 → `execShell(ctx, binary, args)` 执行 → `postProcess` 截断

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

方法逻辑：
- `Name()` → 拼接 `"mcp__" + serverName + "__" + toolName`
- `Description()` / `Source()` / `Concurrency()` → 直接返回对应字段
- `Execute(ctx, params)` → 委托 `manager.CallTool(ctx, serverName, toolName, params)`，结果经 `postProcess` 截断

## 包级 API

| 函数 | 签名 | 职责 |
|------|------|------|
| `Register` | `func(t Tool) error` | 注册工具到进程唯一 map。同名冲突返回 error（builtin > 外部，CLI/MCP 同名报错） |
| `LoadFromConfig` | `func(cfg ToolsConfig, mcp *MCPManager) error` | 解析 tools.json：CLI 条目 → 构造 cliTool → Register；MCP 条目 → Start → ListTools → 构造 mcpTool → Register |
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
| `CLIToolEntry` | `Name`, `Description` | `name`, `description` |
| `MCPToolEntry` | `Name`, `Command []string` | `name`, `command` |

## 内部辅助函数

| 函数 | 签名 | 职责 |
|------|------|------|
| `postProcess` | `func(ToolResult) ToolResult` | 输出 > MaxOutputChars+TruncationMinSavings 时截断：头 40% + 标记 + 尾 60% |
| `execShell` | `func(ctx, binary, ...args) ToolResult` | 执行 shell 命令，合并 stdout+stderr |
| `paramsToArgs` | `func(params map[string]any) ([]string, error)` | `{key: val}` → `["--key=val"]`，按 key 排序。nil value / 不支持类型返回 error |

## 数据流

```
启动时:
  builtin init() → deck.Register() → tools[name]
  LoadConfig() → CLI 条目 → 构造 cliTool → deck.Register()
              → MCP 条目 → spawn 子进程 → tools/list → 构造 mcpTool → deck.Register()

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
