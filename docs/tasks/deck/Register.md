---
module: deck
function: Register
layer: 0
status: code_done
depends_on:
  internal: []
  external:
    - Tool（deck 包接口，chart 已定义规格）
    - unexported tools map（调度器已生成）
---

# deck::Register

## 职责

将 Tool 实例注册到进程唯一的 unexported map 中。同名冲突时返回 error（builtin > 外部，CLI 和 MCP 同名冲突报错）。

## 签名

```go
func Register(t Tool) error
```

## 依赖详情

### 已就绪（可 import 使用）
- `Tool` — 接口，chart 已定义规格
- `tools` map — 包级 unexported map，调度器已生成

### 待实现（需等待）
无

## 测试场景

> 由 gen-test 填充，初始为空。gen-test 完成后在此列出测试方法名和覆盖的场景。

| 测试方法 | 覆盖场景 |
|---|---|
| TestRegister_Success | 正常注册: Register 后 tools map 中存在该 name 且指向同一实例 |
| TestRegister_DuplicateExternal_ReturnsError | 两个外部工具同名重复注册返回 error, 原工具不被覆盖 |
| TestRegister_NilTool_ReturnsError | Register(nil) 返回 error, 不 panic |
| TestRegister_BuiltinPriority | builtin 已注册时, 同名外部工具注册返回 error, builtin 不被覆盖 |
| TestRegister_MultipleDistinctNames | 多个不同名工具同时注册全部成功 |
| TestRegister_MCP_MCP_Conflict | MCP vs MCP 同名，第二次注册返回 error |
| TestRegister_CLI_MCP_Conflict | CLI vs MCP 平权，不分先后，后者报 error |
| TestRegister_TypedNil_ReturnsError | Register(typed nil) 返回 error，不 panic |
| TestRegister_EmptyName | 空 name 注册返回 error，map 中不产生空 name 条目 |

## 实现状态

- 实现文件: deck/registry_stub.go → deck/registry.go（原 stub 文件已被替换为正式实现）
- 测试通过: 9/9 全部通过
- 测试结果: go test -v -run "Register" ./deck/ — 2026-06-12 全部 PASS
- 状态: code_done
