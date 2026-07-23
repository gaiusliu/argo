# 010：DI 改造 — 消除全局状态 + 接口拆分

**日期**：2026-07-13
**状态**：✅ 已完成（闭环 → `docs/{brig,voyage}/design.md`）
**涉及模块**：sail、brig、voyage

## 目标

按 [handoff.md](../../handoff.md) 中第一阶段 DI 改造推荐顺序，完成前两项：消除 `http.DefaultClient` 硬编码、brig 接口按职责三层拆分。

## 范围

### 已完成

| # | 改造项 | 涉及文件 |
|---|--------|---------|
| 1 | `sail` 注入 `*http.Client` | `src/sail/sail.go`、`src/helm/phase_model.go` |
| 2 | `brig` 拆分：`Gate` + `Warden` + `RuleLoader` | `src/brig/brig.go`、`src/brig/deny.go`、`src/voyage/voyage.go` |
| 4 | `voyage.Session` 注入 Vault + Gate | `src/voyage/voyage.go`（SessionOption + WithVault/WithGate） |

### 跳过

| # | 改造项 | 原因 |
|---|--------|------|
| 3 | 移除 `knot.GetConfig()` 全局单例 | 需改动 server 层引入 agent 依赖，用户决定保持全局单例形式 |
| 5 | `deck.Registry` 实例化 | 收益不大，deck 保持全局单例 |

### 不含（后续迭代）

- `helm.PhaseRunnerModel` 注入 Provider + Registry + Crew（#6）→ 见 011
- `helm.PhaseRunnerModel` 注入 Provider + Registry + Crew（#6）
- `crew` 模块实现（#7）
- `AgentService` 抽象（#8）
- Asking reply channel（#9）

## 设计决策

| 决策 | 说明 |
|------|------|
| `RuleIron` / `RuleCord` | 规则类型枚举：Iron（红线，不可绕过）、Cord（可配置，绳令）。零值无意义，必须显式指定 |
| `RuleLoader.Load()` | 只在 `NewWarden` 构造时调用一次，`Evaluate` 时只读遍历内置规则 |
| `Warden` 零规则 | Warden 不内联任何规则，所有规则由 `...RuleLoader` 注入 |
| `Gate` 去掉 `IsCachedApproval` | 无外部调用者，死代码 |
| `SessionOption` 函数选项 | Vault/Gate 注入用 functional options，默认行为零改动 |

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-07-13 | #1 sail *http.Client 注入 完成 |
| 2026-07-13 | #2 brig Gate+Warden+RuleLoader 拆分 完成 |
| 2026-07-13 | #3 knot.GetConfig 改造 跳过，保持全局单例 |
| 2026-07-13 | #4 voyage.Session 注入 Vault+Gate 完成 |
| 2026-07-13 | #5 deck.Registry 跳过 |
| 2026-07-13 | 迭代开始 |
