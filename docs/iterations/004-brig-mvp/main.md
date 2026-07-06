# 004-brig-mvp

**日期**：2026-07-05 ~ 2026-07-06
**状态**：🔄 已重写取代
**涉及模块**：brig（新）、helm、knot

> ⚠️ **本文档描述的是 brig 的初版设计（v1）**，该版本已于 2026-07-06 被推倒重写。
>
> **当前设计见 [docs/modules/brig/design.md](../../modules/brig/design.md)**。
>
> 初版与重写版的核心差异：
> - **动态审批**：初版用 `Append` 泛化 pattern 追加到 rules 列表 → 重写版用 `Approve` 精确 ToolCall 指纹存入 `userApprovals` map
> - **求值策略**：初版用 `resolveVerdict` "最后命中者胜" → 重写版用 `resolveStaticRules` 5 层固定顺序首命中即返回
> - **Deny 处理**：初版用 `checkDeny` 独立第一层 → 重写版并入同一次遍历
> - **参数提取**：初版用 `extractPatterns` 提炼碎片 → 重写版用 `getRawParam` 零损失提取
> - **回调命名**：初版用 `ConfirmFunc` → 重写版用 `AskUser`
>
> 以下为初版设计文档原文，保留作为历史参考。

---

## 目标

实现工具调用的权限审批门控——`helm.RunLoop` 收到 LLM 工具调用后，先经过 `brig.Engine.Evaluate()` 判定 Allow/Ask/Deny，再决定是直接执行、走 `AskUser` 用户确认、还是直接拒绝。

MVP 阶段规则引擎做内存级会话状态（不持久化），但设计上预留持久化接口。不引入用户自定义规则配置。

## 核心概念

### Verdict 三态

| 态 | 含义 | 后续动作 |
|----|------|---------|
| `Allow` | 免审批，直接执行 | RunLoop 调 `tool.Execute()` |
| `Ask` | 需用户确认 | RunLoop 调 `AskUser` → true 则执行并追加动态规则，false 则跳过并返回 "user rejected" |
| `Deny` | 直接拒绝 | RunLoop 跳过执行，返回拒绝原因给 LLM |

### Rule — 规则即记忆

统一数据结构，既是求值依据，也是"已审批"的记忆载体：

```go
type Rule struct {
    Tool    string     // 工具名（bash / write / edit / web_fetch / read / grep）
    Pattern string     // 匹配模式（目录、域名、命令、文件路径）
    Action  Verdict    // Allow / Ask / Deny
    Source  RuleSource // Static（代码内置，不可变）/ Dynamic（用户确认后追加，可持久化）
}
```

### Engine 结构

```
Engine
├─ rules []Rule   → 规则列表（静态 + 动态），求值时遍历
│
│  静态规则（NewEngine() 时加载，所有 session 共享同一份副本）:
│  ├─ deny 库        → {bash, "rm -rf /", Deny, Static}
│  ├─ 安全命令白名单  → {bash, "ls", Allow, Static}
│  ├─ 危险命令表      → {bash, "curl", Ask, Static}
│  ├─ 脚本/解释器表   → {bash, "python", Ask, Static}
│  └─ 敏感文件表      → {read, ".env", Ask, Static}
│
│  动态规则（用户确认后追加，Pre session 私有）:
│  └─ {write, "src/*", Allow, Dynamic}   ← 用户已确认 write src 目录
│
└─ mu  sync.Mutex  → 保护动态规则的并发追加
```

### 各 session 拥有私有 engine 实例

启动时创建 engine → 注入 RunLoop。用户确认某条工具调用后，追加动态 Allow 规则到 engine。后续同一 tool + pattern 直接 Allow。动态规则仅属于当前 session，不跨 session 共享。

### AskUser — 用户交互回调

```go
// knot/types.go
type AskUser func(tu ToolUse, reason string) (bool, error)
```

由调用方（CLI / Gateway）注入，helm 不感知实现细节：

| 场景 | ask 实现 |
|------|-------------|
| CLI | 打印到 stderr → 读 stdin y/n |
| Gateway | 发 SSE permission.asked → 阻塞等用户 reply |
| 测试 | `func(_, _) (bool, error) { return true, nil }` |

### RunLoop 签名

```go
func RunLoop(ctx context.Context, state *TurnState, ask knot.AskUser, eng *brig.Engine) (<-chan knot.Event, error)
```

`ask` 和 `eng` 为 session 级依赖注入，`TurnState` 只保留会话流转状态（Messages / TurnCount / ModelName）。

## 整体流程

```
helm.RunLoop
  → LLM 返回 tool calls
  → for each tu in toolUses:
      │
      ├─ eng.Evaluate(tu.Name, tu.Params)
      │   │
      │   ├─ 1. 遍历 rules，按 Pattern 匹配 Tool
      │   │     匹配规则：精确匹配 > 前缀匹配 > 通配符 *
      │   ├─ 2. 取最后一条匹配规则的 Action
      │   │     ├─ Deny  → 返回 Deny + 原因
      │   │     ├─ Allow → 返回 Allow
      │   │     └─ Ask   → 返回 Ask + 原因
      │   └─ 3. 无匹配规则 → 默认 Ask
      │
      ├─ Allow → tool.Execute()
      ├─ Ask   → ask(tu, reason)
      │           ├─ true  → eng.Approve(tu)
      │           │          → tool.Execute()
      │           └─ false → Result = StatusError + "user rejected"
      └─ Deny  → Result = StatusError + 拒绝原因
```

## Evaluate 内部详细流程

```
eng.Evaluate("bash", {cmd: "git push && ls"})
│
├─ 提取 pattern: 对 bash 用的是基础命令列表 ["git push", "ls"]
├─ 对每个子 pattern 遍历 rules:
│
│   "git push":
│   ├─ {bash, "rm -rf /", Deny}      → 不匹配
│   ├─ {bash, "curl", Ask}           → 不匹配
│   ├─ {bash, "git push", Ask}       → 匹配！记下 Ask
│   │
│   "ls":
│   ├─ {bash, "ls", Allow}           → 匹配！记下 Allow
│
├─ 汇总: 有 Ask → 返回 Ask（最高优先级）
└─ 返回 Ask + "git push 为危险命令"
```

## 规则清单

### 1. 静态 deny 库 — NewEngine() 时加载

命中即 Deny，不可绕过：

| 维度 | Pattern 示例 | 说明 |
|------|-------------|------|
| 命令 | `rm -rf /`、`mkfs`、`dd if=` | 不可逆的破坏性命令 |
| 域名 | `127.0.0.1`、`localhost`、`0.0.0.0` | SSRF 防护 |
| 文件 | `/etc/shadow`、`C:\Windows\System32` | 系统关键文件 |

> MVP 阶段 pattern 做精确匹配。后续支持 glob / 前缀匹配。

### 2. BashRule 静态规则 — 安全/危险/解释器三类

三类命令按风险分级：纯读取/查询类 Allow，可能修改状态类 Ask，无法静态分析的脚本解释器 Ask。完整列表见 `brig/deny.go`。

### 3. FileMutationRule — 文件写入/编辑

处理 `write`、`edit` 工具。提取文件所在目录作为 Pattern：

```
Evaluate("write", {filePath: "src/internal/config.go"})
  → pattern = "src/internal"
  → 遍历 rules，查找 {Tool: "write", Pattern: 匹配 "src/internal"} 的 allow 规则
  → 未命中 → Ask
```

用户确认后：

```
eng.Approve(tu)  // 以精确 ToolCall 指纹存入 userApprovals map
```

### 4. SensitiveReadRule — 敏感文件读取

处理 `read`、`grep` 工具。静态规则——敏感文件模式匹配：

🟡 **敏感文件模式**（静态 Ask 规则）

匹配 `.env`、`*.pem`、`*secret*`、`*token*`、`id_rsa*` 等凭证/密钥类路径。完整列表见 `brig/deny.go`。`read` 和 `grep` 命中敏感路径 → Ask。

### 5. WebFetchRule — 域名审批

处理 `web_fetch` 工具。提取 URL 域名作为 Pattern：

```
Evaluate("web_fetch", {url: "https://api.github.com/repos/..."})
  → pattern = "api.github.com"
  → 遍历 rules
  → 未命中 → Ask
```

用户确认后追加：`{Tool: "web_fetch", Pattern: "api.github.com", Action: Allow, Source: Dynamic}`。

### 6. 工具首次使用

以下工具在首次调用时（rules 中无对应 Dynamic allow 规则）默认 Ask：

`bash`、`write`、`edit`、`web_fetch`

`read`、`grep` 只在命中敏感文件模式时 Ask。

`list`、`lookup`、`glob`、`web_search` 默认 Allow（静态规则直接加入 allow 列表）。

## 动态规则的持久化

当前 MVP 不持久化，但已预留接口：

```go
func (e *Engine) Snapshot() []Rule   // 返回 Source==Dynamic 的规则
func (e *Engine) Restore([]Rule)      // 反序列化后追加回 rules
```

未来 session 切换时，vault 调 `Snapshot()` → 写文件；session 恢复时读文件 → `Restore()` → 完成。

## 工具→规则覆盖矩阵

| 工具 | Deny | 安全命令 | 危险命令 | 解释器 | FileMutation | SensitiveRead | WebFetch | 动态追加 |
|------|------|---------|---------|--------|-------------|--------------|----------|---------|
| `bash` | ✅ | ✅ | ✅ | ✅ | - | - | - | ✅ |
| `write` | - | - | - | - | ✅ | - | - | ✅ |
| `edit` | - | - | - | - | ✅ | - | - | ✅ |
| `read` | - | - | - | - | - | ✅ | - | - |
| `grep` | - | - | - | - | - | ✅ | - | - |
| `web_fetch` | ✅ | - | - | - | - | - | ✅ | ✅ |
| `list`/`lookup`/`glob`/`web_search` | - | - | - | - | - | - | - | - |

## 公共设计约束

- `brig` 包只依赖 `knot.ToolUse`，不依赖 helm/deck/sail
- `Engine` 线程安全（`mu` 保护动态规则），静态规则只读无锁
- 动态规则仅做精确匹配（MVP 不做 glob），静态规则支持前缀匹配
- 每条规则匹配按 "最后一条命中者胜" 原则，Deny > Ask > Allow 优先级
- `AskUser` 定义在 `knot`，实现在调用方（`cmd/run` / Gateway），brig 不感知用户交互
- 每个函数不超过 50 行
- 动态规则可持久化（`Snapshot` / `Restore`），MVP 不执行，但接口不阻塞设计

## 文件结构

```
src/
  brig/
    brig.go      — Verdict / Rule / RuleSource / Engine{ rules, mu } / Evaluate / Approve / Snapshot / Restore
    deny.go      — 静态 deny 规则集 + 安全命令 + 危险命令 + 解释器 + 敏感文件
    bash.go      — bash 命令拆解 + 子命令提取
    webfetch.go  — URL 域名提取

  knot/types.go  — AskUser 类型定义

  helm/
    loop.go      — RunLoop(ctx, state, ask, eng) 签名变更，集成 brig
    types.go     — TurnState 去掉 brig 相关字段，只保留会话流转状态

  cmd/run/
    main.go      — 创建 engine + ask + TurnState，调 RunLoop
```

## 整体开发状态

| 功能 | 状态 | 文件 |
|------|------|------|
| Verdict + Rule + Engine + Evaluate | ✅ | brig.go |
| 静态规则集（deny/safe/danger/interpreter/sensitive） | ✅ | deny.go |
| Bash 命令拆解 | ✅ | bash.go |
| WebFetch URL 域名提取 | ✅ | webfetch.go |
| AskUser 类型定义 | ✅ | knot/types.go |
| RunLoop 签名变更 + brig 集成 | ✅ | helm/loop.go |
| CLI ask 实现 + main.go | ⏳ | cmd/run/main.go |
