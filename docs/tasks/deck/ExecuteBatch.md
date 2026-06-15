---
module: deck
function: ExecuteBatch
layer: 1
status: pending_test
depends_on:
  internal:
    - Lookup
  external:
    - Tool（deck 包接口，chart 已定义规格）
---

# deck::ExecuteBatch

## 职责

批量执行工具调用。按 Tool.Concurrency() 分区：concurrent 组 goroutine + sync.WaitGroup 并行执行，sequential 组 for 循环串行执行。合并所有结果返回。

## 签名

```go
func ExecuteBatch(ctx context.Context, calls []ToolCall) []ToolResult
```

## 依赖详情

### 已就绪（可 import 使用）
- `Tool` — 接口，提供 Concurrency() 判断分区（task: [Tool.md](Tool.md)）
- `ToolCall` · `ToolResult` — 基础类型，调度器已生成

### 待实现（需等待）
- `Lookup` — 按名查找 Tool 实例（task: [Lookup.md](Lookup.md)）

## 测试场景

> 由 gen-test 填充，初始为空。gen-test 完成后在此列出测试方法名和覆盖的场景。

## 实现状态

> 由 gen-code 填充。初始为空。gen-code 完成后标注实现文件和测试通过情况。
