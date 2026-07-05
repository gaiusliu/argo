# Handoff — brig 代码审核与修复

## 背景

对 004-brig-mvp 做了全面的 code-review，发现 15 个 bug。已修复大部分核心问题，但 `resolveVerdict` 仍存在设计缺陷待进一步讨论。

## 已完成的修复（本会话）

### 正确性修复

| 修复 | 涉及文件 | 说明 |
|------|---------|------|
| `matchPattern` 重写 | `brig/brig.go` | 从 3 条路径扩展为 6 条（精确/中间通配/前导通配/目录通配/后缀通配/反向前缀）。配合 `filepath.Base` 处理绝对路径 |
| `extractPatterns` → `getRawParam` | `brig/brig.go` | 匹配时不再提炼碎片，直接取原始参数。零信息损失 |
| `checkDeny` 新增 | `brig/brig.go` | Deny 规则用 raw 做第一层独立检查，不被 Ask/Allow 的"最后命中"覆盖 |
| `GeneralizePattern` 新增 | `brig/brig.go` | 用户确认后概括 raw → 可复用 pattern，仅在 Append 时用 |
| `Evaluate` 重写 | `brig/brig.go` | 流程：getRawParam → checkDeny → resolveVerdict |
| `loop.go` Delta 事件收集 | `helm/loop.go` | 改为从 `EventToolUseDelta` 收集 ToolUse（含 Parameters），不再依赖 Start 事件 |

### sail adapter 流式修复

| 修复 | 涉及文件 | 说明 |
|------|---------|------|
| `streamState` 重构 | `sail/adapter.go` | 三个 map 合并为 `map[int]toolCallState`，包含 id/name/args |
| handleChunk | `sail/adapter.go` | 不再 streaming 期间发 Delta，改为静默累积 `function.arguments` 片段 |
| flush 发完整 Delta | `sail/adapter.go` | `[DONE]` 触发 flush → 累积完成的完整 JSON → 发 `EventToolUseDelta` |

### 基础设施

| 改动 | 涉及文件 | 说明 |
|------|---------|------|
| 参数键常量 | `knot/types.go` | 13 个 `ParamXxx` 常量，deck/brig/sail 共用 |
| 全仓字符串字面量替换 | `deck/*.go`, `brig/*.go`, `sail/*.go` | `params["cmd"]` → `params[knot.ParamCmd]` 等 |

## ⚠️ 遗留问题：resolveVerdict 的设计缺陷

### 问题描述

当前 `resolveVerdict` 采用"最后命中者胜"：

```go
func resolveVerdict(rules []Rule, tool, raw string) (Verdict, string) {
    var last *Rule
    for i := range rules {
        if matchPattern(raw, r.Pattern) { last = &rules[i] }
    }
    // ...
}
```

同一 raw 可能命中多条规则，且它们可能 verdict 不一致。例如：

```
dangerCommands: {bash, "rm", Ask}
动态规则:       {bash, "rm -rf*", Allow}   ← 用户之前确认过
```

raw = `"rm -rf /tmp/test"` 同时命中两条。"最后命中"依赖隐式的规则顺序——动态规则排在末尾，Allow 覆盖掉 Ask。

**但这不可靠**：如果未来动态规则被插入到中间，或者两条冲突的静态规则都命中（如 `safeCommands` 的 Allow 和 `dangerCommands` 的 Ask），`resolveVerdict` 的行为完全取决于遍历顺序，没有语义保证。

### 已完成的调研

查阅了三个参考项目的做法：

- **opencode/kilocode**：`findLast`（最后命中者胜）+ 分层检查。Deny 在第一层独立检查（不可覆盖），Ask/Allow 在第二层走 `findLast`。没有 specificity 算法。正确性靠规则顺序保证：agent rule → user saved rule，后追加的覆盖之前的。
- **Claude Code (pi)**：没有规则引擎，危险命令检测用硬编码正则。不存在 rule conflict 问题。

argo 当前的 `checkDeny` + `resolveVerdict` 分层结构已经和 opencode 一致，但 `resolveVerdict` 内部的"靠顺序"没有文档化、没有保证。

### 需要决策的方向

两个候选方案：

1. **Append 时去重**：追加动态 Allow 规则前，扫描已有规则，删除被新 pattern 覆盖的旧 Ask 规则。保证 resolveVerdict 时同一 raw 至多命中一条非 Deny 规则。逻辑简单，改动小。

2. **规则加 Priority 字段**：给每条规则一个显式优先级（Deny=100, Ask=50, Allow=0），动态规则的 Priority 高于静态。resolveVerdict 取最高优先级的命中。语义明确，不依赖顺序。

### 建议下一步

先讨论选方案一还是方案二，然后实现，并补充 resolveVerdict 的单元测试覆盖冲突场景。

## 已记录的文档

| 文件 | 操作 |
|------|------|
| `docs/iterations/005-brig-fix/main.md` | 新建——code-review 15 个 bug 的四根因分析 |
| `docs/insights.md` | 追加——DeepSeek SSE 流式 tool call 格式实测发现（逐字增量、首块结构、多 tool call 同 chunk） |

## 后续工作

### P0 — resolveVerdict 修复

选定方案 → 实现 → 单元测试。

### P1 — 独立修复

| 任务 | 涉及文件 | 说明 |
|------|---------|------|
| SSRF deny 规则扩展 | `brig/deny.go` | 加 IPv6 `[::1]`、RFC 1918 私有段 `10.*`/`172.16-31.*`/`192.168.*` |
| `go fmt` 归类修正 | `brig/deny.go` | 从 safeCommands 移到 dangerCommands（go fmt 会修改文件） |
| Restore 去重 | `brig/brig.go` | 追加前检查已有规则，防止动态规则膨胀 |
| sensitiveFiles + denyRules 去重 | `brig/deny.go` | 提取公共 pattern 列表，read/grep 和 write/edit 分别循环生成 |

### P2 — 启用模板

| 任务 | 说明 |
|------|------|
| `cmd/run/main.go` | CLI 入口创建，集成 brig Engine + ConfirmFunc |
| 端到端测试 | 实际调 DeepSeek API 验证完整链路（sail → loop → brig → deck） |

## 当前进度总览

| 模块 | 状态 |
|------|------|
| deck | ✅ |
| sail | ✅（adapter 流式累积已修） |
| helm | ✅（Delta 事件收集已修） |
| brig | 🚧（核心门控已修，resolveVerdict 待修） |
| reef | 🚧 |
| vault | ❌ |
| crew | ❌ |
| cmd | 🚧 |

## 建议 skills

- 直接继续对话即可，无需特定 skill
- 如需代码生成规范，参考 CLAUDE.md 中的"写代码之前"和"写代码时"规则
