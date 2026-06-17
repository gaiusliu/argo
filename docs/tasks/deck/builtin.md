---
module: deck
function: builtin
layer: 1
status: pending_test
notes: 回退到 pending_test — postProcess 签名改为 func(result, cfg TruncationConfig)，Execute 内部传入 t.truncation。NewBuiltinTool 新增 truncation 参数。
depends_on:
  internal:
    - postProcess
  external:
    - Tool（deck 包接口，chart 已定义规格，task: Tool.md）
---

# deck::builtin

## 职责

实现 Tool 接口的五个方法（Name / Description / Source / Execute / Concurrency），通过 handler 函数指针执行具体逻辑。Execute 内部按 Tool 自己的 `TruncationConfig` 调用 `postProcess(result, t.truncation)` 做结果截断（详见 [DEC-007](../../deck/decisions.md#dec-007工具结果后处理方案重写supersedes-原-dec-007-和-dec-009)）。

## 签名

```go
type builtinTool struct {
    name        string
    description string
    handler     func(ctx context.Context, params map[string]any) (ToolResult, error)
    concurrency ConcurrencyMode
    truncation  TruncationConfig  // 截断配置，NewBuiltinTool 时传入
}

func NewBuiltinTool(name, description string, handler func(ctx context.Context, params map[string]any) (ToolResult, error), concurrency ConcurrencyMode, truncation TruncationConfig) *builtinTool
func (t *builtinTool) Name() string
func (t *builtinTool) Description() string
func (t *builtinTool) Source() ToolSource
func (t *builtinTool) Execute(ctx context.Context, params map[string]any) (ToolResult, error)
func (t *builtinTool) Concurrency() ConcurrencyMode
```

## 依赖详情

### 已就绪（可 import 使用）
- `Tool` — 接口（task: [Tool.md](Tool.md)）
- `ToolResult` · `ConcurrencyMode` · `TruncationConfig` · `TruncationStrategy` — 基础类型

### 待实现（需等待）
- `postProcess(result, cfg TruncationConfig)` — 新签名后 Execute 调用（task: [postProcess.md](postProcess.md)）

## 测试场景

以下测试方法位于 `deck/builtin_test.go`：

| # | 测试方法 | 场景 |
|---|---------|------|
| 1 | TestBuiltinTool_Name | Name() 返回构造时传入的 name（普通英文名、中文名、空字符串、特殊字符） |
| 2 | TestBuiltinTool_Description | Description() 返回构造时传入的 description（普通英文、中文、空字符串、长文本） |
| 3 | TestBuiltinTool_Source | Source() 始终返回 ToolSourceBuiltin，与构造参数无关 |
| 4 | TestBuiltinTool_Concurrency | Concurrency() 返回构造时传入的 ConcurrencyMode（Concurrent / Sequential） |
| 5 | TestBuiltinTool_Execute_CallsHandler | Execute() 调用 handler 并返回其结果，验证 handler 被调用、传入正确的 ctx 和 params |
| 6 | TestBuiltinTool_Execute_HandlerError | handler 返回 error 时，Execute() 正确传播该 error |
| 7 | TestBuiltinTool_Execute_PostProcess_Head | truncation=head(20) + 超长 Output：保留前 20% 字符，metadata.truncated=true、truncation_strategy="head" |
| 8 | TestBuiltinTool_Execute_PostProcess_Tail | truncation=tail(20) + 超长 Output：保留后 20% 字符，metadata.truncated=true、truncation_strategy="tail" |
| 9 | TestBuiltinTool_Execute_PostProcess_HeadAndTail | truncation=HeadAndTail(20,20) + 超长 Output：保留头尾各 20%，metadata.truncation_strategy="HeadAndTail" |
| 10 | TestBuiltinTool_Execute_PostProcess_NotTriggered | Output < MaxOutputChars 时不截断，Output 原样返回，metadata.truncated=false |
| 11 | TestBuiltinTool_Execute_HandlerError_OutputTruncated | handler 返回 error + 超长 Output 时仍调用 postProcess 截断 + 透传 error |
| 12 | TestBuiltinTool_Execute_ContextCancelled | context 已取消时，Execute() 返回 context error |
| 13 | TestBuiltinTool_Execute_NilHandler | handler 为 nil 时，触发 panic 或返回 error（边界防御） |
| 14 | TestBuiltinTool_Execute_NilParams | params 为 nil 时正常执行，不 panic |
| 15 | TestBuiltinTool_ImplementsToolInterface | 编译时类型断言验证 *builtinTool 满足 Tool 接口 |

## 实现状态

> 由 gen-code 填充。初始为空。gen-code 完成后标注实现文件和测试通过情况。
