# Dependency Injection Refactoring Plan

> 本文档记录 Argo 项目已达成一致的依赖注入改造范围。
> 不包含 Server 层 `AgentService` 抽象（该方案暂不实施）。

## 背景

当前实现中存在多处包级全局状态或内部硬编码的依赖创建逻辑：

- `deck` 使用全局 `defaultRegistry`
- `knot.GetConfig()` 使用 `sync.Once` 全局单例
- `helm` 内部直接 `new` 具体的 LLM 实现
- `voyage` 内部直接 `new` 具体的 Vault 和 Gate 实现
- `sail` 直接使用 `http.DefaultClient`
- `brig` 的静态规则在代码里写死，且规则加载与裁决逻辑混在一个类型里

这些设计在项目早期降低了复杂度，但带来以下隐患：

1. 单元测试难以替换实现（必须真实读写磁盘/调用网络）
2. 组件之间耦合具体实现，替换 provider、存储或规则需要修改被调用方源码
3. 全局状态导致并发测试和配置隔离困难

## 目标

将依赖的**创建权**从各包内部上移到 `argo-server/main.go` 启动时统一组装，被调用方只依赖**接口**。

核心原则：

- 组件声明自己需要什么能力（interface）
- 启动时根据配置决定使用哪个实现
- 单元测试可注入 mock 实现

## 改造范围

### 包含

| 组件 | 注入内容 | 涉及文件 |
|------|----------|----------|
| `helm.PhaseRunnerModel` | `model.Provider` + `tool.Registry` | `src/helm/phase_model.go` |
| `helm.PhaseRunnerExecTools` | `tool.Registry` | `src/helm/phase_exec_tools.go` |
| `helm` 工厂 | `RunnerFactory` 预组装 4 个 PhaseRunner | `src/helm/phase_runner.go` |
| `voyage.Session` | `vault.Vault` + `brig.Gate` | `src/voyage/voyage.go` |
| 配置读取 | 移除 `knot.GetConfig()` 全局单例，启动时加载并注入 | `src/knot/config.go` 及所有调用点 |
| `sail` | `*http.Client` | `src/sail/sail.go` |
| `brig` 静态规则 | `RuleLoader` 接口，`FileGuard` 拆分为 `Warden`（裁决引擎）+ `RuleLoader`（规则来源） | `src/brig/brig.go`、`src/brig/deny.go` |

### 不包含

- Server 层抽象 `AgentService`：当前 `handlePrompt` 继续直接调用 voyage / helm，暂保持现状。

## 具体改造

### 1. `helm.PhaseRunnerModel`

**当前：**

```go
func (pr *PhaseRunnerModel) Run(...) {
    pr.getModelConfig()                  // 内部读全局配置
    s := sail.NewSailOpenAI(...)         // 内部创建具体 LLM 客户端
    events := s.Chat(ctx)
}
```

**改造后：**

```go
type PhaseRunnerModel struct {
    Provider model.Provider   // 注入
    Registry tool.Registry    // 注入
}

func (pr *PhaseRunnerModel) Run(...) {
    events := pr.Provider.Chat(ctx, req)
}
```

`model.Provider` 接口至少包含：

```go
type Provider interface {
    Chat(ctx context.Context, req ChatRequest) (<-chan knot.Event, error)
}
```

`tool.Registry` 接口至少包含：

```go
type Registry interface {
    List() []knot.Tool
    Lookup(name string) (knot.Tool, bool)
}
```

### 2. `helm.PhaseRunnerExecTools`

**当前：**

```go
tool, found := deck.Lookup(tu.Name)
```

**改造后：**

```go
type PhaseRunnerExecTools struct {
    Registry tool.Registry   // 注入
}

tool, found := pr.Registry.Lookup(tu.Name)
```

### 3. `helm.RunnerFactory`

将 `newRunner(phase int)` 内部临时创建 runner 的方式，改为启动时预组装：

```go
type RunnerFactory struct {
    ModelRunner     PhaseRunner
    ExecToolsRunner PhaseRunner
    AskingRunner    PhaseRunner
    DoneRunner      PhaseRunner
}

func (f *RunnerFactory) Create(phase int) PhaseRunner {
    switch phase {
    case HelmPhaseModel:
        return f.ModelRunner
    case HelmPhaseExecTools:
        return f.ExecToolsRunner
    case HelmPhaseAsking:
        return f.AskingRunner
    case HelmPhaseDone:
        return f.DoneRunner
    default:
        return f.DoneRunner
    }
}
```

### 4. `voyage.Session`

**当前：**

```go
func NewSession(argoDir, workingDir string) (*Session, error) {
    ...
    return &Session{
        Vault: vault.NewFileVault(sessionDir),  // 硬编码
        Guard: brig.NewFileGuard(workingDir),    // 硬编码
    }, nil
}
```

**问题：** 如果直接改为 `NewSession(info, vault, guard)`，调用方（`server.go`）被迫要知道 session 目录怎么建、`session.json` 怎么写、ID 怎么生成——实现细节全部泄露到上层。

**改造后：** 采用 functional options 模式。签名不变，默认行为零改动，测试通过 Option 注入 mock。

```go
// 新增 Option 类型
type SessionOption func(*Session)

func WithVault(v vault.Vault) SessionOption {
    return func(s *Session) { s.Vault = v }
}

func WithGate(g brig.Gate) SessionOption {
    return func(s *Session) { s.Gate = g }
}

// NewSession 签名只加可变参数，其余不变
func NewSession(argoDir, workingDir string, opts ...SessionOption) (*Session, error) {
    // …创建目录、生成 ID、写 session.json（与当前完全一致）…

    // 先填默认实现
    s := &Session{
        SessionInfo: info,
        Vault:       vault.NewFileVault(sessionDir),
        Gate:        brig.NewDefaultWarden(workingDir),
    }

    // 再让 Option 覆盖
    for _, opt := range opts {
        opt(s)
    }
    return s, nil
}
```

**生产代码——零改动：**

```go
session, err := voyage.NewSession(s.argoDir, req.CWD)
```

**测试代码——注入 mock：**

```go
session, _ := voyage.NewSession("/tmp/argo", "/tmp/work",
    voyage.WithVault(&vault.MemVault{}),
    voyage.WithGate(&brig.NoopGate{}),
)
```

`ResumeSession` 同理，加 `opts ...SessionOption` 可变参数。

`vault.Vault` 和 `brig.Gate` 接口保持现有定义不变（仅 `Guard` 更名为 `Gate`）。

### 5. 配置注入

**当前：**

```go
var (
    globalCfg  *Config
    globalOnce sync.Once
)

func GetConfig() (*Config, error) { ... }
```

多个包内部调用 `knot.GetConfig()` 或 `knot.LoadConfig()`：

- `src/helm/phase_model.go`
- `src/sail/sail.go`
- `src/deck/tool.go`

**改造后：**

- 删除 `knot.GetConfig()` 全局单例
- 只在 `argo-server/main.go` 启动时调用一次 `knot.LoadConfig()`
- 将配置（或相关子配置）注入到各组件

示例组装：

```go
cfg, err := knot.LoadConfig()

provider := openai.NewProvider(cfg.Sail)
registry := deck.NewRegistry(cfg.Deck)

runner := helm.NewRunnerFactory(
    helm.NewPhaseRunnerModel(provider, registry),
    helm.NewPhaseRunnerExecTools(registry),
    &helm.PhaseRunnerAsking{},
    &helm.PhaseRunnerDone{},
)
```

### 6. `sail` 注入 HTTP Client

**当前：**

```go
resp, err := http.DefaultClient.Do(httpReq)
```

**改造后：**

```go
type OpenAIProvider struct {
    Client *http.Client
    ...
}

resp, err := p.Client.Do(httpReq)
```

生产环境可设置超时、代理；测试环境可注入 mock client。

### 7. `brig.FileGuard` 拆分为 Warden + RuleLoader

**当前问题：** `FileGuard` 把规则加载和裁决逻辑混在一起，且规则来源硬编码。未来若支持多租户、Redis / DB 规则源，无法扩展。

**改造方向：** 拆为三层——顶层接口 `Gate`、裁决引擎 `Warden`、规则来源接口 `RuleLoader`。

```
Gate (接口)              ← helm/voyage 只认识这个
  └── Warden (实现)       ← 裁决引擎，分层评估
        ├── mandates []Rule          ← 系统红线，不可绕过
        ├── rules (RuleLoader 加载)  ← 用户/租户规则
        └── approvals map            ← 动态审批缓存
```

#### 命名

| 原名 | 新名 | 说明 |
|------|------|------|
| `brig.Guard` (接口) | `brig.Gate` | 门禁——非角色名，不和 Warden 撞 |
| `brig.FileGuard` (struct) | `brig.Warden` | 典狱长——裁决引擎的单一实现 |
| — | `brig.RuleLoader` (接口) | 规则来源——只拉数据，不参与裁决 |
| — | `brig.BuiltinRuleLoader` | 代码内置规则 |
| — | `brig.FileRuleLoader` | JSON 文件规则 |
| — | `brig.StaticRuleLoader` | 测试用静态规则 |

#### Gate 接口（继承原 Guard，仅重命名）

```go
type Gate interface {
    Evaluate(tu knot.ToolUse) (int, string)
    Approve(tu knot.ToolUse)
    IsCachedApproval(tu knot.ToolUse) bool
    Snapshot() []ApprovalEntry
    Restore([]ApprovalEntry)
}
```

#### RuleLoader 接口

```go
type RuleLoader interface {
    Load() ([]Rule, error)
}
```

#### Warden 实现

```go
type Warden struct {
    mandates   []Rule
    rules      []Rule           // 由 RuleLoader 加载
    approvals  map[ToolCall]struct{}
    workingDir string
    mu         sync.RWMutex
}

func NewWarden(workingDir string, mandates []Rule, loaders ...RuleLoader) (*Warden, error) {
    var rules []Rule
    for _, l := range loaders {
        loaded, err := l.Load()
        if err != nil {
            return nil, fmt.Errorf("brig: %w", err)
        }
        rules = append(rules, loaded...)
    }
    return &Warden{
        mandates:   mandates,
        rules:      rules,
        approvals:  make(map[ToolCall]struct{}),
        workingDir: workingDir,
    }, nil
}
```

#### Evaluate 分层裁决逻辑

```
Warden.Evaluate(toolCall)
  │
  ├── 1. mandates（系统红线）→ Deny 直接拒，不可绕过
  ├── 2. rules（RuleLoader 来源）→ 命中即生效，可收紧系统 Allow/Ask
  └── 3. 兜底 → Ask
```

```go
func (w *Warden) Evaluate(tu knot.ToolUse) (int, string) {
    tc := ToolCall{ToolName: tu.Name, RawParam: scopeKey(tu.Name, knot.GetRawParam(tu)), WorkingDir: w.workingDir}

    // 第一层：系统红线 Deny（不可绕过）
    for _, r := range w.mandates {
        if r.Action == Deny && matchRule(r, tc) {
            return Deny, formatReason(tc, "blocked by security policy")
        }
    }

    // 第二层：用户/租户规则（first-match）
    for _, r := range w.rules {
        if matchRule(r, tc) {
            if r.Action == Ask && w.approved(tc) {
                return Approve, formatReason(tc, "user approved")
            }
            return r.Action, formatReason(tc, "matched user rule")
        }
    }

    // 兜底
    if w.approved(tc) {
        return Approve, formatReason(tc, "user approved")
    }
    return Ask, formatReason(tc, "permission required")
}
```

#### 三个内置 RuleLoader

```go
// BuiltinRuleLoader 返回代码内置的安全规则（safeCommands / dangerCommands / sensitiveFiles 等）
type BuiltinRuleLoader struct{}
func (l *BuiltinRuleLoader) Load() ([]Rule, error)

// FileRuleLoader 从 JSON 文件加载用户自定义规则
type FileRuleLoader struct{ Path string }
func (l *FileRuleLoader) Load() ([]Rule, error)

// StaticRuleLoader 测试用，直接传 []Rule
type StaticRuleLoader struct{ Rules []Rule }
func (l *StaticRuleLoader) Load() ([]Rule, error)
```

#### 构造方式

用 `...RuleLoader` 变参（不用 Option，因为只有规则来源这一个可变维度）：

```go
// 零 loader → 纯红线（与当前 NewFileGuard 能力等价）
warden, _ := brig.NewWarden(wd, brig.BuiltinMandates())

// 红线 + 用户文件规则
warden, _ := brig.NewWarden(wd, brig.BuiltinMandates(),
    &brig.FileRuleLoader{Path: "~/.argo/rules.json"},
)

// 多租户：红线 + 文件 + Redis
warden, _ := brig.NewWarden(wd, brig.BuiltinMandates(),
    &brig.FileRuleLoader{Path: tenantRules},
    &brig.RedisRuleLoader{Client: rdb, Key: "tenant:42:rules"},
)

// 红线 + 文件规则 + 内存兜底规则——可以各规则混合使用
warden, _ := brig.NewWarden(wd, brig.BuiltinMandates(),
    &brig.FileRuleLoader{Path: "~/.argo/rules.json"},
    &brig.StaticRuleLoader{Rules: []Rule{{Action: Ask, Tool: "bash", Pattern: "docker"}}},
)

// 测试：无红线，纯自定义
warden, _ := brig.NewWarden(wd, nil,
    &brig.StaticRuleLoader{Rules: []Rule{{Action: Deny, Tool: "bash", Pattern: "rm"}}},
)
```

#### 便捷构造

```go
func BuiltinMandates() []Rule { return denyRules }

func NewDefaultWarden(workingDir string, loaders ...RuleLoader) (*Warden, error) {
    return NewWarden(workingDir, BuiltinMandates(), loaders...)
}
```

#### 与当前代码的映射

| 当前 | 改造后 |
|------|--------|
| `brig.Guard` 接口 | → `brig.Gate`，方法签名不变 |
| `brig.FileGuard` struct | → 删除，拆为 `Warden` + `FileRuleLoader` |
| `loadStaticRules()` | → `BuiltinRuleLoader.Load()` |
| `resolveStaticRules()` | → `Warden.Evaluate()` 内部分层逻辑 |
| `denyRules` 变量 | → `BuiltinMandates()` 辅助函数 |
| `NewFileGuard(wd)` | → `NewDefaultWarden(wd)` |
| `safeCommands` 等规则组 | → `BuiltinRuleLoader.Load()` 内部 |

## 组装点

所有依赖的组装统一放在 `argo-server/main.go`：

```
argo-server/main.go
    │
    ├─ 加载配置：cfg, _ := knot.LoadConfig()
    │
    ├─ 创建基础设施
    │   ├─ client := &http.Client{Timeout: 5m}
    │   ├─ provider := openai.NewProvider(cfg.Sail, client)
    │   ├─ registry := deck.NewRegistry(cfg.Deck)
    │   ├─ ruleLoader := &brig.BuiltinRuleLoader{}
    │   └─ ...
    │
    ├─ 创建 voyage.Session 时通过 Option 注入 vault + guard（默认值无需传参）
    │
    ├─ 创建 helm.RunnerFactory 时注入 provider + registry
    │
    └─ 启动 server
```

## 推荐改造顺序

1. **先改 `sail` 的 HTTP Client 注入**：改动最小，无接口新增，影响面小
2. **再改 `brig` 的 FileGuard → Warden + RuleLoader**：拆分数据层和逻辑层，新增 `Gate`/`RuleLoader` 接口
3. **改造 `knot.GetConfig()`**：删除全局单例，修改所有调用点
4. **改造 `voyage` 的 Vault / Gate 注入**：通过 Option 模式注入，不影响 session 创建路径
5. **改造 `deck` registry**：从包级全局改为实例化注册表
6. **最后改 `helm` PhaseRunner 注入 + RunnerFactory**：依赖前面的 provider 和 registry

## 预期收益

- 各组件可通过接口 mock 独立测试
- 替换 LLM provider、存储后端、规则来源不需要改被调用方代码
- 消除全局状态，支持多实例和并发测试
- 启动时的依赖关系一目了然

## 未决事项

- Server 层是否抽象 `AgentService`：当前不实施，待 `handlePrompt` 进一步膨胀或需要支持新传输层时再评估。
- 日志模块：保持现有 `slog.SetDefault` 方案；chi 负责 access log，slog 负责 application log。
