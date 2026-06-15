---
module: deck
function: MCPManager + mcpTool
layer: 1
status: pending_test
depends_on:
  internal:
    - postProcess
  external:
    - Tool（deck 包接口，chart 已定义规格，task: Tool.md）
---

# deck::MCPManager + mcpTool

## 职责

MCPManager：管理 MCP 子进程生命周期（Start 启动 → IsAlive 健康检查 → ListTools 获取工具清单 → CallTool 发起工具调用 → Shutdown 清理）。通过后台 goroutine 运行 cmd.Wait() 被动检测子进程退出。

mcpTool：实现 Tool 接口。Name 由 `"mcp__" + serverName + "__" + toolName` 推导。Execute 委托 MCPManager.CallTool，再经 postProcess 截断。

> 两者合并为一个 task，避免循环依赖。

## 签名

```go
type MCPManager struct { ... }

func NewMCPManager(entries []MCPToolEntry) *MCPManager
func (m *MCPManager) Start(ctx context.Context) error
func (m *MCPManager) ListTools(ctx context.Context) ([]mcpTool, error)
func (m *MCPManager) CallTool(ctx context.Context, serverName, toolName string, params map[string]any) (ToolResult, error)
func (m *MCPManager) IsAlive(serverName string) bool
func (m *MCPManager) Shutdown()

type mcpTool struct {
    description string
    serverName  string
    toolName    string
    inputSchema json.RawMessage
    manager     *MCPManager
    concurrency  ConcurrencyMode
}

func (t *mcpTool) Name() string
func (t *mcpTool) Description() string
func (t *mcpTool) Execute(ctx context.Context, params map[string]any) (ToolResult, error)
func (t *mcpTool) Concurrency() ConcurrencyMode
```

## 依赖详情

### 已就绪（可 import 使用）
- `Tool` — 接口（task: [Tool.md](Tool.md)）
- `ToolResult` · `ConcurrencyMode` · `MCPToolEntry` — 基础类型，调度器已生成
- `mcpTool` struct 定义 — 基础类型，调度器已生成

### 待实现（需等待）
- `postProcess` — 结果截断（task: [postProcess.md](postProcess.md)）

## 测试场景

> 由 gen-test 填充，初始为空。gen-test 完成后在此列出测试方法名和覆盖的场景。

## 实现状态

> 由 gen-code 填充。初始为空。gen-code 完成后标注实现文件和测试通过情况。
