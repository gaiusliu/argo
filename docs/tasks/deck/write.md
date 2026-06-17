---
module: deck/tools/builtin
function: write
layer: 2
status: pending_test
depends_on:
  internal:
    - Register（deck 包，task: ../Register.md）
    - builtinTool（deck 包，task: ../builtin.md）
  external:
    - os（标准库）
---

# builtin::write

## 职责

通过 `init()` 调用 `deck.Register` 注册 write 工具。handler 调用 `os.WriteFile` 创建或覆写文件，父目录自动创建。参数：params["filePath"]、params["content"]。

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
- `deck.builtinTool` 的 Execute 方法（task: [../builtin.md](builtin.md)）

## 测试场景

> 由 gen-test 填充，初始为空。gen-test 完成后在此列出测试方法名和覆盖的场景。

## 实现状态

> 由 gen-code 填充。初始为空。gen-code 完成后标注实现文件和测试通过情况。
