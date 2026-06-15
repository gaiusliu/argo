---
module: deck
function: cliTool
layer: 1
status: test_ready
depends_on:
  internal:
    - paramsToArgs
    - execShell
    - postProcess
  external:
    - Tool（deck 包接口，chart 已定义规格，task: Tool.md）
---

# deck::cliTool

## 职责

实现 Tool 接口，将外部 CLI 命令适配为 Tool。Execute 内部调用 paramsToArgs 转换参数 → execShell 执行命令 → postProcess 截断输出。

## 签名

```go
type cliTool struct {
    name        string
    description string
    binary      string
    concurrency  ConcurrencyMode
}

func (t *cliTool) Name() string
func (t *cliTool) Description() string
func (t *cliTool) Execute(ctx context.Context, params map[string]any) (ToolResult, error)
func (t *cliTool) Concurrency() ConcurrencyMode
```

## 依赖详情

### 已就绪（可 import 使用）
- `Tool` — 接口（task: [Tool.md](Tool.md)）
- `ToolResult` · `ConcurrencyMode` — 基础类型，调度器已生成

### 待实现（需等待）
- `paramsToArgs` — 参数转换（task: [paramsToArgs.md](paramsToArgs.md)）
- `execShell` — shell 调用（task: [execShell.md](execShell.md)）
- `postProcess` — 结果截断（task: [postProcess.md](postProcess.md)）

## 测试场景

> 由 gen-test 填充。gen-test 完成后在此列出测试方法名和覆盖的场景。

| # | 测试方法 | 场景 | 预期 |
|---|---------|------|------|
| 1 | TestCliTool_Name | Name() 返回构造时传入的 name（含中英文、空字符串、特殊字符） | 与构造函数传入值一致 |
| 2 | TestCliTool_Description | Description() 返回构造时传入的 description（含中英文、空字符串、长文本） | 与构造函数传入值一致 |
| 3 | TestCliTool_Source | Source() 始终返回 ToolSourceCLI，与构造参数无关 | 返回 "cli" |
| 4 | TestCliTool_Concurrency | Concurrency() 返回 Concurrent / Sequential | 与构造时传入的模式一致 |
| 5 | TestCliTool_Execute_Success | Execute() 正常路径：go 二进制 + 空 params，验证 paramsToArgs→execShell→postProcess 管线 | Status=success，Output 非空，无 error |
| 6 | TestCliTool_Execute_EmptyParams | Execute() 传入空 map[string]any | 正常执行无 error |
| 7 | TestCliTool_Execute_NilParams | Execute() 传入 nil params | 不 panic，正常执行 |
| 8 | TestCliTool_Execute_PassParamsToBinary | Execute() 编译辅助程序打印 os.Args，传入 string/int/bool 多种类型 params | Output 含按字母序的 --alpha=first、--beta=42、--gamma=true |
| 9 | TestCliTool_Execute_Truncation | Execute() 编译辅助 Go 二进制输出 100K 字节，验证 postProcess 截断 | Output 长度 ≤ MaxOutputChars+1000，含"输出被截断"标记 |
| 10 | TestCliTool_Execute_ShellError | Execute() 二进制不存在（execShell 失败） | 返回非 nil error |
| 11 | TestCliTool_Execute_ParamsError | Execute() params 含 nil value（paramsToArgs 失败） | 返回非 nil error |
| 12 | TestCliTool_Execute_UnsupportedParamType | Execute() params 含 []int 类型（paramsToArgs 失败） | 返回非 nil error |
| 13 | TestCliTool_Execute_ContextCancelled | Execute() context 已取消 | 返回非 nil error，Status 非 success |
| 14 | TestCliTool_Execute_ContextTimeout | Execute() 编译 sleep 辅助程序 + 50ms 超时 context | 硬断言：返回 timeout status 或 error |
| 15 | TestCliTool_Execute_EmptyBinary | Execute() binary 为空字符串 | 返回非 nil error，Status 非 success |
| 16 | TestCliTool_ImplementsToolInterface | 编译期 + 运行时接口合规检查 | 编译通过 + 接口方法可达 |

## 实现状态

> 由 gen-code 填充。初始为空。gen-code 完成后标注实现文件和测试通过情况。
