# deck 技术决策记录

---

## DEC-001：工具接口最小字段

**日期**：2026-06-02
**状态**：已决定

**决策**：`ToolDef` 最少注册字段为 4 个——`name` + `description` + `parameters` + `execute`。

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
| 内部工具 | `init()` 自注册 + barrel 触发 | Go 编译时保证，无需配置文件 |
| 外部 CLI | `~/.argo/tools.json` → `cli` 列表，每项 2 字段（name + description） | 零代码接入，参数通过 `--help` 现场获取 |
| MCP | `~/.argo/tools.json` → `mcp` 列表，每项 2 字段（name + command） | 服务器自描述（`tools/list`），无需手写 schema |

三种来源归一化为同一 `ToolDef` 接口。CLI 和 MCP 共用一个配置文件 `~/.argo/tools.json`。

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

`ToolDef` 携带 `[]Condition` 字段，MVP 支持两种条件类型：`Kind: "binary"`（检查可执行文件在 PATH）和 `Kind: "env"`（检查环境变量已设置）。

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
