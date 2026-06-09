# deck 技术决策记录

---

## DEC-001：工具接口最小字段

**日期**：2026-06-02
**状态**：已决定

**决策**：`Tool` 接口最少方法为 4 个——`Name` + `Description` + `Execute` + `Concurrency`。

**背景**：调研 7 个项目（Claude-Code、OpenCode、Hermes Agent、CrewAI、OpenClaw、Nanoclaw、pi）后，pi 的四要素接口（name + description + parameters + execute）是最干净的抽象。Claude-Code 的 Tool 接口有 10+ 字段（包括 UI 渲染器、权限检查、MCP 标志），OpenCode 的返回 Effect 类型，这些对 MVP 都是过度设计。

**拒绝的替代方案**：
- 加入 `toolset` 字段用于分组过滤 → MVP 阶段工具数量少，不需分组。后续可加。
- 加入 `isConcurrencySafe` / `isReadOnly` 标志 → 这些属于执行阶段的属性，与注册无关。按需在 executor 层处理。
- 加入 UI 渲染器（`renderToolUseMessage` 等）→ Argo 无 GUI/TUI，不需要。

---

## DEC-002：工具注册方式（三来源）

**日期**：2026-06-02
**状态**：已决定

**决策**：三种工具来源，三种注册方式：

| 来源 | 注册方式 | 理由 |
|------|---------|------|
| 内部工具 | `init()` + `deck.Register()` + `tools/builtin` 包导入触发 | Go 编译时保证，无需配置文件 |
| 外部 CLI | `~/.argo/tools.json` → `cli` 列表，每项 2 字段（name + description） | 零代码接入，参数通过 `--help` 现场获取 |
| MCP | `~/.argo/tools.json` → `mcp` 列表，每项 2 字段（name + command） | 服务器自描述（`tools/list`），无需手写 schema |

三种来源归一化为同一 `Tool` 接口。CLI 和 MCP 共用一个配置文件 `~/.argo/tools.json`。

**拒绝的替代方案**：
- CLI 工具和 MCP 工具分别用两个配置文件 → 增加用户心智负担，一个文件两个列表足够清晰。
- 外部 CLI 工具也走 `init()` 注册 → 要求用户写 Go 代码并重新编译 Argo，不可接受。
- 为外部 CLI 工具写 JSON Schema → 维护成本高，且与 `--help` 可能不同步。Vug 的 "CLAUDE.md 描述 + --help 动态发现" 模式更务实。
- 外部 CLI 工具描述散落在 CLAUDE.md 里 → 无法做可用性判定，工具离线时 LLM 仍会尝试调用。
- CLI wrapper 方案 → 每个工具都要从零手写 wrapper，不如直接用社区现成的 MCP 服务器。

---

## DEC-003：可用性判定方案

**日期**：2026-06-02
**状态**：已决定

**决策**：声明式条件（方案 B）+ 来源分流。

`Tool` 携带 `[]Condition` 字段，MVP 支持两种条件类型：`Kind: "binary"`（检查可执行文件在 PATH）和 `Kind: "env"`（检查环境变量已设置）。

- 内部工具：大多数不写 Conditions（默认永远可见），少数需要运行时环境的开发者手写
- 外部 CLI：`LoadConfig()` 自动生成 `{Kind: "binary", Key: "<name>"}`
- MCP：不写 Conditions，通过 `MCPManager.IsAlive()` 判定（`cmd.Wait()` + 后台 goroutine 被动检测子进程退出）

判定失败的工具有清晰的诊断信息（如 `"gh 不在 PATH"`），标记为 hidden 但保留在注册表中供诊断使用。

**拒绝的替代方案**：

| 方案 | 拒绝理由 |
|------|---------|
| **A: Check 函数** | 每个工具挂一个 `func Check() bool`，内部工具手写、外部工具自动生成。灵活但内部工具的检查逻辑散落在各工具包里，不可审计。 |
| **C: 来源自动判定** | 不暴露条件概念，按来源自动判定。太死——遇到"gh 在 PATH 但我暂时不想让 Agent 用它"用户无法控制。 |
| **OpenClaw 递归表达式树** | `allOf/anyOf` 递归组合很优雅，但 Argo MVP 的工具量级不需要复合条件。OpenClaw 用它是为了 133 个插件 + 35+ Provider 的规模。引入时机：Argo 外部工具超过 20 个且出现复合条件需求时。 |

---

## DEC-004：CLI vs MCP 的分工

**日期**：2026-06-02
**状态**：已决定

**决策**：CLI 和 MCP 之间不造中间层（如 CLI wrapper）。用户按需选择：

| | 选 CLI | 选 MCP |
|------|------|------|
| **场景** | 不关心权限管理，想省 token | 需要细颗粒度权限控制 |
| **token 消耗** | 低（schema 不进 prompt，--help 按需读） | 高（每个工具带完整 JSON Schema） |
| **权限管理** | 做不到方案层面的拦截（参数是字符串），但保留操作日志供事后审计 | 能做（参数是结构化 JSON，调用前可判定） |

**背景**：CLI 给 Argo 的是完整命令字符串（如 `gh repo delete argo --yes`），LLM 改一个单词就能绕过正则拦截。MCP 给的是结构化 JSON（`{"action": "delete_repo"}`），可以精确到字段级的事前判定。

**拒绝的替代方案**：CLI wrapper（Argo 调用自定义 wrapper，wrapper 接收 JSON 参数后内部调 CLI）——技术上可行但每个工具都要手写，不如直接用社区现成的 MCP 服务器。

---

## DEC-005：工具使用阶段的分区执行

**日期**：2026-06-02
**状态**：已决定

**决策**：按 `ConcurrencyMode` 静态标志分区——concurrent 的工具并行执行（goroutine + sync.WaitGroup），sequential 的工具串行执行（for 逐个）。

**原因**：LLM 一次可能返回多个 tool_use 块。写入操作（Edit、Write、Bash）会修改共享状态，并发执行会导致数据竞争和结果不可预测。只读操作（Read、Glob、Grep）不修改状态，可以安全并行。Claude-Code 的注释把这概括为"对人类开发工作流程的观察——同时查看多个文件，但逐个应用编辑"。

**分区方式**：pi 的静态标志方案——工具定义时声明 `executionMode: "parallel" | "sequential"`。工具作者预设，不做运行时判断。

**拒绝的替代方案**：

| 方案 | 拒绝理由 |
|------|---------|
| **全部串行** | 安全但慢——同时读 5 个文件也要排队，浪费 LLM 的等待时间 |
| **全部并发** | 快但不安全——写操作互相踩踏，结果不可预测 |
| **Claude-Code 的运行时判断** | `isConcurrencySafe(input)` 函数需要开发者针对每个工具写运行时判断逻辑。MVP 阶段工具数量少，静态标志足够，不需要运行时开销 |

---

## DEC-006：不做独立参数校验步骤

**日期**：2026-06-02
**状态**：已决定

**决策**：工具使用阶段不做独立的参数校验（JSON Schema 验证）步骤。工具直接执行，错误通过 ToolResult.Status="error" 原样返回给 LLM，由 LLM 自行修正。

**原因**：

1. 外部 CLI 工具根本无法预先校验——Argo 没有它的参数 schema，LLM 根据 `--help` 自己拼的命令字符串
2. 内部/MCP 工具的 schema 校验只是"参数类型对不对、必填字段有没有"的卫生检查，不能保证业务合法性。额外加一个关卡和"执行失败返回错误给 LLM"是同样的结果——LLM 都要收到错误信息然后自己修正
3. 框架层面做的校验只验证形态合法性（类型、必填），真正的业务校验（端口范围、文件是否存在）无论如何都要工具自己做
4. 多看一层就有多做一层 bug 的风险，MVP 应当先执行再纠错

**拒绝的替代方案**：

| 方案 | 拒绝理由 |
|------|---------|
| **工具使用前统一做 JSON Schema 校验** | 外部 CLI 没有 schema，做不了。内部/MCP 的校验和"执行失败返回错误"是同一条链路，没必要多一层 |
| **Hermes 的 coerce 类型强制** | LLM 传错了类型就直接报错让 LLM 自己改，不需要框架替 LLM 猜。锦上添花，MVP 不做 |

---

## DEC-007：工具结果后处理方案

**日期**：2026-06-04
**状态**：已决定

**决策**：MVP 取最简方案——单工具字符数上限 + 保留头部 40% + 尾部 60% + 明确截断标记。

```
if len(Output) > MaxOutputChars:
    head = Output[:MaxOutputChars * 0.4]
    tail = Output[end - MaxOutputChars * 0.6:]
    return head + "\n\n[... 输出被截断，省略 X 字符，原始长度 Y 字符 ...]\n\n" + tail
```

默认值：`MaxOutputChars=50000`，`PreviewChars=2000`。每个工具可通过 `Tool` 覆盖上限。

**原因**：工具输出截断是所有 Agent 框架的通用需求。一个 `cat 大文件` 就可能撑爆 context window。必须做，但 MVP 不需复杂方案。

**调研项目的分布**：

| 类别 | 项目 | 做法 |
|------|------|------|
| 纯文本截断 | Hermes Agent | 头 40%+尾 60%，中间标记 |
| 纯文本截断 | OpenClaw | 头+尾（关键词检测），明确标记 |
| 纯文本截断 | pi | 按工具类型选头/尾，行感知切割 |
| 持久化文件 | Claude-Code | 贪心摘除 → 完整内容写文件 → XML 引用 + 2KB 预览 |
| 持久化文件 | OpenCode | 单工具超限 → 写磁盘 → 返回预览 + outputPath |
| 不截断 | Nanoclaw | 依赖 SDK 内部处理 |

外部项目（Eino/CloudWeGo）有两阶段方案（截断 + 清理），但那是为长对话和多轮累积设计的。Argo MVP 先做最简单的。

**拒绝的替代方案**：

| 方案 | 拒绝理由 |
|------|---------|
| **持久化文件 + 预览（Claude-Code/OpenCode/Eino）** | 需管理临时文件生命周期、文件路径引用、清理策略。MVP 不引入文件管理复杂度 |
| **头+尾混合策略（Hermes/OpenClaw）** | 采纳。头部保留上下文标识（如文件名/命令名），尾部保留最新输出 —— 两端都比纯尾完整。按 Hermes 的 40/60 比例 |
| **聚合预算 + 贪心摘除（Claude-Code）** | 需遍历所有工具结果、排序、逐个替换。MVP 工具数量少，单工具上限已覆盖主要场景 |
| **两阶段截断+清理（Eino）** | 成熟的治理方案，但属于多轮对话累积场景。后续单轮对话溢出时考虑引入 |
| **持久化文件 + 预览（Claude-Code/OpenCode/Eino）** | 需管理临时文件生命周期、文件路径引用、清理策略。MVP 不引入文件管理复杂度 |

---

## DEC-008：Tool 接口 —— 三种未导出实现

**日期**：2026-06-04
**状态**：已决定

**决策**：工具统一抽象为 `Tool` 接口，三种来源通过三个未导出的 struct 实现。

```go
type Tool interface {
    Name() string
    Description() string
    Execute(ctx context.Context, params map[string]any) (ToolResult, error)
    Concurrency() ConcurrencyMode
}

// builtinTool — handler 函数指针。参考 Claude Code SDK 的 SimpleTool 模式
type builtinTool struct {
    name        string
    description string
    handler     func(ctx context.Context, params map[string]any) (ToolResult, error)
    concurrency  ConcurrencyMode
}

// cliTool — exec.Command 通用适配
type cliTool struct {
    name        string
    description string
    binary      string
    concurrency  ConcurrencyMode
}

// mcpTool — 委托 MCPManager.CallTool。Name 由 serverName + toolName 推导
type mcpTool struct {
    description string
    serverName  string
    toolName    string
    inputSchema json.RawMessage
    manager     *MCPManager
    concurrency  ConcurrencyMode
}
```

**原因**：

原设计使用 `Executor` interface + `ToolExecutor` 调度器两层结构。后续审查发现：

1. `ToolExecutor` 和 `Executor` 职责重叠——ToolExecutor 做权限检查 + 分区调度 + 后处理，Executor 做单次执行，但 helm 实际只需要 `Lookup` + `Execute`
2. 参考 Claude Code SDK 的 `SimpleTool` 模式，builtin 工具用函数指针而非独立 struct，足够简单
3. 包级 `ExecuteBatch` 吸收批量调度，`postProcess` 降为内部函数，不再需要 `ToolExecutor` 这个公开类型

**拒绝的替代方案**：

| 方案 | 拒绝理由 |
|------|---------|
| **Executor interface + ToolExecutor 调度器** | 两层抽象导致 helm 需要了解 Lookup → buildExecutor → Execute 三步流程，暴露了内部结构 |
| **Tool struct + 工厂函数** | 工具不应该先定义再构建——Tool 接口直接具备 Execute 能力，拿到就能用 |
