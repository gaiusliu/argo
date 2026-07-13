# Handoff — 2026-07-13（Tale 引入 + Voyage 合并完成）

## 上次会话成果（2026-07-12）

参见旧版 handoff 或 commit `80c2538`。核心产出：架构全景评审、kimi 提案评审、最终命名决策、改造计划。

## 上次会话成果（2026-07-13 前半）

| # | 改造项 | 涉及文件 |
|---|--------|---------|
| 1 | `sail` 注入 `*http.Client` | `src/sail/sail.go` |
| 2 | `brig` 拆分：`Gate` + `Warden` + `RuleLoader` | `src/brig/brig.go`、`src/brig/deny.go`、`src/voyage/voyage.go` |
| 4 | `voyage.Session` 注入 Vault + Gate（Option 模式） | `src/voyage/voyage.go` |
| 6 | `sail.Provider` 接口重构 + `PhaseRunnerModel` 注入 | `src/sail/provider.go`、`src/sail/sail.go`、`src/knot/types.go`、`src/helm/phase_model.go` |

跳过：#3 移除 `knot.GetConfig()`（保留全局单例）、#5 `deck.Registry` 实例化（收益不大）。

## 本次会话成果（2026-07-13 后半）

### Tale 引入 + 架构精简

| 变更 | 说明 |
|------|------|
| `src/tale/tale.go` | 新建。Tale 封装 Messages + Saved 游标：`AppendUser/Assistant/Tool` + `BuildRequest` + `Unsaved/MarkSaved` |
| Voyage 去接口 | `Voyage interface` 删除，`*Voyage` 直接使用（内部变化性由 Vault/Gate 覆盖） |
| Session → Voyage | `Session` 改名 `Voyage`，`SessionInfo` → `Info`，包级函数改短名（`New`/`Resume`/`List`/`Delete`/`RenameByID`） |
| HelmState 删除 | `src/helm/helm_state.go` 删除。Phase/Model/ToolUses/Saved 并入 Voyage，Phase 常量移入 voyage 包 |
| Tale 归 Voyage | Tale 由 Voyage 构造时从 Vault 加载创建，`AppendMessages` 内化 `vault.MessagesToRecords` + `MarkSaved` |
| SaveState/LoadState | Voyage 自管理控制流持久化，游标自动同步 |
| PhaseRunner 返回值 | `Run(ctx, v, emit) bool`：`true`=goroutine 退出，`false`=继续循环 |
| checkpoint 单参数 | `checkpoint(v)`，不再传 `st *HelmState` |

### 文档更新

- `docs/decisions.md` — DEC-038：Voyage 单一聚合体
- `docs/voyage/design.md` — 重写
- `docs/architecture.md` — Voyage/helm 描述、数据流、代码目录同步

## 新增设计决策

| 决策 | 说明 |
|------|------|
| DEC-038 | Voyage 单一聚合体——去接口 + Session 改名 + HelmState 合并 + Tale 归属。PhaseRunner 统一签名 |
| `RuleIron` / `RuleCord` | 规则类型枚举（Iron=红线、Cord=可配置），零值无意义 |
| `RuleLoader.Load()` | 只在 `NewWarden` 构造时调用一次，`Evaluate` 只读 |
| `Warden` 零规则 | 不内联任何规则，所有规则由 `...RuleLoader` 注入 |
| `Provider` 接口 | `Name()` / `Model()` / `Chat(ctx, messages, tools)` |
| `ProviderConfig.API` 字段 | 显式声明协议（`"openai-completions"` 或空默认） |
| `SessionOption` | Vault/Gate 注入用 functional options |
| Provider 在 `Run()` 中创建 | `PhaseRunnerModel.Run()` 内部调 `sail.NewProvider(st.Model)` |

## 改造计划 — 第一阶段剩余

| # | 改造项 | 涉及文件 |
|---|--------|---------|
| 7 | `crew` 模块实现 | 新建 `src/crew/` |
| 8 | `AgentService` 抽象 | 新建，`src/server/prompt.go` handler 瘦身 |
| 9 | Asking reply channel | `src/voyage/voyage.go`、`src/server/` |

## 已知残留问题

| 问题 | 位置 | 说明 |
|------|------|------|
| 死代码 `sail.Chat()` 包级函数 | `src/sail/sail.go` | 已由用户清理 |
| 死代码 `sail.Window()` | `src/sail/sail.go:218` | 0 调用者，依赖 `knot.LookupModel()` |
| `http.DefaultClient` 残留 | `src/sail/adapter.go:315`、`src/deck/builtin_tool.go:611,674` | 不走 ProviderOpenAI 路径 |
| `NewProviderOpenAI` 冗余参数 | `src/sail/sail.go` | messages/tools 参数已无意义 |
| 旧 `Sail` 接口 | `src/sail/sail.go:16` | 0 外部调用者，Provider 接口已替代 |
| `knot.LookupModel()` | `src/knot/config.go:150` | 仅被 `sail.Window()` 调用，随 Window 一起删 |

## 正在进行：interrupt 能力

### 目标

用户可随时终止正在运行的 agent loop（`helm.Run()` goroutine）。

### 方案（已讨论，待实施）

```
Server
  + interrupts map[string]context.CancelFunc    ← sessionID → cancel
  + POST /session/interrupt {sessionID}          ← 新端点

handlePrompt
  ctx, cancel := context.WithCancel(context.Background())  ← 不绑 request 生命周期
  s.interrupts[sessionID] = cancel
  defer delete(s.interrupts, sessionID) + cancel
  events := helm.Run(ctx, v)                     ← 传可取消 ctx

helm.Run
  每轮循环前检查 ctx.Done() → checkpoint + SaveState + return

PhaseRunnerModel.Run
  事件流结束后检查 ctx.Done() → 跳过 AppendAssistant（LLM 被中断时不写入不完整回复）
```

### 中断时点

| 时点 | 行为 |
|------|------|
| Model 阶段（LLM 流式中） | ctx 取消 → HTTP body 关闭 → 事件流终止 → 检测 ctx.Done() → 不追加 assistant → exit |
| ExecTools 阶段 | tool.Execute(ctx) 收到取消 → 返回 error → run.go 检测 ctx.Done() → exit |
| Asking 阶段 | goroutine 已退出，无需中断 |

### 未决问题

- 中断后是否保留部分文本回复（当前方案：不保留，ctx.Done() 后跳过 AppendAssistant）
- interrupt 端点是否需要权限校验（当前无）
- 是否需要 CLI 快捷键（Ctrl+C）触发 interrupt

## 未来可做

| 项 | 触发条件 |
|----|----------|
| 事件总线 | 多客户端同时操作同一 session |
| `helm.Machine` | 当 `newRunner` 需要超过 3 个共享参数时 |

## 建议技能

- `two-axis-review`：全部改造完成后做代码审查
- `grow-doc`：后续迭代生成迭代文档 + 模块 design.md
