# Handoff — 2026-07-12（架构评审 + 改造规划）

## 本次会话成果

### 1. 架构全景评审

通读了 argo 全部 46 个 Go 文件，分析优劣势，结论：状态机 + 接口分层骨架正确，问题集中在全局状态、硬编码耦合、缺少 token 管理、零测试覆盖。

### 2. kimi 提案评审

逐一评审了 [dependency-injection-plan.md](docs/dependency-injection-plan.md) 和 [architecture-evolution.md](docs/architecture-evolution.md)，去粗取精。详见两份文档中的更新。

### 3. 最终命名决策

| 名称 | 层次 | 说明 |
|------|------|------|
| `brig.Gate` | 顶层接口 | 门禁（替代 `Guard`） |
| `brig.Warden` | 裁决引擎 | 典狱长（替代 `FileGuard`），分层评估 |
| `brig.RuleLoader` | 规则来源接口 | `BuiltinRuleLoader` / `FileRuleLoader` / `StaticRuleLoader` |
| `Tale` | 对话历史封装 | 替代 `HelmState` 中的 `Messages` + `Saved` |
| `crew.Loader` | skill 加载接口 | 扫描 SKILL.md，渐进式披露 |

### 4. 关键设计决策

- **voyage.Session 注入**：functional options 模式，不用构造器注入。生产代码零改动。
- **brig 分层裁决**：mandates（红线）→ rules（RuleLoader）→ Ask，不摊平排序。
- **`...RuleLoader` 变参**：不用 Option，因为只有规则来源这一个可变维度。
- **Asking reply channel**：goroutine 阻塞等回复，不用事件总线。消除断连重建和磁盘序列化风险。
- **crew 设计**：参照 pi / kilocode / opencode 三项目的渐进式披露模式。

## 改造计划

### 第一阶段：DI 改造（按推荐顺序实施）

| # | 改造项 | 涉及文件 |
|---|--------|---------|
| 1 | `sail` 注入 `*http.Client` | `src/sail/sail.go` |
| 2 | `brig` 拆分：`Gate` + `Warden` + `RuleLoader` | `src/brig/brig.go`、`src/brig/deny.go` |
| 3 | 移除 `knot.GetConfig()` 全局单例 | `src/knot/config.go` 及所有调用点 |
| 4 | `voyage.Session` 注入 Vault + Gate（Option 模式） | `src/voyage/voyage.go` |
| 5 | `deck.Registry` 实例化 | `src/deck/tool.go` |
| 6 | `helm.PhaseRunnerModel` 注入 Provider + Registry + Crew | `src/helm/phase_model.go` |
| 7 | `crew` 模块实现 | 新建 `src/crew/` |
| 8 | `AgentService` 抽象 | 新建，`src/server/prompt.go` handler 瘦身 |
| 9 | Asking reply channel | `src/voyage/voyage.go`、`src/server/` |

### 第二阶段：领域模型（需 compact / token 计数触发）

| # | 改造项 |
|---|--------|
| 10 | `Tale` 封装 `HelmState` 中的 `Messages` + `Saved` |
| 11 | 删除 `src/reef/`（compact 逻辑归 Tale） |
| 12 | `ToolUse` 拆分为 `ToolCall` + `ToolExecution` |

### 不做（定案）

- `RunnerFactory` struct：一个 `newRunner()` 函数够用
- `RuleLoader` 用 Option 模式：`...RuleLoader` 变参够用
- DDD 四层架构：规模不匹配

### 未来可做

| 项 | 触发条件 |
|----|----------|
| 事件总线 | 多客户端同时操作同一 session（扇出 + 应答路由） |

## 参考项目

skill 管理设计已参照 pi、kilocode、opencode 三个本地项目，核心模式一致：扫描 SKILL.md → 渐进式披露到 system prompt → 模型用已有工具加载完整内容。

## 建议技能

- `grow-doc`：将改造计划生成 010 迭代文档（`docs/iterations/010-di-refactor/main.md`）
- `two-axis-review`：改造完成后做代码审查
- `codegen-exec-flow`：按执行流顺序逐步生成代码
