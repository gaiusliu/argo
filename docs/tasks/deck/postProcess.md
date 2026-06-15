---
module: deck
function: postProcess
layer: 0
status: code_done
depends_on:
  internal: []
  external:
    - ToolResult（deck 包内基础类型，调度器已生成）
---

# deck::postProcess

## 职责

对 ToolResult.Output 做字节截断。仅在 `len(Output) > MaxOutputChars + TruncationMinSavings` 时触发（默认 50000 + 200 = 50200），避免截断标记 + head + tail 比原文还长。触发后保留头部 40% + 尾部 60%，中间插入截断标记。

## 签名

```go
func postProcess(result ToolResult) ToolResult
```

## 依赖详情

### 已就绪（可 import 使用）
- `ToolResult` — 基础类型，调度器已生成
- `MaxOutputChars` — 常量，默认 50000，调度器已生成
- `TruncationMinSavings` — 常量，默认 200，调度器已生成

### 待实现（需等待）
无

## 测试场景

### 测试方法名和覆盖场景

以下全部 12 个测试方法位于 `deck/postprocess_test.go`：

| # | 测试方法 | 场景 |
|---|---------|------|
| 1 | TestPostProcess_OutputLessThanLimit | 输出 100 字符，不截断 |
| 2 | TestPostProcess_OutputEqualToLimit | 输出 = 50000，不截断 |
| 3 | TestPostProcess_WithinBufferZone | 输出 50001，在 TruncationMinSavings=200 缓冲区内，不截断 |
| 4 | TestPostProcess_ExceedsBufferZone | 输出 50201，刚好越过缓冲区，触发截断 |
| 5 | TestPostProcess_OutputFarExceedsLimit | 输出 200000，大幅截断 |
| 6 | TestPostProcess_StatusAndMetadataPreserved | Status/Metadata 截断前后不变 |
| 7 | TestPostProcess_EmptyOutput | 空字符串不截断 |
| 8 | TestPostProcess_OutputWithNullCharacters | 含 \x00 字节不 panic |
| 9 | TestPostProcess_OutputWithUnicode | 含多字节 UTF-8 不 panic |
| 10 | TestPostProcess_HeadTailNoOverlap | 50300 字符（越过缓冲区），head/tail 无重叠，中间部分被标记替换 |
| 11 | TestPostProcess_MarkerFormat | 截断标记格式验证 |
| 12 | TestPostProcess_HugeOutput | 1MB 不 panic |

## 实现状态

| 项目 | 状态 |
|------|------|
| 测试文件 | deck/postprocess_test.go — 已就绪 |
| 实现文件 | deck/postprocess.go — 已实现 |
| 测试通过 | 全部 12 个测试通过 |
