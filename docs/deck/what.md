# deck 行为规约

## 概述

deck 负责工具的注册、发现和调用。对上（helm）暴露包级函数 `List` / `Lookup` / `ExecuteBatch`，对下通过 `Tool` 接口统一 CLI / MCP / builtin 三种工具来源。

## 三大工具来源

### 内部工具（builtin）

```
tools/builtin/ 包下各 .go 文件通过 init() 调用 deck.Register(&builtinTool{...})
cmd 入口 import _ "argo/deck/tools/builtin" 触发所有 init()
```

### 外部 CLI 工具

```
~/.argo/tools.json → cli 列表 → LoadFromConfig() 时构造 cliTool → deck.Register()
```

### MCP 工具

```
~/.argo/tools.json → mcp 列表 → MCPManager.Start() spawn 子进程 → tools/list 获取清单 → 构造 mcpTool → deck.Register()
```

### tools.json 格式

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

---

## Tool 接口

```go
type ToolSource string

const (
    ToolSourceBuiltin ToolSource = "builtin"
    ToolSourceCLI     ToolSource = "cli"
    ToolSourceMCP     ToolSource = "mcp"
)

type Tool interface {
    Name() string
    Description() string
    Source() ToolSource
    Execute(ctx context.Context, params map[string]any) (ToolResult, error)
    Concurrency() ConcurrencyMode
}
```

三种实现：

| 实现 | Name | Description | Source | Execute |
|------|------|------|--------|------|
| builtinTool | 编译时指定 | 编译时指定 | `ToolSourceBuiltin` | handler 函数指针 |
| cliTool | 配置文件指定 | 配置文件指定 | `ToolSourceCLI` | exec.Command |
| mcpTool | `"mcp__" + serverName + "__" + toolName` | tools/list 返回的 description | `ToolSourceMCP` | MCPManager.CallTool |

---

## 五大能力

### 能力 1：工具注册

三个 `Register` 时机：

- builtin：`tools/builtin/*.go` 的 `init()` 调用 `deck.Register()`
- CLI：`LoadFromConfig()` 读取 tools.json 的 cli 列表，构造 cliTool 后 `Register()`
- MCP：`MCPManager.Start()` 后 `tools/list` 获取清单，构造 mcpTool 后 `Register()`

同名冲突：builtin > 外部。CLI 和 MCP 平权——启动时报错，不做静默覆盖。builtin 的 `init()` 注册在所有外部 Register 之前完成（Go 运行时保证），不存在"外部先注册、builtin 后覆盖"的反向时序。

### 能力 2：工具发现

```go
tools := deck.List()  // 返回当前所有已注册工具
tool, ok := deck.Lookup("bash")  // 精确查找
```

### 能力 3：工具使用

```
helm → deck.Lookup(name) → Tool
     → Tool.Execute(ctx, params) → ToolResult
```

不做独立参数校验——CLI 工具无法预知参数 schema，所有错误统一走 `ToolResult.Status="error"`，由 LLM 自行修正。

### 能力 4：结果后处理

`Execute()` 内部调用 `postProcess()`：单工具输出超过 50,000 字符时截断，保留头 40% + 尾 60% + 截断标记。

### 能力 5：批量执行

```go
results := deck.ExecuteBatch(ctx, calls)
```

按 `Tool.Concurrency()` 分区：concurrent 组 goroutine 并行 + sequential 组串行执行。

## 边界情况

- 工具重名：builtin > 外部。CLI 和 MCP 同名冲突启动报错
- MCP 服务器断连：标记该服务器所有工具不可用
- MCP 服务器启动失败：记录诊断信息，该服务器工具不加入注册表
- 工具执行超时：按工具自身 timeout 配置终止，返回超时错误
- 工具未注册：Lookup 返回 false，helm 告知 LLM
