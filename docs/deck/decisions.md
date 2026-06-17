# deck 技术决策记录

---

## DEC-001：工具接口最小字段

**日期**：2026-06-02
**状态**：已决定

**决策**：`Tool` 接口 5 个方法——`Name` + `Description` + `Source` + `Execute` + `Concurrency`。

**背景**：调研 7 个项目（Claude-Code、OpenCode、Hermes Agent、CrewAI、OpenClaw、Nanoclaw、pi）后，pi 的四要素接口（name + description + parameters + execute）是最干净的抽象。Claude-Code 的 Tool 接口有 10+ 字段（包括 UI 渲染器、权限检查、MCP 标志），OpenCode 的返回 Effect 类型，这些对 MVP 都是过度设计。

2026-06-12 新增 `Source() ToolSource` 方法。Register 在处理同名冲突时需要区分 builtin > 外部，若通过隐式类型断言获取来源，任何人对 Tool 接口的修改都可能间接破坏 Register 的优先级逻辑。`Source()` 作为显式契约消除这个隐患。`ToolSource` 为三值枚举（`ToolSourceBuiltin` / `ToolSourceCLI` / `ToolSourceMCP`）。

**拒绝的替代方案**：
- 加入 `toolset` 字段用于分组过滤 → MVP 阶段工具数量少，不需分组。后续可加。
- 加入 `isConcurrencySafe` / `isReadOnly` 标志 → 这些属于执行阶段的属性，与注册无关。按需在 executor 层处理。
- 加入 UI 渲染器（`renderToolUseMessage` 等）→ Argo 无 GUI/TUI，不需要。
- 加入 `isMCP` 或特定来源标志 → 不采用单一来源标志，`Source()` 统一表达三种来源，扩展新来源时无需加新方法。

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

## DEC-007：工具结果后处理方案（重写，supersedes 原 DEC-007 和 DEC-009）

**日期**：2026-06-16
**状态**：已决定

**决策**：每个 Tool 自带 `TruncationConfig`（策略 + 保留比例），postProcess 按配置执行单一策略截断，不再做隐式的 head+tail 组合。

> 原 DEC-007（2026-06-04）采用 head 40% + tail 60% 组合，截断后总长约 50078 字符 ≈ `MaxOutputChars`，几乎不节省（< 1%），存在设计悖论。
> 原 DEC-009（2026-06-16）改为 head/tail 二选一 + truncation 目录，但缺少 HeadAndTail 策略，过于极端。
> 本条重写并替代上述两条。

### 1. 三种截断策略

`TruncationStrategy` 为 iota 整型枚举，落在 `types.go`：

| 常量 | String() | 行为 |
|------|----------|------|
| `TruncationHead` | `"head"` | 保留前 N 字符，N = OutputLen × Ratio |
| `TruncationTail` | `"tail"` | 保留后 N 字符，N = OutputLen × Ratio |
| `TruncationHeadAndTail` | `"HeadAndTail"` | 保留前 N1 字符 + 后 N2 字符，N1 = OutputLen × Ratio, N2 = OutputLen × TailRatio |

`TruncationConfig` 结构（落在 `types.go`）：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `Strategy` | `TruncationStrategy` | — | 截断策略（必填） |
| `Ratio` | `float64` | `0.2` | 主保留比例，head 和 tail 策略的保留比例，HeadAndTail 时的头部保留比例 |
| `TailRatio` | `float64` | `0.2` | 尾部保留比例，仅 HeadAndTail 策略启用 |

`Validate()` 方法：
- `Ratio`：必须 `> 0` 且 `≤ 0.8`，否则返回 error
- `TailRatio`：仅 `TruncationHeadAndTail` 时校验，必须 `> 0` 且 `≤ 0.8`，否则返回 error
- 每个维度独立校验，独立报错

### 2. 配置来源分离

| 工具来源 | TruncationConfig 来源 | 可被覆盖 |
|---------|----------------------|---------|
| builtin | `NewBuiltinTool` 时传入默认值 | 否，硬编码 |
| CLI | `tools.json` cli 条目内嵌可选 `truncation` 字段 | 用户可配 |
| MCP | `tools.json` mcp 条目内嵌可选 `truncation` 字段 | 用户可配 |

CLI/MCP 省略 `truncation` 时使用默认值：strategy=`tail`, ratio=`0.2`。

`tools.json` 截断配置格式：

```json
// CLI head 策略
{ "name": "gh", "description": "GitHub CLI",
  "truncation": { "strategy": "head", "ratio": 0.3 } }

// MCP tail 策略（ratio 默认 0.2）
{ "name": "github", "command": ["npx", "-y", "..."],
  "truncation": { "strategy": "tail" } }

// HeadAndTail 策略
{ "name": "docker", "description": "Docker CLI",
  "truncation": { "strategy": "HeadAndTail", "ratio": 0.2, "tail_ratio": 0.2 } }

// 省略时使用默认 tail(0.2)
{ "name": "kubectl", "description": "Kubernetes CLI" }
```

### 3. 6 个 builtin 工具默认策略

| 工具 | Strategy | Ratio | 理由 |
|------|----------|-------|------|
| `bash` | `tail` | `0.2` | 错误/退出码/最终结果在尾部（pi `shell-output.ts:112` 同） |
| `read` | `head` | `0.2` | 文件开头是文件元信息/第一行（pi `truncate.ts:120` 同） |
| `grep` | `head` | `0.2` | 匹配项在头部（前 N 行） |
| `glob` | `head` | `0.2` | 输出小，几乎不触发截断 |
| `write` | `head` | `0.2` | 输出小，几乎不触发截断；保底用 head |
| `edit` | `head` | `0.2` | 输出小，几乎不触发截断；保底用 head |

### 4. 完整输出持久化到 truncation 目录

截断时把完整 Output 写入 `~/.argo/truncation/tool-<timestamp>-<random>.log`。LLM 通过 `read` 工具按需查尾。

目录管理（`deck/truncation.go`）：
- `writeTruncation(content) (path string, err error)`
- `cleanupOldTruncations(retention time.Duration)` — 默认保留 7 天（参考 opencode）

### 5. metadata 驱动 LLM 决策

截断时 `ToolResult.Metadata` 自动追加 5 个字段：

| 字段 | 类型 | 含义 |
|------|------|------|
| `truncated` | `bool` | 是否被截断 |
| `original_length` | `int` | 截断前 Output 长度 |
| `kept_length` | `int` | 截断后 Output 长度 |
| `truncation_strategy` | `string` | 截断策略名（`"head"` / `"tail"` / `"HeadAndTail"`） |
| `full_output_path` | `string` | 完整输出文件绝对路径（truncated=true 时必填） |

LLM 看到 metadata 可决策：任务只需截断内容→直接用；任务需要尾部/完整信息→调用 `read` 工具读 `full_output_path`。

### 6. 触发条件

`outputLen > MaxOutputChars` 时触发截断。去掉 `+TruncationMinSavings` 兜底——新设计截断后输出远小于上限，永远节省。

`TruncationMinSavings` 常量废弃（从 `types.go` 删除）。

### 7. postProcess 签名

```go
func postProcess(result ToolResult, cfg TruncationConfig) ToolResult
```

按 `cfg.Strategy` 分派 head / tail / HeadAndTail 行为，按 `cfg.Ratio` / `cfg.TailRatio` 计算截断位置，自动写 metadata 字段。

### 8. LoadFromConfig → LoadConfig

原 `LoadFromConfig` 函数重命名为 `LoadConfig`，语义更简洁。

### 拒绝的替代方案

| 方案 | 拒绝理由 |
|------|---------|
| 原 DEC-007 head 40%+tail 60% 组合 | head+tail=100%，截断后几乎不节省（< 1%），悖论式实现 |
| 原 DEC-009 head/tail 二选一，不做组合 | 过于极端，缺少保留两端信息的策略 |
| 框架统一决定 head/tail | 缺乏场景感知（bash 关键在尾、read 关键在头） |
| 工具可通过 tools.json 覆盖 builtin 默认策略 | builtin 策略由开发者预设，用户覆盖会导致行为不可预期 |
| `none` 策略 | 无意义——不需要截断的工具不会触发；万一触发了必须有策略兜底，用 head(0.2) 即可 |
| 字符串类型枚举 | 用 iota 整型更类型安全，配合 `String()` 方法和 JSON 自定义序列化 |

### 参考文献

- pi: `packages/agent/src/harness/utils/truncate.ts:125-289`（truncateHead/truncateTail），`shell-output.ts:50-138`（ensureFullOutputFile）
- opencode: `packages/opencode/src/tool/truncate.ts:16-142`（MAX_BYTES / output / write）

---

## DEC-010：TruncationConfig Ratio/TailRatio 改 int 百分点

**日期**：2026-06-17
**状态**：已决定

**决策**：`TruncationConfig.Ratio` 和 `TailRatio` 从 `float64` 改为 `int` 百分点（1~80），含义"保留 N% 字符"。

### 1. 改动详情

| 字段 | 原类型 | 新类型 | 默认值 | 范围 |
|------|--------|--------|--------|------|
| `Ratio` | `float64` | `int` | 20 | 1~80 |
| `TailRatio` | `float64` | `int` | 20 | 1~80 |

截断计算：

```go
// 原
n := int(cfg.Ratio * float64(len(output)))

// 新
n := len(output) * cfg.Ratio / 100
```

JSON 配置：

```json
// 原
{ "strategy": "head", "ratio": 0.3 }

// 新
{ "strategy": "head", "ratio": 30 }
```

### 2. 理由

1. **与三个参考项目对齐**：pi / opencode / Claude-Code 均用 int 做截断单位（`MAX_LINES` / `MAX_BYTES` / `DEFAULT_MAX_RESULT_SIZE_CHARS`），argo 是唯一引入 float 比例的项目。
2. **消除 float 痛点**：
   - 取整歧义：`0.2 × 60001 = 12000.2`，floor/ceil/round 三种实现差 1 字符
   - IEEE 754 表示误差：`0.1` 不可精确表示，`0.15` 实际是 0.14999999...
   - JSON 序列化漂移：边界值可能序列化为 `"0.20000000000000001"`
3. **零精度损失**：所有现有比例（0.1/0.15/0.2/0.25/0.3/0.5/0.8）都是 5% 整数倍，×100 后整数除法零误差。验证：`int(0.2 × 60000) = 12000` = `60000 × 20 / 100 = 12000`。
4. **API 更直观**：用户配置 `"ratio": 20` 比 `"ratio": 0.2` 表达"20%"更直接。
5. **与 pi/opencode 设计哲学一致**：byte 触发 + 整数化（避免浮点），argo 保留"按比例"模型但用 int 表达。

### 3. 改动范围

| 类别 | 文件数 |
|------|--------|
| 核心代码 | 3（`types.go` / `post_process.go` / `post_process_test.go`）|
| 任务文档 | 4（`postProcess.md` / `LoadConfig.md` / `builtin.md` / `cli.md`）|
| 设计文档 | 4（`how.md` / `what.md` / `examples.md` / `_tree.md`）|
| 审核文档 | 1（`postProcess.test-review.md`）|
| **合计** | **12 个文件** |

### 4. 拒绝的替代方案

| 方案 | 拒绝理由 |
|------|---------|
| 保留 float + 明文规定 `int(Ratio × float64(Len))` 向下取整 | 取整歧义仍存在（不同实现的 floor/ceil 选择不同）；argo 仍特立于三个参考项目 |
| 改 int 千分位（1~800）| 配置文件 `"ratio": 150` 看起来像 150%，不直观 |
| 改 int 上限模型（与 pi/opencode 一致）| 放弃"按比例"模型（DEC-007 核心设计），破坏 API 向后兼容 |
| 改 int 但 LoadConfig 仍用 float 转换 | 维护两层类型，未真正解决 float 痛点 |
| 限制 float 只能配"无小数比例"（0.1/0.2/0.5/0.8）| 仍然引入 float 痛点（取整、序列化），且限制用户灵活性 |

### 5. 兼容性

`TruncationConfig` 是 deck 包内基础类型（DEC-007 新增），尚未被外部用户使用——本次改动无外部 API 兼容性问题。LoadConfig 任务的 `TruncationConfigJSON` 反序列化类型同步改 `*int`。

---

## DEC-011：截断标记格式

**日期**：2026-06-17
**状态**：已决定

**决策**：截断标记统一为单行 ASCII 格式，包含三段信息（分号分隔）：

```
\n[...truncated: <strategy>, original <N> chars; full output: <path>...]\n
```

### 1. 格式说明

| 段 | 含义 | 取值 |
|---|------|------|
| `<strategy>` | 截断策略名 | `head` / `tail` / `HeadAndTail`（来自 `TruncationStrategy.String()`）|
| `<N>` | 原始 Output 字节数 | `len(input.Output)` |
| `<path>` | 完整输出文件绝对路径 | `~/.argo/truncation/tool-<timestamp>-<random>.log`（复用 DEC-007 第 212 行约定）|

### 2. 示例

head 策略 + 60000 字节输入 + 路径 `/home/u/.argo/truncation/tool-1700000000-abc123.log`：

```


[...truncated: head, original 60000 chars; full output: /home/u/.argo/truncation/tool-1700000000-abc123.log...]

```

### 3. 理由

1. **自包含**：路径在 Output 字符串中，LLM 必看 Output 就能找到完整文件，无需查 metadata
2. **LLM 读原文概率**：方案 A（仅 metadata 含路径）约 50-70%，方案 B（Output 含路径）接近 100%
3. **单行 + ASCII**：vs 方案 C 的 3-4 行，节省 token；ASCII 字符避免自身引入 UTF-8 复杂度（截断标记不会被截断）
4. **结构化**：分号分隔三段，LLM 可正则解析 `strategy` / `N` / `path`
5. **与 DEC-007 metadata 冗余设计**：路径在 metadata 和标记中各写一次，冗余是必要的强提示，避免 LLM 忽略 metadata

### 4. 与 DEC-007 关系

DEC-007 第 222-228 行规定 metadata 字段（含 `full_output_path`）。本 DEC 与之冗余设计——路径在 metadata 和标记中各写一次。**冗余是必要的强提示**，不是设计冗余。

### 5. 拒绝的替代方案

| 方案 | 拒绝理由 |
|------|---------|
| 方案 A（仅策略+原始长度，路径走 metadata）| LLM 注意力集中 Output，metadata.full_output_path 易被忽略，read 原文概率 50-70% |
| 方案 C（多行 hint + Read 工具指引）| 3-4 行标记 token 消耗大；hint 文字会被 LLM 复读浪费 token |
| 多语言标记（中英文混合）| LLM 输入统一英文，中文标记无意义 |

### 6. 测试要求

postProcess 任务测试必须包含（详见 `docs/tasks/deck/postProcess.md` 测试场景表）：

- `TestPostProcess_Marker_Head`：验证 head 策略 marker 完整格式
- `TestPostProcess_Marker_Tail`：验证 tail 策略 marker 包含 `tail`
- `TestPostProcess_Marker_HeadAndTail`：验证 HeadAndTail 策略 marker 包含 `HeadAndTail`
- `TestPostProcess_Marker_NotTriggered`：未截断时 Output 不含截断标记
- `TestPostProcess_Marker_TruncationPath`：marker 中的路径与 `metadata.full_output_path` 一致

---

## DEC-012：truncation 目录管理归属

**日期**：2026-06-17
**状态**：已决定

**决策**：truncation 目录管理（writeTruncation / cleanupOldTruncations）作为 Layer 0 独立 task，由 postProcess 通过 `truncation.Writer` 接口调用。**不污染 postProcess 函数签名**，不归 postProcess 内部实现，不归 builtin/cli 各自重复实现。

### 1. 架构

| 组件 | 职责 | 所在 |
|------|------|------|
| `truncation.Writer` 接口 | 抽象文件写入能力 | `truncation` 包（Layer 0） |
| `truncation.NewDefault()` | 返回 `~/.argo/truncation/` 写入器 | `truncation` 包 |
| `truncation.StartCleanup()` | 后台 ticker 清理超期文件 | `truncation` 包 |
| `postProcess` | 截断逻辑 + 调 Writer.Write | `post_process.go` |

### 2. Writer 接口签名

```go
package truncation

type Writer interface {
    // Dir 返回 Writer 关联的存储目录绝对路径。
    Dir() string
    // Write 把 content 写入 truncation 目录，返回完整绝对路径。
    // 失败时返回 ("", error)，不 panic。
    Write(content string) (path string, err error)
}

func NewDefault() Writer

func StartCleanup(w Writer, retention time.Duration, interval time.Duration) (stop func(), ticked <-chan struct{})
```

### 3. postProcess 集成

postProcess 函数签名**不变**：`func postProcess(result ToolResult, cfg TruncationConfig) ToolResult`

通过全局单例或依赖注入获取 Writer（具体方案在 [truncation.md](../tasks/deck/truncation.md) 规定）：

```go
var defaultWriter truncation.Writer = truncation.NewDefault()

func postProcess(result ToolResult, cfg TruncationConfig) ToolResult {
    // ... 触发截断时
    path, _ := defaultWriter.Write(result.Output)
    // 失败降级：path == "" 时 marker 不含 path 段
}
```

### 4. Write 失败降级策略

| Writer.Write 返回 | postProcess 行为 |
|------------------|------------------|
| `(path, nil)` | marker = `[...truncated: <strategy>, original <N> chars; full output: <path>...]`；`metadata.full_output_path = path` |
| `("", err)` | marker = `[...truncated: <strategy>, original <N> chars...]`（**不含 path 段**）；`metadata.full_output_path = ""`；log warn **不中断截断** |

**理由**：避免 LLM 见到无效路径（如 `""` 或错误占位符）后调用 read 工具产生不可预测操作。截断本身仍是有效的，LLM 仍能拿到截断后内容 + 策略 + 原始长度信息。

### 5. 路径复用 DEC-007

`~/.argo/truncation/tool-<unix-nano>-<random>.log`，retention = 7 天（DEC-007 第 212-216 行已规定）。

### 6. 拒绝的替代方案

| 方案 | 拒绝理由 |
|------|---------|
| 方案 A（归 postProcess）| postProcess 签名变化 `(ToolResult, error)`，波及所有 3 个 Tool.Execute |
| 方案 B（归 builtin/cli Execute）| 3 个 Tool 重复写 IO 逻辑，未来新增 Tool 也要重复 |
| Write 失败时中断截断 | metadata 4 字段都缺失，LLM 失去截断信息；降级保留截断信息更有用 |
| Write 失败时 marker 用 N/A 占位 | N/A 不是有效路径，LLM 仍可能尝试 read "N/A" 文件 |
| Write 失败时 marker 用错误信息 | 错误信息暴露内部细节（如路径、errno），且 LLM 看到错误可能误判 |

