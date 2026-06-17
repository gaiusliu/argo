---
module: deck
function: LoadConfig
layer: 1
status: pending_test
depends_on:
  internal:
    - Register
    - cliTool
    - MCPManager
  external:
    - ToolsConfig（deck 包基础类型，调度器已生成）
    - TruncationConfig（types.go，postProcess task 内新增）
    - os（标准库，读取 ~/.argo/tools.json）
---

# deck::LoadConfig

## 职责

解析 `~/.argo/tools.json` 配置文件。CLI 列表：读取每条的可选 `truncation` 字段 → 构造 `TruncationConfig` → 构造 cliTool 后 Register。MCP 列表：读取每条的可选 `truncation` 字段 → 构造 `TruncationConfig` → 构造 MCPManager → Start → ListTools 获取清单 → 构造 mcpTool 后 Register。省略 `truncation` 字段时默认 tail(20)。处理文件不存在 / JSON 格式错误等异常。

## 签名

```go
func LoadConfig(cfg ToolsConfig, mcp *MCPManager) error
```

## 依赖详情

### 已就绪（可 import 使用）
- `ToolsConfig` · `CLIToolEntry` · `MCPToolEntry` · `TruncationConfigJSON` — 基础类型，调度器已生成
- `LoadConfig()` — 配置文件读取（调度器已生成的基础函数，读取并反序列化 ~/.argo/tools.json）

### 待实现（需等待）
- `Register` — 注册工具到 map（task: [Register.md](Register.md)）
- `cli` — CLI 工具适配器（task: [cli.md](cli.md)）
- `MCPManager` — MCP 子进程管理（task: [MCPManager-mcpTool.md](MCPManager-mcpTool.md)）

## 测试场景

> 由 gen-test 填充，初始为空。gen-test 完成后在此列出测试方法名和覆盖的场景。

## 实现状态

> 由 gen-code 填充。初始为空。gen-code 完成后标注实现文件和测试通过情况。
