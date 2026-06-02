# 工具管理（toolbox / deck）设计调研

> 调研对象：Claude-Code、OpenCode、Hermes Agent、CrewAI、OpenClaw、Nanoclaw、pi

---

## 1. 全局共识

七个项目在工具管理上形成了一种**趋同演化**的模式：

- **统一的工具接口**：CLI 内建工具和 MCP 外部工具最终归一化为同一个接口
- **声明式工具描述**：通过 JSON Schema / TypeBox / Zod 模式让 LLM 理解工具签名
- **权限/审批门控**：在执行前做安全检查，而非执行后审计
- **来源透明的执行**：LLM 不关心工具是本地执行还是通过 MCP 协议远程调用

---

## 2. Claude-Code：`buildTool()` 工厂 + MCP 一等公民

**核心文件**：`src/Tool.ts`、`src/tools.ts`、`src/services/tools/toolOrchestration.ts`

### 大白话

Claude-Code 的工具箱像一个**配备智能前台的巨型五金店**。

——**发现流程**：店里有 60+ 种工具，但前台不会一口气把所有工具目录甩给顾客（那样顾客会看晕，而且纸张也要钱）。大部分工具的说明书被收在抽屉里（`shouldDefer: true`），只给顾客一张"工具索引卡"（`ToolSearch` 元工具）。顾客说"我需要一个能搜代码的工具"——前台翻抽屉，抽出 GrepTool 的说明书递过去。即用即查，不浪费纸（token）。

——**选取和使用流程**：顾客拿到多张说明书后，可能会同时要求"读文件 A""读文件 B""搜关键词"——前台判断这些都是只读操作，互相不打架，于是一次性交给三个店员并行处理（`isConcurrencySafe=true` → Promise.all）。但如果顾客要求"编辑文件 A""编辑文件 B"，前台会要求排队——改代码这种事，不能两个人同时改同一个地方（`isConcurrencySafe=false` → 顺序执行）。这套规则来自对人类工匠的观察：你能同时翻好几本参考书，但下刀的时候一定是一刀一刀来。

——**外来工具接入**：店里有自营工具（内建），也接受外部供应商供货（MCP 服务器）。外来货必须套上统一的包装盒（MCPTool 包裹为 Tool 接口），贴上 `mcp__供应商名__货名` 的标签。如果外来货和自营货重名，自营优先——你不能让一个外部供应商的"锤子"替换掉店里质检过的锤子。

### 设计概述

Claude-Code 的工具系统围绕一个核心洞察构建：**60+ 工具如果每个都手写完整的样板代码（权限检查、并发标记、UI 渲染、错误处理），维护成本将是灾难性的**。因此它引入了 `buildTool()` 工厂函数——你只需要传入工具的核心定义（名称、描述、参数 schema、执行函数），工厂自动填充所有默认值：`isEnabled=true`、`isConcurrencySafe=false`、`checkPermissions=allow`。这形成了一套"约定优于配置"的工具开发范式。

工具的"执行时"生命周期比"注册时"更有趣。当 LLM 返回 tool_use 块后，系统进入 `partitionToolCalls()` 阶段——所有工具调用按 `isConcurrencySafe` 属性拆分为并发组和顺序组。只读工具（Glob、Grep、Read）天然并发安全，可以 Promise.all 批量执行；写入工具（Edit、Write、Bash）必须串行。这个分区逻辑源自对人类开发行为的观察：你会同时打开多个文件查看，但改代码时一定是一个一个来。

MCP 工具的处理方式体现了"一等公民"的设计态度：外部 MCP 工具被包装为完全相同的 `Tool` 接口，与内建工具通过 `assembleToolPool()` 合并。合并时 `uniqBy('name')` 保证内建工具优先——这意味着即使 MCP 服务器提供了一个名叫 `bash` 的工具，也不会覆盖内置的 BashTool。命名空间用 `mcp__<server>__<tool>` 格式隔离。

另一个关键设计是**延迟加载（ToolSearch）**：60+ 个工具的全部 JSON Schema 描述加起来非常消耗 prompt token。因此大部分工具标记为 `shouldDefer: true`，不随 system prompt 发送。LLM 改用一个 `ToolSearch` 元工具按需搜索——"我现在需要一个能搜索代码的工具" → ToolSearch 返回 GrepTool 的描述。这在不牺牲发现能力的前提下大幅节省 prompt 空间。

最后，Claude-Code 的工具系统还有一个独特之处：**工具即 UI 组件**。每个工具携带自己的 React 渲染器（`renderToolUseMessage`、`renderToolResultMessage`），这意味着工具的执行过程不仅是函数调用，也是一段可交互的 UI 体验——例如 BashTool 的渲染器会显示终端风格的输出，EditTool 会渲染 diff 视图。

### 工具接口

```typescript
type Tool<Input, Output, Progress> = {
  name: string
  description(input, options): Promise<string>      // LLM 可读描述
  inputSchema: Input                                  // Zod 模式
  call(args, context, canUseTool): Promise<ToolResult> // 核心执行
  isConcurrencySafe(input): boolean                   // 可并行？
  isReadOnly(input): boolean                          // 不修改状态？
  isEnabled(): boolean                                // 功能门控
  checkPermissions(input, context): PermissionResult  // 权限检查
  renderToolUseMessage / renderToolResultMessage       // UI 渲染
  isMcp?: boolean                                     // MCP 来源标志
}
```

### 工具注册与合并

```
getAllBaseTools() → Tool[]     // 60+ 内建工具
    +
MCP 服务器发现的工具 → MCPTool[]
    │
    └── assembleToolPool(builtIn, mcp):
          1. 按名称排序（缓存稳定性）
          2. uniqBy('name') → 内建工具优先于 MCP 同名冲突
```

### 工具执行分区

```
模型产生 tool_use 块
  │
  └── partitionToolCalls():
        │
        ├── 对于每个工具，检查 isConcurrencySafe(input):
        │   YES → 加入当前 "并发安全" 批次
        │   NO  → 开始新批次（顺序执行）
        │
        └── runTools(batches):
              ├── 批次 [A,B,C] 全部并发安全 → Promise.all(max=10)
              └── 批次 [D] 非并发安全 → for/of 逐个执行
```

### 权限系统

```
canUseTool(tool, input):
  ├── allow → 继续
  ├── deny  → 返回错误
  └── prompt → UI 对话框 → 用户决定 allow/deny
```

### 延迟加载（ToolSearch）

标记为 `shouldDefer: true` 的工具不直接发送给 LLM。模型获得一个 `ToolSearch` 工具按需搜索。这节省了 prompt 空间（60+ 工具的描述会很占 token）。

### 设计思想

1. **`buildTool()` 消除样板代码**：传入部分定义，自动填充 isEnabled=true、isConcurrencySafe=false、checkPermissions=allow 等默认值
2. **读并行、写串行**：源自对人类开发工作流程的观察——同时查询多个文件，但逐个应用编辑
3. **MCP 命名空间**：`mcp__${serverName}__${toolName}` 格式防止冲突，内建工具优先
4. **工具即 UI 组件**：每个工具有自己的 React 渲染器（renderToolUseMessage、renderToolResultMessage），工具执行不仅是函数调用，也是 UI 体验

---

## 3. OpenCode：Effect-TS 服务 + MCP 动态管理

**核心文件**：`packages/opencode/src/tool/tool.ts`、`tool/registry.ts`、`mcp/index.ts`

### 大白话

OpenCode 的工具箱像一个**流水线质检车间**。

——**发现流程**：车间有一个总调度台（`ToolRegistry`）。每天早上开工时，调度台先按"今天用哪个模型"筛一轮（某些工具只有特定模型能用），再按"这个 Agent 的权限等级"筛一轮，最后让插件工头（Plugin）拿着清单做最后一轮检查——"这个工具的描述要不要换个说法，让今天的模型更好理解？"（`transform(tool.definition)`）。三轮筛选后，最终清单才递到模型手上。

——**选取和使用流程**：每条工具的执行不是一个直接操作，而是一张**工单**（`Effect`）。工单上写的是"如何做这件事"，不是"现在就做"。好处是——如果老板按了暂停键（AbortSignal），所有相关工单自动取消；如果某道工序失败了，工单可以按预设策略自动重试（`Effect.retry`）；如果某道工序需要连接数据库，工单上已经标注好了找谁要钥匙（依赖注入）。

——**外来工具接入**：车间有一间专门的"外部供应商调度室"（`MCP.Service`），它负责和所有外部供应商保持联系。本地供应商通过管道直连（`StdioClientTransport`，开个子进程）；远程供应商通过 HTTP 定时联络（`StreamableHTTP + SSE`）。如果有供应商说"我换了新货"（`ToolListChangedNotification`），调度室立刻更新目录，不停工。

### 设计概述

OpenCode 的工具系统建立在 Effect-TS 之上，这意味着每个工具的 `execute` 函数返回的不是 Promise，而是一个 `Effect`——一个描述"如何执行这个副作用"的数据结构。这带来的好处是：工具的执行天然支持取消（通过 AbortSignal 传播）、重试（`Effect.retry`）、依赖注入（工具可以通过 Effect 的 `Context` 获取数据库连接等服务）。工具的并发控制因此变得非常简单——Effect 运行时自己管理并发上限，不需要手写线程池。

工具发现由 `ToolRegistry` 统一管理。每轮对话开始时，`ToolRegistry.tools(model, agent)` 会做三层过滤：先按模型能力过滤（例如 websearch 工具只在 opencode provider 下可用），再按 agent 权限过滤，最后让 Plugin 通过 `transform(tool.definition)` 钩子在工具发送给 LLM 之前做最后一轮调整。这种"先过滤再变换"的流水线确保了 LLM 看到的工具列表是精确匹配当前场景的。

MCP 工具的管理方式值得一提：`MCP.Service` 是一个长期运行的服务，管理所有 MCP 服务器的连接生命周期。它同时支持本地连接（`StdioClientTransport`，启动一个子进程）和远程连接（`StreamableHTTP` + SSE + OAuth）。当 MCP 服务器发来 `ToolListChangedNotification` 时，Service 自动刷新工具缓存，无需重启 Agent。

内建工具本身涵盖了一个编码 Agent 的典型需求：shell、read、write、edit、glob、grep、task（子 Agent 委派）、skill、question（用户交互）、todo、webfetch、websearch 等。值得注意的是 `invalid` 工具——当 LLM 调用了一个不存在的工具时，`invalid` 会捕获并返回错误信息，帮助 LLM 自我修复（tool repair）。

自定义工具通过文件系统扫描加载：任何匹配 `{tool,tools}/*.{js,ts}` 的文件都会被自动发现和注册。这使得添加新工具的步骤简化到"在正确的目录下创建一个文件"——不需要修改任何注册表代码。

### 工具定义

```typescript
interface Def<Parameters, Metadata> {
  id: string
  description: string
  parameters: Schema           // Zod/Effect Schema
  execute(args, ctx): Effect<{ title, metadata, output, attachments? }>
  formatValidationError?(error): string
}
```

### 工具发现流程

```
ToolRegistry.tools(model, agent)
  │
  ├── 对于每个 [ ...builtin, ...custom ]:
  │     ├── 按 model 能力过滤（如 websearch 仅 opencode provider）
  │     ├── 按 agent 权限过滤
  │     └── Plugin.transform(tool.definition)  允许插件修改描述/参数
  │
  └── 合并 MCP 工具:
        MCP.Service.tools() → Record<string, AITool>
        每个 MCP 工具包装权限检查

MCP 服务器管理:
  ├── connectLocal(): StdioClientTransport
  ├── connectRemote(): StreamableHTTP / SSE + OAuth
  └── 动态工具列表变更：ToolListChangedNotification → 刷新缓存
```

### 设计思想

1. **Effect-TS 纯函数式工具**：工具的 execute 返回 `Effect`，天然支持取消、重试、依赖注入
2. **Plugin 可修改工具定义**：通过 `Plugin.transform(tool.definition)` 钩子在工具发送给 LLM 前动态调整
3. **MCP 名称清理**：`serverName_toolName` 格式，去除特殊字符
4. **自定义工具文件扫描**：从 `{tool,tools}/*.{js,ts}` 文件加载，零配置

---

## 4. Hermes Agent：自发现单例注册表 + 工具集分组

**核心文件**：`tools/registry.py`、`model_tools.py`、`toolsets.py`

### 大白话

Hermes Agent 的工具箱像一个**自带"自动盘点"功能的分区仓库**。

——**发现流程**：仓库管理员（`ToolRegistry`）不需要靠员工主动报到——每天早上他拿着一张 AST 扫描仪在 `tools/` 走廊走一圈，扫描仪会自动识别每扇门上有没有贴"我注册了工具"的标签（`registry.register()`）。有标签的门才推开进去看，空白房间直接跳过。标签上还写了"我属于 web 区"或"我属于 coding 区"——这叫工具集（`Toolset`），管理员可以一键开关整个区域（`enabled_toolsets: ["web"]`）。某些工具门口还有感应灯（`check_fn`，TTL 30 秒），灯灭了说明工具不可用（比如 Docker 没开），管理员就把它从当天给 LLM 的目录中划掉。

——**选取和使用流程**：工具调度时分两路。有一些"内部事务"工具——比如记笔记（memory）、查日程（session_search）、分派任务给下属（delegate_task）——这些不走通用调度台，而是直接由 Agent 自己处理。因为它们需要看 Agent 的私人笔记本（内部状态），普通工具没这个权限。其他工具走通用调度（`registry.dispatch`）——按名字找到工具，是异步的就走异步通道（`_run_async`），是同步的就直接执行。

——**外来工具接入**：外部供应商（MCP 服务器）被分配在专门的 `mcp-<供应商名>` 区域。后台有一个 asyncio 事件循环维持着和各家供应商的长连接，供应商通知"货品目录更新了"（`tools/list_changed`）仓库自动刷新。外来货可以覆盖外来货，但不许覆盖自营货——你不能用一个外部工具替换掉内置的 BashTool。

### 设计概述

它的注册机制非常巧妙：启动时不靠任何配置文件来声明"有哪些工具"，而是扫描 `tools/` 目录下的所有 `.py` 文件，用 AST 解析每个文件是否调用了 `registry.register()`——只有真正注册了工具的文件才会被 import。这避免了无意义的模块加载，同时保持了"零配置"的开发体验：写一个新工具文件，在模块顶层调 `registry.register()`，就会被自动发现。

工具的组织方式采用**工具集（Toolset）**分组。每个工具在注册时声明自己属于哪个 toolset（如 `"web"`、`"coding"`、`"mcp-github"`），用户只需一行配置 `enabled_toolsets: ["web", "coding"]` 就能控制整个工具组的开关。工具集还可以嵌套引用——`"coding"` 可能包含 `["bash", "read", "write", "edit"]`——形成层次化的工具权限管理。

工具调度时有一个关键的**分流逻辑**：Agent 级工具（todo、memory、session_search、delegate_task）直接由 `AIAgent` 类自己处理，不走通用注册表。原因是这些工具需要访问 Agent 的内部状态（会话上下文、记忆管理器等），而通用注册表的工具是无状态的纯函数。这种"核心工具直接处理 + 通用工具走注册表"的分层调度，避免了注册表被迫承载它本不应该知道的 Agent 内部状态。

每个工具还有一个 `check_fn`（可用性检测函数），结果以 30 秒 TTL 缓存。例如 Docker 工具会检测 Docker 守护进程是否在运行，Playwright 工具会检测浏览器是否安装。未通过检测的工具自动从 LLM 可见列表中移除，而不是报错——这避免了 LLM 尝试调用一个根本用不了的工具。

MCP 工具以 `toolset="mcp-<servername>"` 格式注册，通过后台的 asyncio 事件循环连接 MCP 服务器。当收到 `tools/list_changed` 通知时，工具列表会动态刷新。MCP 工具可以覆盖 MCP 工具，但 MCP 不允许覆盖 CLI 内建工具——确保核心工具的安全性。

### 自发现注册模式

```python
# 每个 tools/*.py 文件在模块级别自注册：
registry.register(
    name="web_search",
    toolset="web",
    schema={...},
    handler=search_web,
    check_fn=lambda: has_api_key(),
    requires_env=["SEARCH_API_KEY"]
)
```

启动时 AST 解析每个 `.py` 文件，只 import 那些调用了 `registry.register()` 的文件——无浪费的导入。

### 工具集（Toolset）分组

```
toolsets.resolve_toolset("web") → [web_search, web_fetch, ...]
toolsets.resolve_toolset("coding") → [bash, read, write, edit, ...]
toolsets.resolve_toolset("mcp-github") → [github_search, github_pr, ...]
```

### 工具调度链

```
AIAgent._execute_tool_calls()
  │
  ├── Agent 级工具（todo, memory, session_search, delegate_task）
  │   → AIAgent 直接处理（需要 agent 状态）
  │
  ├── Context engine 工具（lcm_grep 等）
  │
  └── model_tools.handle_function_call()
        ├── coerce_tool_args()   类型强制（string→int）
        ├── Plugin hook: pre_tool_call（可阻断）
        ├── registry.dispatch(name, args)
        │     ├── 查找 ToolEntry
        │     ├── is_async? → _run_async(coro)
        │     └── handler(args, **kwargs)
        └── Plugin hook: post_tool_call
```

### 可用性检查

每个工具有 `check_fn`（TTL 缓存 30 秒）——例如检测 Docker 守护进程是否运行。未通过的工具从 LLM 可见列表中移除。

### 设计思想

1. **自发现 = 零配置**：添加工具只需创建 `.py` 文件并调用 `registry.register()`
2. **工具集分组**：`enabled_toolsets: ["web", "coding"]` 一行配置控制整个工具组
3. **TTL 缓存的 check_fn**：30 秒缓存平衡了准确性和性能——避免每轮都探测 Docker/Playwright

---

## 5. CrewAI：BaseTool + 双通道工具执行

**核心文件**：`tools/base_tool.py`、`tools/structured_tool.py`、`tools/tool_usage.py`

### 大白话

CrewAI 的工具箱像一个**正在从"人工传话"升级到"数字对讲机"的施工队**。

——**发现流程**：工具不是固定分配给工人的。每天早上出工前，工头（`Crew._prepare_tools()`）会根据当天的任务动态打包工具箱——工人自带的工具 + 需要派活给同事的通讯器 + 外部供应商名录 + 平台总部远程接口 + 记忆笔记本（如果需要记东西）+ 文件工具（如果今天有材料要看）→ 合成一个当天专属的工具包。同一个工人做不同的活，背的工具包可能完全不同。

——**选取和使用流程**：施工队目前有两套通讯方式。旧方式是**人工传话**——工头在指令书里描述每个工具长什么样，工人看完指令书后在一张纸条上写"Action: search\nAction Input: {query: xxx}"，然后跑回工头那里。但工人字迹潦草，工头得用 4 种方法（JSON → ast.literal_eval → JSON5 → 修复破损 JSON）反复辨认，再用模糊比对（`SequenceMatcher, 0.85`）猜测"他写的到底是 web_search 还是 websearch"。新方式是**数字对讲机**——工人直接在对讲机上按标准频率（OpenAI tool schema）发指令，工头收到后分派给 8 个伙计（`ThreadPoolExecutor, max_workers=8`）同时干活。新老两套并存，但新工地已经默认用对讲机了。

——**一条聪明的捷径**：有些工具的结果本身就是要交的活。比如工人让计算器算了个数，算出来的结果就是最终答案，不需要再跑回工头那里让工头转述一遍。`result_as_answer` 标志就是标记这种"结果即是答案"的工具——少跑一趟路，省时又省钱。

### 设计概述

CrewAI 的工具系统体现了从旧架构向新架构演进的痕迹。它目前同时维护着两条工具执行通道：**ReAct 路径**（旧版）和**原生工具路径**（新版），两者在 `AgentExecutor._invoke_loop()` 中通过"LLM 是否支持原生函数调用"来分流。

ReAct 路径的工作方式是将工具描述嵌入到 prompt 文本中，LLM 会输出类似 `Action: tool_name\nAction Input: {...}` 的文本，然后系统用 `ToolUsage.parse_tool_calling()` 来解析这段文本。因为 LLM 输出的文本格式不稳定，解析器采用了一个**级联回退策略**：先尝试标准 JSON 解析，失败则尝试 `ast.literal_eval`，再失败则用 JSON5，最后尝试修复破损的 JSON。这是该路径最脆弱但也最能容忍 LLM"不规范输出"的设计。同时，为了进一步容错，它通过 `SequenceMatcher(threshold=0.85)` 做**模糊名称匹配**——LLM 把 `web_search` 写成 `web-search` 或 `websearch` 时仍然能正确匹配到目标工具。

原生工具路径则将工具转换为 OpenAI 的标准 tool schema（`convert_tools_to_openai_schema()`），直接传给 LLM 的 `tools` 参数。LLM 返回结构化的 `tool_calls`，系统用 `ThreadPoolExecutor(max_workers=8)` 并行执行。这条路径更干净、更可靠，但要求 LLM 支持原生的函数调用 API。

工具分配机制有一个值得注意的设计：工具不是静态绑定到 Agent 的，而是在 `Crew._prepare_tools()` 中**动态组装**的。Agent 的显式工具 + 委派工具（如果允许委派）+ MCP 工具 + 平台 API 工具 + 记忆工具（如果启用记忆）+ 文件工具（如果有附件）→ 合并去重。这意味着同一个 Agent 在不同任务中可能拥有不同的工具集。

`result_as_answer` 标志是一个聪明的优化：当某个工具的结果显然就是最终答案时（例如计算器返回最终数字、翻译工具返回完整译文），系统会**跳过一次 LLM 调用**，直接将工具结果作为最终输出返回。这对减速和降费都有帮助。`cache_function` 则提供了**选择性缓存**：默认所有工具结果被缓存，但工具可以自定义策略（例如缓存搜索结果但设置过期时间、完全不缓存实时数据等）。

### 工具定义

```python
class BaseTool(BaseModel):
    name: str
    description: str
    args_schema: Type[BaseModel]   # Pydantic 参数模型
    cache_function: Callable       # 缓存策略
    max_usage_count: int           # 使用次数上限
    result_as_answer: bool         # 工具结果即最终答案？

    def _run(self, **kwargs) -> str: ...     # 同步
    async def _arun(self, **kwargs) -> str: ... # 异步
```

### 工具分配

```
agent.tools (显式分配)
  + 委派工具（如 allow_delegation）
  + MCP 工具
  + 记忆工具（如 memory=True）
  + 文件工具（如有附件）
  │
  └── _merge_tools() 去重（按名称）
        └── _parse_tools() 转换为 CrewStructuredTool
```

### 双通道执行

```
LLM 返回工具调用
  │
  ├── ReAct 路径（旧版）：
  │     工具描述嵌入 prompt 文本
  │     LLM 输出 "Action: tool_name\nAction Input: {...}"
  │     → ToolUsage.parse_tool_calling()  解析文本
  │     → 尝试 JSON → ast.literal_eval → JSON5 → 修复的 JSON
  │
  └── 原生工具路径（新版）：
        工具转换 OpenAI schema
        LLM 返回结构化 tool_calls
        → 并行执行（ThreadPoolExecutor x8）
```

### 设计思想

1. **`result_as_answer` 捷径**：当工具结果显然就是最终答案时（如计算器），跳过一次 LLM 调用
2. **选择性缓存**：`cache_function` 可由工具自定义策略（如缓存搜索结果但不过期）
3. **模糊名称匹配**：通过 `SequenceMatcher(threshold=0.85)` 容忍 LLM 的轻微命名错误

---

## 6. OpenClaw：声明式可用性 + 四种工具来源

**核心文件**：`src/tools/types.ts`、`tools/planner.ts`、`tools/availability.ts`

### 大白话

OpenClaw 的工具箱像一个**有"自动安检门"的国际物流中心**。

——**发现流程**：这个仓库里有四种来源的货：自营货（`core:`）、品牌商供货（`plugin:`）、快递通道专用货（`channel:`，比如 WhatsApp 发消息、Telegram 回复）、海外代购货（`mcp:`）。但不管货从哪来，进仓库前全部被装进一模一样的标准集装箱（统一的 JSON Schema），贴上标签（`ToolExecutorRef`）。收货台（`buildToolPlan()`）每天早上把所有集装箱按标签排序，逐个过**安检门**（`evaluateToolAvailability()`）。安检门检查的不是"这个工具是否存在"，而是"今天这个工具能不能正常运转"。安检规则不是写在代码里的 if-else，而是一本公开的安检手册（声明式配置），上面写着"如果没 API 密钥 → 不放行""如果环境变量 X 没设置 → 不放行""如果插件未激活 → 不放行"。过得了就放进 visible 区（交给 LLM 的目录），过不了就放在 hidden 区（保留记录但不展示）。任何人想看安检规则都可以翻这本手册——审计变得非常简单。

——**选取和使用流程**：LLM 拿到的目录上，每个集装箱的描述完全一样——不标产地（`core:` vs `mcp:`），不标运输方式。LLM 只管说"我要用这个工具"，仓库调度系统自动找到对应的集装箱、找到正确的搬运方式。对 LLM 来说，所有工具看起来都是一个样子的黑箱。

——**安全设计**：某些货箱上贴着特殊标签——"老板本人才能开"（Owner-only）、"开箱前需安检员审批"（exec approval: `always` / `on-miss` / `never`）、"只有白名单上的员工能碰"（allowlist）。这些标签贴在箱子上而非写在流程手册里——安检门在处理每个箱子时直接读取标签，而不是执行一堆分散的 if 语句。

### 设计概述

OpenClaw 的工具系统最与众不同的地方在于它**将工具可用性从代码逻辑中彻底剥离**，变成了一套声明式配置。传统做法是到处散落 `if has_api_key and docker_running and config_enabled` 这样的条件判断。OpenClaw 的做法是：每个工具的元数据中嵌入一个 `availability` 表达式（`{ kind: "allOf", conditions: [...] }`），由 `evaluateToolAvailability()` 递归评估。表达式的节点类型包括 `always`、`auth`（需要某 Provider 的 API Key）、`config`（需要某配置项）、`env`（需要某环境变量）、`plugin-enabled`（需要某插件激活）、`context`（需要上下文满足条件），以及组合节点 `allOf` 和 `anyOf`。

这套系统的好处是：管理员可以通过配置文件精确控制每个 Agent 的工具权限，不需要改任何代码；审计时也一目了然——所有工具的可用条件都是可读的表达式树，而不是分散在几十个文件中的 if 语句。

工具来源方面，OpenClaw 定义了四种互不重叠的来源——`core:`（内置）、`plugin:`（插件注册）、`channel:`（通道消息操作，如 WhatsApp 发消息、Telegram 回复）、`mcp:`（外部 MCP 服务器）。四种来源通过统一的 `ToolExecutorRef` 命名空间（如 `mcp:my-server:getDatabaseData`）隔离，但在 LLM 看来完全一样——都是相同的 JSON Schema 描述。`buildToolPlan()` 在每轮开始时遍历所有来源的工具描述符，评估 availability、按 `sortKey + name` 排序、分为 `visible`（注入 system prompt）和 `hidden`（保留诊断信息但不发送给 LLM）两组。

安全模型也非常声明式：Owner-only 标记、exec 审批模式（`always` / `on-miss` / `never`）、工具允许名单都定义在工具元数据中，在工具计划生成时就参与决策。例如，一个标记为 `{"execApproval": "on-miss"}` 的 Bash 工具，如果命令不在预设白名单中，会触发审批流程——但这个行为是工具定义的属性，不是执行层到处判断的结果。

### 四种工具来源

```
ToolExecutorRef（执行器引用）:
├── core:<executorId>         内置工具，Agent 进程内直接执行
├── plugin:<pluginId>:<name>  插件注册的工具
├── channel:<channelId>:<id>  通道提供的消息操作
└── mcp:<serverId>:<toolName> 外部 MCP 服务器
```

### 声明式可用性

```typescript
// 递归评估声明式表达式，而非命令式代码
evaluateToolAvailability(expr):
  ├── kind: "always"              永远可用
  ├── kind: "auth"                 需要某 Provider 的 API Key
  ├── kind: "config"               需要某配置项
  ├── kind: "env"                  需要某环境变量
  ├── kind: "plugin-enabled"       需要某插件激活
  ├── kind: "context"              需要上下文满足条件
  ├── allOf: [...]                 全部通过
  └── anyOf: [...]                 至少一个通过
```

### 设计思想

1. **声明优于命令**：工具的可用性、权限、审批模式全是声明式配置，不是代码逻辑。管理员通过文件精确控制，新增工具不需要改代码
2. **来源无关的执行接口**：LLM 看到的全是相同的 JSON Schema，不关心工具是怎么实现的
3. **安全边界显式化**：Owner-only 标记、exec 审批、允许名单都定义在工具元数据中

---

## 7. Nanoclaw：双工具系统（SDK 内建 + MCP）

**核心文件**：`container/agent-runner/src/mcp-tools/server.ts`、`mcp-tools/core.ts`

### 大白话

Nanoclaw 的工具箱像一个**邮轮上的双舱系统**——大舱装通用工具，小舱装通讯工具。

——**发现流程**：这艘邮轮有两个完全不连通的货舱。主舱（SDK 内建工具）装的是 Agent 干活的通用工具：锤子、扳手、螺丝刀（Bash、Read、Write、Edit、Glob、Grep）。辅舱（MCP 自建工具）装的是船上的通讯设备：对讲机（send_message）、广播系统（send_file）、日程表（schedule_task）、船员名册（create_agent）。两个舱各自独立管理——主舱的货由 SDK 直接盘点，辅舱的货通过 `registerTools([])` 注册。新货进辅舱只需三步：造货 → 贴注册标签 → 在进货单（barrel 文件）上加一行。

——**选取和使用流程**：乘客（LLM）想用主舱的工具——走 SDK 内部通道，无需经过辅舱。乘客想用辅舱的通讯工具——走 MCP 管道（`mcp__nanoclaw__*`），管道上有两扇安全门：PreToolUse hook 检查"这个工具被禁了吗？"（`SDK_DISALLOWED_TOOLS`），通过后在服务端按名称查找、执行。执行完毕后 PostToolUse hook 清除执行标记（防止后续的超时检测误杀正常运行的长时间任务）。

——**精细的地址簿设计**：辅舱里有一个地址本（`destinations` SQLite 表），上面只写"Alice"→ `{Telegram, chat_id_123}`。Agent 发消息时只需要喊"给 Alice 发个消息"，不用关心 Alice 在 Telegram 还是 WhatsApp。这本地址本由船长（宿主端）亲自维护和校验——即使有人在辅舱偷偷修改了地址本，船长那边的权威版本也会在投递时拦截异常路由。地址本即 ACL。

### 设计概述

Nanoclaw 的独特之处在于它是一个**宿主-容器架构**——而它的工具系统被清晰地划分为两层，各自服务于不同的角色。第一层是 **SDK 内建工具**（Bash、Read、Write、Edit、Glob、Grep、WebSearch 等），由 Claude Code SDK 在容器内部直接管理。这些工具负责文件操作、代码搜索、命令执行等 Agent 的核心工作能力。Nanoclaw 通过 `TOOL_ALLOWLIST` 白名单控制哪些 SDK 内建工具对 Agent 可见，同时通过 `SDK_DISALLOWED_TOOLS` 黑名单屏蔽某些不适合自动化的工具（如 `CronCreate`、`AskUserQuestion`）。

第二层是 **MCP 自建工具**（send_message、send_file、schedule_task、ask_user_question、create_agent 等），由 Nanoclaw 自己的代码库定义，通过 MCP stdio 协议暴露给 SDK。这些工具负责**平台操作**——向用户发消息、调度周期性任务、创建子 Agent、安装包——即那些需要与宿主系统交互而非仅仅操作文件的能力。

两层之间的隔离非常干净：SDK 内建工具做"纯 Agent 工作"（读写文件、执行命令），MCP 自建工具做"平台工作"（发消息、调度任务、管理 Agent 生命周期）。这种隔离意味着即使换一个 Agent SDK（从 Claude 换成 Codex 或 OpenCode），平台工具层完全不受影响。

工具的注册采用的是**隐式注册模式**：每个工具模块在顶层调用 `registerTools([...])`，barrel 文件 `mcp-tools/index.ts` 通过副作用导入触发所有注册。添加新工具的步骤是：1) 创建工具模块文件 → 2) 调用 `registerTools()` → 3) 在 barrel 文件中追加一行 `import`。不需要修改任何中心化的注册表代码。

还有一个精致的设计是 **`destinations` 映射表**——一个由宿主写入 SQLite 的表，将 Agent 内部的"用户名"映射为实际的路由信息（channel_type、platform_id、thread_id）。Agent 在发送消息时只指定目标名称（如 `"@alice"`），不用关心 Alice 是通过 Telegram、WhatsApp 还是 Slack 连接的。这张表同时充当 ACL 的角色——宿主端的 `delivery.ts` 会重新验证每条消息的路由，即使容器内的映射表被篡改也不会影响安全性。

### 双层工具架构

```
工具分为两层：

1. SDK 内建工具（Claude Code SDK 提供）：
   Bash, Read, Write, Edit, Glob, Grep,
   WebSearch, WebFetch, Task, TodoWrite, Skill, ...

2. MCP 工具（Nanoclaw 容器自己暴露）：
   send_message, send_file, edit_message, add_reaction,
   schedule_task, list_tasks, ask_user_question,
   send_card, create_agent, install_packages, add_mcp_server
```

### 隐式注册模式

```typescript
// 每个工具模块在顶层调用 registerTools()
// mcp-tools/core.ts
registerTools([sendMessageDef, sendFileDef, editMessageDef, ...])

// mcp-tools/index.ts barrel 导入触发所有副作用
import './core.js'
import './scheduling.js'
import './interactive.js'
import './agents.js'

// SDK 启动 MCP 子进程，通过 stdio 暴露工具
startMcpServer() → StdioServerTransport
```

### 工具执行流程

```
Agent 决定使用工具
  │
  ├── SDK 内建工具 → Claude Code SDK 内部处理
  │
  └── MCP 命名空间工具 (mcp__nanoclaw__*)
        ├── PreToolUse hook: 检查 SDK_DISALLOWED_TOOLS
        ├── MCP 服务器调度: server.ts 按名称查找
        ├── 调用 handler(args)
        └── PostToolUse hook: 清除 in-flight 标记
```

### 设计思想

1. **隐式注册 = 最少样板**：添加工具只需 `registerTools([])` + barrel 文件追加一行 import
2. **SDK 内建 + MCP 自建两层隔离**：文件操作由 SDK 安全处理，平台操作（发消息、调度任务）通过自建 MCP 工具
3. **目标映射表**：`destinations` SQLite 表既是路由映射也是 ACL——宿主端重新验证所有发送操作

---

## 8. pi：TypeBox 模式 + 扩展注册

**核心文件**：`packages/agent/src/types.ts`、`packages/coding-agent/src/core/tools/`

### 大白话

pi 的工具箱像一个**极简瑞士军刀 + 可插拔扩展坞**。

——**发现流程**：军刀本身只提供最基础的工具——读写文件、执行命令、搜索代码——差不多够用就行（`createAllTools()`）。真正丰富的是扩展坞（Extension API）——第三方厂商可以设计任意模块插上去：`registerTool({ name, description, parameters, execute })` 四样东西交齐，工具就自动注册到池子里，和原厂工具对 LLM 来说没有任何区别。核心军刀的 `AgentTool` 接口只有 name、description、parameters、execute 四个字段——不要求你填写权限策略、不要求你提供 UI 渲染器、不要求你声明并发安全——因为核心军刀根本不在这些层面做假设。它可以在任何地方运行（浏览器、Node.js、Bun）而不挑环境。

——**选取和使用流程**：工具有一个极简的标志位 `executionMode: "sequential" | "parallel"`。Agent 循环在决定执行顺序时只看这个标志——所有标 `"parallel"` 的先进一批并发执行，`"sequential"` 的逐个排队。没有复杂的"可并行性判断函数"——工具作者自己最清楚自己的工具能不能并行跑，不需要系统来猜。

——**分层丰富**：军刀本身保持干净（`packages/agent` 核心包），但厂商设计的扩展坞模块（`packages/coding-agent` 层）可以给工具加各种"皮肤"——`renderCall` 定制工具调用时的 UI 显示、`renderResult` 定制结果渲染、`promptSnippet` 在 system prompt 里省钱（只贴一句话摘要而非完整描述）。核心和皮肤之间通过 `wrapToolDefinition()` 一键转换。

### 设计概述

pi 的工具系统体现了"最小可行抽象"的哲学。在 `packages/agent` 核心包中，`AgentTool` 接口只有四个字段：`name`（工具名）、`description`（LLM 可读描述）、`parameters`（TypeBox 模式）、`execute`（执行函数）。没有权限检查、没有 UI 渲染器、没有并发标记、没有可用性检测——这些全部留给上层 `packages/coding-agent` 去丰富。这意味着核心包可以在浏览器、Node.js、Bun 等任何环境中运行，因为它对工具的唯一假设是"有一个名字、一段描述、一个参数 schema、一个执行函数"。

在 `packages/coding-agent` 层，通过 `ToolDefinition`（扩展了 `AgentTool`）和 `wrapToolDefinition()` 适配器实现了**分层丰富**。`ToolDefinition` 增加了 `promptSnippet`（在 system prompt 中显示的单行摘要）、`promptGuidelines`（注入 system prompt 指南部分）、`renderCall` / `renderResult`（TUI 自定义渲染器）、`renderShell`（控制是否自绘 shell 样式）。`wrapToolDefinition()` 作为桥梁，将丰富的 `ToolDefinition` 转换为核心包能消费的最小 `AgentTool` 接口。

工具的扩展注册通过 Extension API 的 `registerTool()` 实现。第三方开发者可以创建一个 TypeScript 扩展文件，在工厂函数中调用 `registerTool({ name, description, parameters, execute })`。这个工具会被 AgentHarness 在启动时合并到工具池中，与内建工具对 LLM 来说完全一致。扩展还可以使用 `registerProvider()` 添加新的 AI Provider、用 `on("tool_call")` 订阅工具执行事件、用 `registerCommand()` 注册新的 slash 命令——但这些都是 Extension 层面的事，不是 Skill 层面的事。

工具执行的并行/顺序控制非常简洁：每个工具的 `executionMode` 可以是 `"sequential"` 或 `"parallel"`。循环在执行前评估——所有标记为 `"parallel"` 的工具在一个批次中并发执行，`"sequential"` 的工具按顺序逐个执行。没有复杂的"可并行性判断函数"，只是一个静态标志。

### 最小工具接口

```typescript
interface AgentTool<TParameters, TDetails> {
  name: string
  label: string
  description: string
  parameters: TSchema                 // TypeBox 模式
  prepareArguments?: (args) => TParameters  // 兼容性 shim
  executionMode?: "sequential" | "parallel"
  execute(id, params, signal?, onUpdate?): Promise<AgentToolResult>
}
```

### 扩展工具注册

```typescript
// 扩展通过 ExtensionAPI 注册工具
extension.registerTool({
  name: "my_tool",
  description: "...",
  parameters: Type.Object({ query: Type.String() }),
  execute: async (args, ctx) => { ... }
})
```

### 工具包装器

```
ToolDefinition (丰富的 UI 元数据)
  │
  └── wrapToolDefinition()
        └── AgentTool (最小可执行接口)
              │
              └── AgentHarness 使用 AgentTool[] 运行 Agent 循环
```

### 设计思想

1. **最小接口原则**：`AgentTool` 仅需 name、description、parameters、execute——四是足以
2. **分层丰富**：coding-agent 层添加 UI 渲染器（renderCall/renderResult），核心包保持纯净
3. **扩展可以注册工具**：Extension API 的 `registerTool()` 允许第三方添加工具，工具在 Agent 启动时合并

---

## 9. 横向对比总结

| 维度 | Claude-Code | OpenCode | Hermes Agent | CrewAI | OpenClaw | Nanoclaw | pi |
|------|------------|----------|-------------|--------|----------|----------|-----|
| **工具定义** | Zod + buildTool() | Effect Schema | Pydantic registry | Pydantic BaseTool | TypeScript + 声明式 | MCP Tool + SDK 内建 | TypeBox + AgentTool |
| **工具来源** | 内建 + MCP | 内建 + MCP + 自定义文件 | 自发现 .py + MCP + 插件 | 内建 + MCP + 记忆 + 文件 | core/plugin/channel/MCP | SDK 内建 + MCP 自建 | 内建 + 扩展注册 |
| **注册方式** | getAllBaseTools() 集中 | ToolRegistry 服务 | AST 解析 + register() | _TOOL_TYPE_REGISTRY | 声明式配置文件 | 隐式 registerTools() | createAllTools() + registerTool() |
| **权限模型** | canUseTool → allow/deny/prompt | Agent permission + session | check_fn TTL 缓存 | 无（Agent 拥有全权） | 声明式 availability | SDK 的 TOOL_ALLOWLIST | beforeToolCall() 阻断钩子 |
| **执行并行** | partitionToolCalls 读并写串 | AI SDK 并行 | 并行/顺序决策 | ThreadPoolExecutor x8 | 无显式并行 | N/A | executionMode 标志 |
| **MCP 处理** | assembleToolPool 合并 | MCP.Service 动态管理 | MCPToolResolver | MCPToolResolver | mcp: 前缀命名空间 | StdioServerTransport | 无原生 MCP |
| **核心创新** | 延迟加载 ToolSearch、工具即 UI | 插件可变换工具定义 | 自发现 AST 扫描、工具集分组 | result_as_answer 捷径、模糊匹配 | 声明式可用性递归评估 | 隐式注册、双数据库路由映射 | 最小接口、分层丰富 |


## 10. 对 Argo 的启示

1. **统一 Tool 接口**是首要设计任务——CLI 工具和 MCP 工具应归一化为同一个接口，LLM 不应感知实现差异
2. **自发现优于集中注册**：Hermes Agent 的 AST 扫描和 Nanoclaw 的 barrel 导入都很好，工具文件自己注册自己
3. **声明式可用性**值得借鉴：OpenClaw 的 `anyOf/allOf` 递归评估比到处散落的 if 语句更可审计
4. **分区执行**是关键优化：读操作并行、写操作串行——Claude-Code 的分区逻辑简单但效果显著
5. **MCP 命名空间**用 `mcp__<server>__<tool>` 格式，内建工具优先于同名 MCP 工具
6. **pi 的四要素最小接口**（name + description + schema + execute）是 Argo 设计 deck 接口的绝佳起点
