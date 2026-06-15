---
module: deck
function: List
layer: 0
status: test_ready
depends_on:
  internal: []
  external:
    - Tool（deck 包接口）
    - unexported tools map（调度器已生成）
---

# deck::List

## 职责

返回当前所有已注册工具的列表，排序稳定。供 helm 在每轮对话前获取工具清单并注入 system prompt。

## 签名

```go
func List() []Tool
```

## 依赖详情

### 已就绪（可 import 使用）
- `Tool` — 接口，chart 已定义规格
- `tools` map — 包级 unexported map，调度器已生成

### 待实现（需等待）
无

## 测试场景

> 由 gen-test 填充，初始为空。gen-test 完成后在此列出测试方法名和覆盖的场景。

| # | 测试方法 | 场景 | 预期 |
|---|---------|------|------|
| 1 | TestList_EmptyRegistry | 空注册表调用 List | 返回空切片（非 nil） |
| 2 | TestList_SingleTool | 注册 1 个工具后调用 List | 返回长度为 1 的切片，包含该工具 |
| 3 | TestList_MultipleTools | 注册 3 个工具（非字母序） | 返回全部 3 个 |
| 4 | TestList_OrderByRegistration | 按注册顺序（z, a, m），验证非字母序 | 返回顺序 = 注册顺序 |
| 5 | TestList_ReturnCopy | 修改返回切片的元素 | 第二次 List 不受影响 |
| 6 | TestList_MixedSourceTypes | 注册 CLI、builtin、MCP 三种来源 | 三种来源各 1 个 |
| 7 | TestList_AfterFailedRegister | Register 同名冲突失败 | List 不包含被拒工具 |
| 8 | TestList_DoesNotIncludeDeletedTool | delete 后 | List 不包含已删除工具 |
| 9 | TestList_DoesNotIncludeNilInterface | tools[name]=nil | List 不包含 nil 元素 |

## 实现状态

> 由 gen-code 填充。初始为空。gen-code 完成后标注实现文件和测试通过情况。
