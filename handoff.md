# Handoff — 全面代码审查与修复

## 背景

对 argo 全量代码执行了 max-effort 级别的 code-review（9 角度并行审查 + 5 验证代理 + 1 扫查代理），发现 15 个问题。已修复 10 个代码问题、记录 1 个架构决策（DEC-028）、更新 4 份设计文档、1 个性能优化延后、2 个经讨论确认不需要改。

## 已完成的代码改动

### 正确性修复

| 改动 | 文件 | 说明 |
|------|------|------|
| case 6 边界检查 | `brig/brig.go` | 新增 `isBoundary()` 辅助函数——match 后检查剩余文本首字符必须为边界（空/空格/tab/`/`/`\`），防止 `"rm"` 误匹配 `"rmdir"` |
| case 4 守卫加强 | `brig/brig.go` | `len(pattern) > 1` → `len(pattern) > 2`，防止 `"/*"` 匹配所有绝对路径 |

### 可维护性改进

| 改动 | 文件 | 说明 |
|------|------|------|
| ToolCall 有键字面量 | `brig/brig.go` | `ToolCall{tu.Name, ...}` → `ToolCall{ToolName: ..., RawParam: ..., WorkingDir: ...}`，防止字段重排静默错位 |
| getRawParam 解耦 adapter | `brig/brig.go` + `sail/adapter.go` | `flush()` 中将 JSON arguments 解析为扁平 map；`getRawParam` 删除嵌套 JSON 解析，brig 不再感知 sail 内部格式 |
| buildOpenAIBody 填充 Tools | `sail/adapter.go` + `knot/types.go` + `deck/builtin_tool.go` | Tool 接口新增 `ParametersSchema()`，10 个内置工具各定义 JSON Schema，adapter 中转换为 `openAITool` 数组 |
| fieldFromArgs 查表化 | `brig/brig.go` | switch-case → `toolParamKeys` map 查表，新增工具加一行映射即可 |
| sensitiveFiles 去重 | `brig/deny.go` | 22 条重复规则 → `sensitiveFilePatterns` + `init()` 循环生成，新增 pattern 改一处 |
| RuleSource 移除 | `brig/brig.go` + `brig/deny.go` | 删除 `RuleSource` 类型、`Static`/`Dynamic` 常量、`Rule.Source` 字段（动态审批已改用 `userApprovals` map） |
| 排序依赖注释 | `brig/brig.go` + `brig/deny.go` | `loadStaticRules` 和 `resolveStaticRules` 明确声明 deny 必须排在首位 |

### 关键代码变更快照

**Tool 接口**（`knot/types.go`）：
```go
type Tool interface {
    Name() string
    Description() string
    Source() ToolSource
    ParametersSchema() map[string]any  // 新增：JSON Schema，供 sail 构造 tools 字段
    Execute(ctx context.Context, params map[string]any) (ToolResult, error)
}
```

**Rule 结构**（`brig/brig.go`）——已移除 Source 字段：
```go
type Rule struct {
    Tool    string
    Pattern string
    Action  Verdict
}
```

**toolParamKeys 表**（`brig/brig.go`）：
```go
var toolParamKeys = map[string]string{
    "bash": ParamCmd, "write": ParamPath, "edit": ParamPath,
    "read": ParamPath, "grep": ParamPath, "web_fetch": ParamURL,
}
```

**isBoundary**（`brig/brig.go`）：
```go
func isBoundary(rest string) bool {
    return rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '/' || rest[0] == '\\'
}
```

## 已记录/更新的文档

| 文件 | 操作 |
|------|------|
| `docs/modules/brig/design.md` | 全面更新：Rule 去 Source、matchPattern 加 isBoundary、新增 toolParamKeys/sensitiveFilePatterns 节、精简冗余代码块（212→144 行） |
| `docs/modules/knot/design.md` | Tool 接口新增 ParametersSchema 方法说明 |
| `docs/modules/deck/design.md` | builtinTool 新增 paramsSchema 字段、内置工具表加参数列 |
| `docs/modules/sail/design.md` | 新增 buildOpenAIBody 概念条目、flush 更新为 JSON 解析 + 扁平 Parameters |
| `docs/decisions.md` | DEC-028：WorkingDir 暂放 Config，待 Session 模块建立后迁移 |

## 经讨论确认不需要改

| 原问题 | 原因 |
|--------|------|
| `cfg, _ := GetConfig()` 忽略错误 + sync.Once 缓存错误 | 执行流中 `sail.Chat` 先于 brig 调用 `GetConfig()` 并检查错误，brig 执行时 globalCfg 已保证非 nil |
| parseSSE ctx 取消时双发 error | 概率小，后发的可覆盖先发的 |
| TokenLimit 死代码 | 为后续模块准备的桩代码 |
| case 6 中 `filepath.Base` 存在的必要性 | 已确认：让文件名 pattern（如 `.env`）能匹配深层路径（如 `/a/b/.env.local`） |

## 延后事项

| 事项 | 优先级 | 说明 |
|------|--------|------|
| 静态规则按工具分桶（`map[string][]Rule`） | P2 | 当前 ~100 条规则扁平遍历，按工具分桶可降 90% 遍历量 |
| CLI 工具删除 | P2 | 用户计划用 bash 替代 CLI 工具，届时 `cli_tool.go`、`tool_registry.go`、`SourceCLI`、`CLIToolEntry`、`DeckConfig.CLI` 一并清理 |
| WorkingDir 从 Config 迁到 Session | P3 | DEC-028 已记录，待 Session 模块建立后执行 |

## 当前模块状态

| 模块 | 状态 |
|------|------|
| deck | ✅ |
| sail | ✅ |
| helm | ✅ |
| brig | ✅ |
| knot | ✅ |
| reef | 🚧 |
| vault | ❌ |
| crew | ❌ |
| cmd | 🚧 |
| session | ❌（新建后处理 WorkingDir 迁移 + Snapshot/Restore 实现） |

## 建议 skills

- 继续开发时参考 `docs/modules/*/design.md` 了解最新设计
- 技术决策记录在 `docs/decisions.md`，DEC-028 与本轮改动相关
- 如需代码生成规范，参考 CLAUDE.md 中的规则
