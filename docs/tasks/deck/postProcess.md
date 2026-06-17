---
module: deck
function: postProcess
layer: 0
status: test_ready
depends_on:
  internal:
    - truncation（Layer 0，详见 [truncation.md](truncation.md)）
  external:
    - ToolResult（deck 包内基础类型，调度器已生成）
    - TruncationConfig（types.go，本 task 内新增）
    - truncation.Writer（truncation 包提供，详见 [DEC-012](../../deck/decisions.md#dec-012truncation-目录管理归属)）
---

# deck::postProcess

## 职责

对 ToolResult.Output 按 `TruncationConfig` 做截断。仅在 `len(Output) > MaxOutputChars` 时触发。按 `cfg.Strategy` 分派 head / tail / HeadAndTail 行为，按 `cfg.Ratio%` / `cfg.TailRatio%` 计算截断位置（byte 触发 + byte 切 + 向下取整到 rune 边界），自动写 metadata 字段（`truncated` / `original_length` / `kept_length` / `truncation_strategy` / `full_output_path`），追加截断标记（格式详见 [DEC-011](../../deck/decisions.md#dec-011截断标记格式)）。

## 签名

```go
func postProcess(result ToolResult, cfg TruncationConfig) ToolResult
```

## 依赖详情

### 已就绪（可 import 使用）
- `ToolResult` — 基础类型，调度器已生成
- `MaxOutputChars` — 常量，默认 50000，调度器已生成

### 本 task 内新增
- `TruncationStrategy` — iota 枚举（`TruncationHead` / `TruncationTail` / `TruncationHeadAndTail`），含 `String()` 方法
- `TruncationConfig` — struct（`Strategy` / `Ratio` / `TailRatio`），含 `Validate()` 方法

### 废弃（本 task 删除）
- `TruncationMinSavings` 常量

### 待实现（需等待）
无

## 截断行为

```
若 Output 长度 > MaxOutputChars：
    path, err := truncWriter.Write(result.Output)
    // err 降级策略详见 DEC-012 第 4 节
    按 cfg.Strategy 分派：
      head:       保留前 len × cfg.Ratio / 100 字节，向下取整到 rune 边界
      tail:       保留后 len × cfg.Ratio / 100 字节，向下取整到 rune 边界
      HeadAndTail:保留前 len × cfg.Ratio / 100 + 后 len × cfg.TailRatio / 100 字节，向下取整到 rune 边界
    追加截断标记：
      path 非空 → `\n[...truncated: <strategy>, original <N> chars; full output: <path>...]\n`
      path 为空 → `\n[...truncated: <strategy>, original <N> chars...]\n`（不含 path 段）
    metadata 写入：
      truncated=true
      original_length=N
      kept_length=len(got.Output)
      truncation_strategy=strategy
      full_output_path=path（失败时为空字符串或不写该字段）
    返回截断后的 ToolResult

若 Output 长度 ≤ MaxOutputChars：
    原样返回，metadata.truncated = false（不写其他截断字段）
```

截断标记格式详见 [DEC-011](../../deck/decisions.md#dec-011截断标记格式)；truncation 目录管理与失败降级详见 [DEC-012](../../deck/decisions.md#dec-012truncation-目录管理归属)。

## 测试场景

> 本节由 gen-test subagent 在测试代码生成后填充。
> 测试文件：`C:\Users\Gaiusliu\Desktop\code\argo\deck\post_process_test.go`

### TruncationStrategy.String() 测试

| 测试方法 | 覆盖场景 |
|----------|---------|
| `TestTruncationStrategy_String` | head/tail/HeadAndTail 三种策略的 String() 返回值 |

### TruncationConfig.Validate() 测试

| 测试方法 | 覆盖场景 |
|----------|---------|
| `TestTruncationConfig_Validate` | ratio=20/80 合法、ratio=0/-10/81 非法、HeadAndTail 下 tail_ratio=20/80 合法、tail_ratio=81/0 非法、ratio=0 + HeadAndTail 非法 |

### postProcess 不截断场景

| 测试方法 | 覆盖场景 |
|----------|---------|
| `TestPostProcess_NoTruncation_BelowThreshold` | outputLen=100 < MaxOutputChars，不截断，metadata.truncated=false |
| `TestPostProcess_NoTruncation_AtThreshold` | outputLen=50000 == MaxOutputChars，不截断（> 比较，不含等号） |

### postProcess Head 策略

| 测试方法 | 覆盖场景 |
|----------|---------|
| `TestPostProcess_HeadTruncation` | 60000 字符 > MaxOutputChars，Head 策略 ratio=20，保留前 12000 字符 |

### postProcess Tail 策略

| 测试方法 | 覆盖场景 |
|----------|---------|
| `TestPostProcess_TailTruncation` | 60000 字符 > MaxOutputChars，Tail 策略 ratio=20，保留后 12000 字符 |
| `TestPostProcess_TailTruncation_LargeOutput` | 200000 字符，Tail 策略 ratio=20，保留后 40000 字符 |

### postProcess HeadAndTail 策略

| 测试方法 | 覆盖场景 |
|----------|---------|
| `TestPostProcess_HeadAndTailTruncation` | 60000 字符，HeadAndTail ratio=20 tail_ratio=20，保留头 12000 + 尾 12000 |
| `TestPostProcess_HeadAndTailTruncation_LargeOutput` | 200000 字符，HeadAndTail ratio=20 tail_ratio=20 |
| `TestPostProcess_HeadAndTail_NoOverlap` | 100000 字符，验证头尾不重叠，kept_length < original_length |

### postProcess metadata 字段

| 测试方法 | 覆盖场景 |
|----------|---------|
| `TestPostProcess_Metadata_Truncated` | 截断时 metadata 包含 truncated/original_length/kept_length/truncation_strategy |
| `TestPostProcess_Metadata_NotTruncated` | 不截断时 metadata.truncated=false，原有字段透传，无截断专用字段 |
| `TestPostProcess_Metadata_Preserved` | 截断时原有 metadata 保留，并追加截断字段 |

### postProcess 边界与鲁棒性

| 测试方法 | 覆盖场景 |
|----------|---------|
| `TestPostProcess_EmptyOutput` | 空字符串输入：不截断、不 panic |
| `TestPostProcess_UTF8MultiByte` | 中文等多字节 UTF-8：不 panic、输出合法 UTF-8 |
| `TestPostProcess_OneMBOutput` | 1MB 输出：不 panic、Tail 策略保留后 10% |

### postProcess 自定义 Ratio

| 测试方法 | 覆盖场景 |
|----------|---------|
| `TestPostProcess_Head_CustomRatio` | Head 策略 ratio=30，保留前 30% |
| `TestPostProcess_Tail_CustomRatio` | Tail 策略 ratio=10，保留后 10% |
| `TestPostProcess_HeadAndTail_CustomRatio` | HeadAndTail 策略 ratio=15 + tail_ratio=25，头 15% + 尾 25% |

### postProcess 截断标记（DEC-011）

| 测试方法 | 覆盖场景 |
|----------|---------|
| `TestPostProcess_Marker_Head` | head 策略截断后 Output 末尾包含 `\n[...truncated: head, original <N> chars; full output: <path>...]\n` |
| `TestPostProcess_Marker_Tail` | tail 策略 marker 包含 `tail` |
| `TestPostProcess_Marker_HeadAndTail` | HeadAndTail 策略 marker 包含 `HeadAndTail` |
| `TestPostProcess_Marker_NotTriggered` | 未截断时 Output 不含截断标记 |
| `TestPostProcess_Marker_TruncationPath` | marker 中的路径与 `metadata.full_output_path` 一致 |
| `TestPostProcess_Marker_TruncationPath_Degraded` | truncation.Writer.Write 失败时 marker 不含 `; full output:` 段，`metadata.full_output_path` 为空字符串（DEC-012 降级策略）|

## 实现状态

> 本节由 gen-code subagent 在实现代码完成后填充。

| 项目 | 状态 |
|------|------|
| 测试文件 | `deck/post_process_test.go` — ✅ 已生成（gen-test 于 2026-06-17 完成，25 个测试方法，RED 验证通过） |
| 实现文件 | `deck/post_process.go` — 待 gen-code 生成 |
| 测试通过 | ⬜ 待 gen-code 完成后验证（当前 2 PASS / 23 FAIL，stub 正确触发 RED） |
