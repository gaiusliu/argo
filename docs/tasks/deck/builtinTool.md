---
module: deck
function: builtinTool
layer: 1
status: test_ready
depends_on:
  internal:
    - postProcess
  external:
    - Tool（deck 包接口，chart 已定义规格，task: Tool.md）
---

# deck::builtinTool

## 职责

实现 Tool 接口的四个方法（Name / Description / Execute / Concurrency），通过 handler 函数指针执行具体逻辑。Execute 内部调用 postProcess 做结果截断。

## 签名

```go
type builtinTool struct {
    name        string
    description string
    handler     func(ctx context.Context, params map[string]any) (ToolResult, error)
    concurrency  ConcurrencyMode
}

func (t *builtinTool) Name() string
func (t *builtinTool) Description() string
func (t *builtinTool) Execute(ctx context.Context, params map[string]any) (ToolResult, error)
func (t *builtinTool) Concurrency() ConcurrencyMode
```

## 依赖详情

### 已就绪（可 import 使用）
- `Tool` — 接口（task: [Tool.md](Tool.md)）
- `ToolResult` · `ConcurrencyMode` — 基础类型，调度器已生成

### 待实现（需等待）
- `postProcess` — 内部函数，Execute 调用（task: [postProcess.md](postProcess.md)）

## 测试场景

以下全部 12 个测试方法位于 `deck/builtinTool_test.go`：

| # | 测试方法 | 场景 |
|---|---------|------|
| 1 | TestBuiltinTool_Name | Name() 返回构造时传入的 name（普通英文名、中文名、空字符串、特殊字符） |
| 2 | TestBuiltinTool_Description | Description() 返回构造时传入的 description（普通英文、中文、空字符串、长文本） |
| 3 | TestBuiltinTool_Source | Source() 始终返回 ToolSourceBuiltin，与构造参数无关 |
| 4 | TestBuiltinTool_Concurrency | Concurrency() 返回构造时传入的 ConcurrencyMode（Concurrent / Sequential） |
| 5 | TestBuiltinTool_Execute_CallsHandler | Execute() 调用 handler 并返回其结果，验证 handler 被调用、传入正确的 ctx 和 params |
| 6 | TestBuiltinTool_Execute_HandlerError | handler 返回 error 时，Execute() 正确传播该 error |
| 7 | TestBuiltinTool_Execute_PostProcess | handler 返回超长 Output 时，Execute 内部经 postProcess 截断，验证截断标记和省略字符数 |
| 8 | TestBuiltinTool_Execute_PostProcess_NotTriggered | handler 返回小输出时不触发截断，Output 原样返回 |
| 9 | TestBuiltinTool_Execute_ContextCancelled | context 已取消时，Execute() 返回 context error |
| 10 | TestBuiltinTool_Execute_NilHandler | handler 为 nil 时，触发 panic 或返回 error（边界防御） |
| 11 | TestBuiltinTool_Execute_NilParams | params 为 nil 时正常执行，不 panic |
| 12 | TestBuiltinTool_ImplementsToolInterface | 编译时类型断言验证 *builtinTool 满足 Tool 接口 |

## 实现状态

> 由 gen-code 填充。初始为空。gen-code 完成后标注实现文件和测试通过情况。
