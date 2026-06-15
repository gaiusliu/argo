---
module: deck
function: paramsToArgs
layer: 0
status: code_done
depends_on:
  internal: []
  external: []
---

# deck::paramsToArgs

## 职责

将 `map[string]any` 转换为 `[]string` 命令行参数（`"--key=val"`），按 key 字母序排序。支持 bool / int / float64 / string 类型。nil value 或不支持的类型返回 error，不 panic。

## 签名

```go
func paramsToArgs(params map[string]any) ([]string, error)
```

## 依赖详情

### 已就绪（可 import 使用）
无（纯数据转换，不依赖任何模块内部类型或外部库）

### 待实现（需等待）
无

## 测试场景

> 由 gen-test 填充，初始为空。gen-test 完成后在此列出测试方法名和覆盖的场景。

| 方法名 | 场景 | 预期 |
|--------|------|------|
| TestParamsToArgs_EmptyMap | nil / 空 map | (nil, nil) |
| TestParamsToArgs_SinglePair | 单键值对 | ["--name=test"] |
| TestParamsToArgs_MultiplePairs | 多键值对，按 key 字母序 | ["--alpha=first","--beta=second","--gamma=third"] |
| TestParamsToArgs_SortedByKey | 排序稳定性，首项为最小 key | 3 次结果一致，首项 "--alpha=first" |
| TestParamsToArgs_BoolValue | bool true/false | "--enabled=true" / "--enabled=false" |
| TestParamsToArgs_NumericValue | int + float64 均接受（LLM 传的是 float64） | int(42/0/-5) 和 float64(10/3.14) |
| TestParamsToArgs_StringValue | string，含空格 | "--msg=hello world" |
| TestParamsToArgs_MixedTypes | 混合 bool/string/float64，count 用 float64 | 4 个参数按 key 排序 |
| TestParamsToArgs_NilValue | nil value | error |
| TestParamsToArgs_UnsupportedType | []string | error |
| TestParamsToArgs_UnsupportedSlice | []int | error |

## 实现状态

> 由 gen-code 填充。初始为空。gen-code 完成后标注实现文件和测试通过情况。
