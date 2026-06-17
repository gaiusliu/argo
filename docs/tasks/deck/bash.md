---
module: deck/tools/builtin
function: bash
layer: 2
status: pending_test
depends_on:
  internal:
    - Register（deck 包，task: ../Register.md）
    - builtinTool（deck 包，task: ../builtin.md）
  external:
    - os/exec（标准库）
---

# builtin::bash

## 职责

通过 `init()` 调用 `deck.Register` 注册 bash 工具。handler 内部执行 `exec.Command("bash", "-c", cmd)`，将 params["command"] 作为 shell 命令执行。

## 签名

```go
func init()
```

handler 函数签名：
```go
func(ctx context.Context, params map[string]any) (deck.ToolResult, error)
```

## 依赖详情

### 已就绪（可 import 使用）
- `deck.Register` — 工具注册函数（task: [../Register.md](Register.md)）
- `builtinTool` struct — 基础类型，调度器已生成
- `deck.ToolResult` · `deck.Concurrent` — deck 包导出类型

### 待实现（需等待）
- `deck.builtinTool` 的 Execute 方法（task: [../builtin.md](builtin.md)）

## 测试场景

> 由 gen-test 填充，初始为空。gen-test 完成后在此列出测试方法名和覆盖的场景。

## 实现状态

> 由 gen-code 填充。初始为空。gen-code 完成后标注实现文件和测试通过情况。
