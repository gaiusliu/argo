# brig

## 概述

brig 是 Argo 的权限审批门控模块。在 helm.RunLoop 执行工具前，brig 按规则判定 Allow/Ask/Deny，决定工具调用是直通执行、需用户确认、还是直接拒绝。brig 不依赖 helm/deck/sail，只依赖 `knot.ToolUse`。

## 核心概念

| 概念 | 说明 |
|------|------|
| Verdict | 三态裁决：Allow（直通执行）/ Ask（调 ConfirmFunc 征求用户）/ Deny（直接拒绝） |
| Rule | 统一结构 `{Tool, Pattern, Action, Source}`，既是求值依据也是"已审批"记忆 |
| Engine | 规则引擎，持有 rules 列表 + 互斥锁，每个 session 一个独立实例 |
| Pattern | 从工具参数中提取的匹配键——bash 子命令、文件父目录、URL 域名 |
| Static / Dynamic | 规则来源——Static 为代码内置（deny 库 + 命令分类表），Dynamic 为用户确认后追加 |
| ConfirmFunc | 用户确认回调，定义在 knot 包，由调用方（CLI / Gateway）注入，brig 不感知实现 |

## 流程

### Evaluate 判定

```
helm 收到 ToolUse
  → eng.Evaluate(tu)
      │
      ├─ 按工具名提取 pattern
      │   ├─ bash        → 拆子命令（&& / || / ; / |），取前 2 词
      │   ├─ write/edit  → filepath.Dir(路径)
      │   ├─ read/grep   → 文件路径本身（敏感文件匹配）
      │   ├─ web_fetch   → url.Parse().Host
      │   └─ list/lookup/glob/web_search → 无 pattern，直接 Allow
      │
      ├─ 遍历 rules，匹配 Tool + Pattern（精确 / 目录通配 /* / 前缀通配 *）
      ├─ "最后命中者胜"：同一 pattern 只保留最后匹配的规则
      └─ Deny > Ask > Allow 优先级汇总裁决
          ├─ 任意 pattern 命中 Deny → 立即返回 Deny
          ├─ 有 pattern 命中 Ask   → Ask
          ├─ 全是 Allow            → Allow
          └─ 无匹配                → 默认 Ask
```

### RunLoop 集成

```
helm.RunLoop
  → deck.Lookup(工具名)
  → eng.Evaluate(tu)           ← brig 门控
      ├─ Allow → tool.Execute()
      ├─ Ask   → confirm(tu, reason)
      │           ├─ err  → StatusError "confirm failed"
      │           ├─ !ok  → StatusError "user rejected"
      │           └─ ok   → eng.Append(Allow, Dynamic) → tool.Execute()
      └─ Deny  → StatusError（拒绝原因）
  → 结果注入 state.Messages → 下一轮 LLM
```

## 行为合约

### 规则引擎

- 静态规则在 NewEngine() 时加载，所有 session 共享同一份副本
- 动态规则由 mu 保护并发追加，仅属于当前 session
- 规则匹配：精确匹配 → 目录通配（`/*`）→ 前缀通配（`*`），MVP 不做完整 glob
- "最后命中者胜"：遍历 rules，后匹配的规则覆盖先匹配的
- Deny 无条件胜出——即使后续规则为 Allow，Deny 依然生效

### 会话隔离

- 每个 session 持有独立 Engine 实例，动态规则不跨 session 共享
- Snapshot() / Restore() 预留持久化接口，MVP 不调用

### brig 边界

- brig 只依赖 knot，不依赖 helm/deck/sail
- Evaluate 返回 (Verdict, reason, pattern)，pattern 供 RunLoop 调 Append
- ConfirmFunc 定义在 knot，实现在调用方——brig 只判定，不参与用户交互
- 默认安全工具（list/lookup/glob/web_search）直接 Allow，不经过规则匹配
