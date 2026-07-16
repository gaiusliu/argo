# brig

## 概述

brig 是 Argo 的权限审批门控模块。在 helm 执行工具前，brig 按规则判定 Allow/Ask/Deny，决定工具调用是直通执行、需用户确认、还是直接拒绝。brig 只依赖 `knot`，不依赖 helm/deck/sail。

裁决采用分层模型：Iron（红线，不可绕过）→ Cord（可配置规则）→ 用户审批记录 → Ask 兜底。规则来源通过 `RuleLoader` 接口解耦，裁决引擎 `Warden` 构造时一次性加载。

## 核心概念

| 概念 | 说明 |
|------|------|
| Verdict | 三态裁决：`Approve`（直通执行）/ `Ask`（征求用户确认）/ `Deny`（直接拒绝） |
| Gate | 门控接口：`Evaluate` / `Approve` / `Snapshot` / `Restore`，不包含 `IsCachedApproval`（已移除死代码） |
| Warden | Gate 的唯一实现——分层裁决引擎。构造时从 `...RuleLoader` 加载规则，按 `Rule.Type` 分桶为 iron / cord，Evaluate 时只读遍历 |
| Rule | 统一规则结构 `{Tool, Pattern, Action, Type}`，Type 为 `RuleIron`（红线=1）或 `RuleCord`（可配置=2） |
| RuleLoader | 规则来源接口 `Load() []Rule`，仅在 `NewWarden` 构造时调用一次，Evaluate 不再调用 |
| BuiltinRuleLoader | RuleLoader 的内置实现，加载 denyRules（Iron）+ safeCommands/dangerCommands/interpreterCommands/sensitiveFiles（Cord） |
| ToolCall | 工具调用指纹 `{ToolName, RawParam, WorkingDir}`——动态审批的精确匹配键 |
| scopeKey | 审批粒度转换：bash/write/edit 保持精确值，`web_fetch` 将完整 URL 转为域名 |
| getRawParam | 从 `ToolUse.Parameters` 中按工具名取对应字段值，由 `toolParamKeys` 表驱动 |
| matchPattern | 6 种通配符匹配（精确/中间/前导/目录/后缀/反向前缀+边界），首个命中即返回 |
| ApprovalEntry | 审批记录持久化结构，`Snapshot()` / `Restore()` 使用 |

## 行为合约

### Evaluate — 四层裁决

```
1. Iron 红线 → 命中即 Deny，不可绕过
2. Cord 可配置规则 → 命中 Approve/Ask/Deny 即返回
3. 用户审批记录 → ToolCall 指纹精确匹配则 Approve
4. 兜底 → Ask
```

每层命中即返回，不继续。Iron 层包含 `rm -rf /`、`mkfs`、内网地址访问、系统文件写入等。

Cord 层内按 `BuiltinRuleLoader.Load()` 的 append 顺序求值：safeCommands（Allow）→ dangerCommands（Ask）→ interpreterCommands（Ask）→ sensitiveFiles（Ask）。同一个 raw 不会同时命中 Allow 和 Ask——safeCommands 先于 dangerCommands，命中 Allow 即返回。

### Approve — 精确指纹匹配

用户确认后，`Approve(tu)` 以 `ToolCall` 指纹存入 `userApprovals` map。不做泛化——只有完全相同的（工具名 + 审批键 + 工作目录）再次出现时才自动 Allow。`web_fetch` 通过 `scopeKey` 将 URL 转为域名，相同域名下自动放行。

### 审批持久化

- `Snapshot()` 返回当前 `userApprovals` 快照
- `Restore()` 从快照恢复
- 审批记录通过 `voyage.SaveApprovals` 持久化到 `approvals.json`
- `Resume` 时自动 `loadApprovals` → `Gate.Restore` 恢复

### matchPattern — 6 种匹配方式

| # | 方式 | Pattern 示例 | 匹配行为 | 典型用途 |
|---|------|-------------|---------|---------|
| 1 | 精确匹配 | `rm -rf /` | `target == pattern` | deny 精确命令 |
| 2 | 中间通配 | `*credentials*` | target 任意位置包含 word | 敏感文件名 |
| 3 | 前导通配 | `*.pem` | target 以该后缀结尾 | 凭证文件后缀 |
| 4 | 目录通配 | `dir/*` | target 在目录下（pattern 长度 > 2） | 同目录批量 Allow |
| 5 | 后缀通配 | `.env*` | target（或 basename）以 prefix 开头 | 敏感文件前缀 |
| 6 | 反向前缀+边界 | `rm` | pattern 后必须为边界（空/空格/tab/路径分隔符） | 防止 `"rm"` 误匹配 `"rmdir"` |

### 会话隔离

每个 session 持有独立 Gate 实例，动态审批不跨 session 共享。`New` / `Resume` 时通过 `NewWarden(workingDir, &brig.BuiltinRuleLoader{})` 创建，测试可通过 `WithGate` 注入 mock。

### 依赖边界

brig 只依赖 `knot`，不依赖 helm/deck/sail。`Evaluate` 返回 `(Verdict, reason)`。
