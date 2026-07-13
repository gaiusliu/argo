# brig

## 概述

brig 是工具调用的权限门控模块，在 helm 执行工具前对每个 `ToolUse` 做裁决。采用分层裁决模型：Iron（红线）→ Cord（可配置规则）→ 用户审批记录 → Ask 兜底。裁决引擎和规则来源通过接口解耦。

## 核心概念

| 概念 | 说明 |
|------|------|
| `Gate` | 顶层门禁接口（Evaluate / Approve / Snapshot / Restore），消费者（voyage）只依赖这个接口 |
| `Warden` | 分层裁决引擎，实现 Gate。持有 Iron + Cord 两桶规则 + 用户审批记忆 |
| `RuleLoader` | 规则来源接口，`Load() []Rule` 在构造 Warden 时调用一次，Evaluate 时不再调用 |
| `BuiltinRuleLoader` | 内置规则实现：denyRules（Iron）+ safeCommands / dangerCommands / interpreterCommands / sensitiveFiles（Cord） |
| `Rule` | 规则条目：Tool / Pattern / Action / Type（RuleIron / RuleCord） |
| `RuleIron` / `RuleCord` | 规则类型枚举。Iron = 红线不可绕过，Cord = 可配置。零值无意义 |

## 行为合约

### 分层裁决

`Gate.Evaluate(tu)` 的行为保证：

1. **Iron 红线优先**——命中即返回 Deny，不受后续规则影响
2. **Cord 规则按序匹配**——首个命中的规则决定结果（Approve / Ask / Deny）
3. **用户审批记忆**——Cord 未命中时检查 `userApprovals`，命中返回 Approve
4. **兜底 Ask**——以上均未命中时返回 Ask

### 规则匹配

`Rule` 支持 6 种通配：

- 精确匹配、`*word*` 中间通配、`*.ext` 前导通配、`dir/*` 目录通配、`prefix*` 后缀通配、反向前缀（防止 `rm` 误匹配 `rmdir`）
- `web_fetch` 工具按域名粒度匹配（非精确匹配）

### 审批持久化

- `Snapshot()` 导出用户审批记录 → 由 voyage 持久化为 `approvals.json`
- `Restore()` 从快照恢复 → session 恢复时调用
- `Approve()` 追加动态审批条目

### RuleLoader 注入

- `NewWarden(workingDir, ...RuleLoader)` 构造时一次性加载所有规则
- 未传 loader 时 Warden 零规则（纯 Ask 兜底）
- 生产代码传 `&BuiltinRuleLoader{}` 获得完整内置规则
