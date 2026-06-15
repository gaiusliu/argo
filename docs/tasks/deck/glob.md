---
module: deck/tools/builtin
function: glob
layer: 2
status: pending_test
depends_on:
  internal:
    - Register（deck 包，task: ../Register.md）
    - builtinTool（deck 包，task: ../builtinTool.md）
  external:
    - path/filepath（标准库）
---

# builtin::glob

## 职责

通过 `init()` 调用 `deck.Register` 注册 glob 工具。handler 调用 `filepath.Glob` 执行文件名模式匹配。参数：params["pattern"]。

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

### 待实现（需等待）
- `deck.builtinTool` 的 Execute 方法（task: [../builtinTool.md](builtinTool.md)）

## 测试场景

> 由 gen-test 填充，初始为空。gen-test 完成后在此列出测试方法名和覆盖的场景。

## 实现状态

> 由 gen-code 填充。初始为空。gen-code 完成后标注实现文件和测试通过情况。
