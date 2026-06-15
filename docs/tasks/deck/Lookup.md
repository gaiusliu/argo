---
module: deck
function: Lookup
layer: 0
status: test_ready
depends_on:
  internal: []
  external:
    - Tool（deck 包接口）
    - unexported tools map（调度器已生成）
---

# deck::Lookup

## 职责

按名精确查找工具。命中返回 (Tool, true)，未命中返回 (nil, false)。helm 据此告知 LLM 工具不可用。

## 签名

```go
func Lookup(name string) (Tool, bool)
```

## 依赖详情

### 已就绪（可 import 使用）
- `Tool` — 接口，chart 已定义规格
- `tools` map — 包级 unexported map，调度器已生成

### 待实现（需等待）
无

## 测试场景

| 测试方法 | 场景 | 预期 |
|----------|------|------|
| TestLookup_Hit | 已注册工具名 | 返回 (Tool, true) |
| TestLookup_Miss | 未注册工具名 | 返回 (nil, false) |
| TestLookup_EmptyName | 空 map / 有工具 map 两种状态 | 均返回 (nil, false) |
| TestLookup_MultipleTools | 多工具注册后精确查找 | 每个 name 精确命中对应工具 |
| TestLookup_AfterDelete | 工具从 map 删除后 | (nil, false) |
| TestLookup_DifferentSources | CLI/Builtin/MCP 三种来源 | Source / Description / Concurrency 均正确 |
| TestLookup_RegisterConflictPreservesOriginal | builtin 注册 → 外部同名被拒 → Lookup | 返回原 builtin，非 nil 非外部 |
| TestLookup_NilEntry | tools map 脏 nil 条目 | Lookup 返回 (nil, false) |

## 实现状态

> 由 gen-code 填充。初始为空。gen-code 完成后标注实现文件和测试通过情况。
