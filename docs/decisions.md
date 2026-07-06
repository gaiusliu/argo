# 技术决策

> 每条技术取舍以 DEC-xxx 编号，append-only。废弃标注替代者，不删除历史。

## DEC-001：grep 工具 MVP 阶段使用纯 Go 实现

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：grepHandler 使用纯 Go 实现（filepath.Walk + regexp），零外部依赖。

**背景**：主流 Coding Agent（OpenCode、Claude Code、KiloCode）的 grep 工具底层都调 `ripgrep`（rg），速度快、内置目录排除、跨平台兼容。但 rg 是外部二进制，用户需自行安装。

**选择**：MVP 阶段不引入额外依赖。纯 Go 实现在小至中型仓库性能足够。接口和参数 schema 固定——将来如果遇到大仓库性能瓶颈，内部替换为 `exec.CommandContext(ctx, "rg", ...)` 即可，helm 侧无感知。

**替代方案**：直接集成 ripgrep。因引入外部依赖被否决。

## DEC-002：CLI 工具参数发现不通过 Schema 方法，由 LLM 自行 --help

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：不在 Tool 接口中添加 `Schema()` 方法。CLI 工具的参数由 LLM 通过 `--help` / `-h` 自行发现，system prompt 中提示 LLM 先运行 `<tool> --help` 查看参数。

**背景**：Agentic Search 设计下，LLM 通过 `list` 发现工具、`lookup` 查看元数据。如果 `lookup` 不能返回参数 schema，LLM 需要另一种方式获取参数契约。两个方向：A) 在 Tool 接口加 `Schema()` 方法，每个工具返回参数定义；B) 利用 CLI 工具自带的 `--help`。

**选择**：方案 B。`--help` 是 CLI 工具的天然参数文档，比手写 schema 更准确、零维护成本。内置工具的参数已在 description 中暗示，LLM 足够推断。具体提示文案交给 helm 模块在拼 system prompt 时处理。

**替代方案**：在 Tool 接口添加 `Schema() map[string]any`。因增加接口复杂度、CLI 工具 schema 可能和实际 `--help` 不同步而被否决。

## DEC-003：工具输出截断使用 HeadTail 策略，后续支持可配置

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：MVP 阶段写死 HeadTail 截断策略（30K 字符上限 + 16K 保留）。超出时截断输出，完整原文写入临时文件，返回截断结果给 LLM。store 失败不影响截断（仅不附带原文路径）。

**背景**：工具输出可能超出上下文窗口。参考 Claude Code（30K 字符）、Kilo Code / OpenCode（50KB）、Codex（10KB），选 30K 为默认上限。

**选择**：HeadTail 策略——保留头尾各 8K 字符，中间插入 `... (truncated)...` 标记。截断后结果直接返回 LLM，原文写入 `os.TempDir()/argo-truncation/`。截断文件保留 7 天，文件名内嵌毫秒时间戳（格式 `argo-<ts_ms_hex>-<random_hex>`），清理时从文件名解码时间戳判断过期——比依赖文件 mtime 更可靠，不受 touch/复制影响。

**未来方向**：支持 `head`、`tail`、`head_tail` 三策略可配置，以及自定义阈值。

## DEC-004：注册容错策略——内置严格、CLI 宽松

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：`RegisterTools()` 内置工具注册遇错即停（代码 bug 应立即暴露），CLI 工具校验失败和注册冲突跳过当前工具、继续注册后续，所有错误收尾汇总返回。

**背景**：原实现中 CLI 工具校验不通过或注册冲突即 `return err`，导致所有未注册的 CLI 工具全部丢弃。用户可能只配错了一个工具，却导致所有外部工具不可用。

**选择**：内置工具仍保持严格（单个失败即 panic 级别的 bug），CLI 工具改为宽松——validateCLIEntry 失败或 RegisterTool 冲突时，将该工具错误记入 `skipped` 列表，`continue` 注册下一个。全部注册完后若 `len(skipped) > 0` 则汇总返回。

## DEC-005：System prompt 四块结构——行为核心 → 环境 → 工具 → 用户指令

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：system prompt 由四部分组成，按固定顺序拼接：行为核心（身份声明）→ 环境信息（session 不变项）→ 工具列表（deck 遍历）→ 用户指令（AGENTS.md）。

**背景**：调研 OpenCode（四模块并行拼接）、Pi（单体函数分段拼接），两者都将 prompt 拆分为独立模块。Argo 的 helm 是唯一知道所有片段如何排列的模块。

**选择**：四块顺序固定。行为核心用常量（MVP 只提供 coding 风格，后续加入通用风格），环境信息和工具列表运行时生成，AGENTS.md 读不到即跳过。

## DEC-006：环境信息只放 session 级不变项

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：system prompt 的 `<env>` 块只包含 Platform、Today's date、Global config 三个字段。Working directory 和 Git repo 状态不放——它们会随用户操作变化，放 user message 更合适。

**背景**：OpenCode 在 env 块中包含 wd、workspace root、git status 全部信息；KiloCode 将 wd 和编辑器上下文挪到 `<environment_details>` user message 块，避免破坏 prompt cache。Argo 没有 prompt cache，但遵循同样的原则——system prompt 应该是一个 session 内不需要重新构建的东西。

**选择**：三个不变项。Model ID 也不放——Argo 支持会话内切换模型。

## DEC-007：ToolRegistry 封装——保留包级 API 的向后兼容

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：将包级全局变量 `regMu`/`tools`/`toolsIdx` 封装为 `ToolRegistry` 结构体，同时保留 `List()`/`Lookup()`/`RegisterTool()`/`RegisterTools()` 包级便捷函数，委托到 `defaultRegistry` 单例。暴露 `NewToolRegistry()` 供测试创建独立实例。

**背景**：包级全局可变状态导致：测试间状态污染不支持 `t.Parallel()`、无法创建多实例（如热重载场景）、并发测试数据竞争。

**选择**：折中方案——不追求完全消除全局状态（会破坏所有调用方），而是用结构体封装并发原语，包级函数保持向下兼容。测试可通过 `NewToolRegistry()` 获得完全隔离的注册表。

## DEC-008：knot 包——跨模块流转类型的独立包

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：创建 `src/knot` 包，存放不属于任何单一模块但在核心流程中跨模块流转的类型——Message、Event、ChatRequest、TokenLimit、TokenUsage、ToolUse、DonePayload。

**背景**：helm 和 sail 各自定义了一套几乎相同的类型。拆共享类型时考虑了 `core`（语义太重，像核心代码），最终选择 `knot`（绳结）——符合 Argo 航海隐喻体系，小、不起眼但不可或缺。

**选择**：knot 单向依赖 deck（ToolUse.Result 引用 `*deck.ToolResult`），不形成循环依赖。deck 作为叶子模块不依赖 knot。

## DEC-009：模型选择单一来源——TurnState.ModelName

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：模型选择权集中在 `TurnState.ModelName` 一个字段。空字符串 = sail 取 config.json 默认模型。不引入 opencode 的多层优先级链（PromptInput → Agent → Command → Session → Config）。

**背景**：模型来源有四种场景：全局配置默认、CLI --model 启动覆盖、/model 命令切换、子代理定义文档指定。opencode 用六层优先级链处理，Argo MVP 阶段只需一个字段。

**选择**：TurnState.ModelName 承载当前会话模型名。三个覆盖场景都在创建/更新 TurnState 时写入，helm 内部零逻辑。

**替代方案**：多层优先级链。MVP 阶段过度设计。

## DEC-010：模型配置三层 Provider 结构

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：config.json 中模型配置采用三层结构——`provider → provider_id → models → model_id`。Provider 级配置 env/options，Model 级配置 name/reasoning/limit。不使用 npm 字段（Go 无法使用 TS AI SDK 的 npm 包，走 HTTP API 或 import Go SDK）。

**背景**：参考 opencode 和 kilocode 的 provider 配置结构，去掉了 Go 不适用的 npm/AI SDK 相关字段，保留 env、options、models 核心结构。ModelLimit 字段全部为可空指针，nil = 使用 API 默认值。

## DEC-011：reef 裁剪失败降级

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：reef.Compact() 出错时不阻塞循环，使用原始消息继续。helm 只在校验 nil 错误后替换 Messages。

**背景**：上下文裁剪是优化而非必须功能。裁剪失败（网络问题导致摘要生成失败、token 估算异常等）不应打断用户正常工作流。

## DEC-012：WebFetch MVP 阶段不限制跨 host 重定向

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：MVP 阶段 WebFetch 使用 Go 标准库 `net/http` 的默认重定向行为（自动跟随 301/302/303/307/308，不限 host），不实现自定义 `CheckRedirect`。

**背景**：Claude Code 的 WebFetch 有跨 host 重定向保护——只允许同 origin 和 `www.` 前缀变化的自动重定向，跨 host 的返回重定向信息让 Agent 用新 URL 显式再调用。目的是防止攻击者利用信任域名的开放重定向漏洞洗白恶意内容（如 `docs.python.org?url=evil.com` → 302 → evil.com，Agent 以为内容来自 docs.python.org）。KiloCode/OpenCode 同样未在 webfetch 中显式处理此场景。

**选择**：Argo MVP 先不处理。理由：① 安全攻击面在 Argo 的初期阶段几乎不存在；② Go 的 `http.Client` 默认最多跟 10 次重定向，不会无限循环；③ 后续需增强时在 `http.Client.CheckRedirect` 回调中加 host 校验即可，接口不变。

**未来方向**：添加 `CheckRedirect` 实现同 origin 限制策略。

## DEC-013：WebFetch MVP 阶段不设置 User-Agent

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：MVP 阶段 WebFetch 不显式设置 User-Agent 头，使用 Go `net/http` 默认值（`Go-http-client/1.1`）。

**背景**：Cloudflare 等 CDN 基于 TLS 指纹 + User-Agent 一致性做 bot 检测。Go 的 `net/http` TLS 指纹与浏览器完全不同。如果手动设置 `User-Agent: Chrome/143` 伪装浏览器，TLS 指纹和 UA 不一致会触发欺骗惩罚，反而更容易被拦截。保持默认的诚实身份（Go 自曝）不会产生这种不一致。

**待定方向**：可能设 `User-Agent: argo/0.1` 明确身份，可能完全不设，也可能后续由用户通过配置指定。取决于实际遇到的网站兼容性问题。

**替代方案**：伪装浏览器 UA。因和 TLS 指纹不一致可能适得其反而暂不采用。

## DEC-014：WebFetch MVP 阶段固定输出纯文本，不做 HTML→Markdown 转换

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：MVP 阶段 WebFetch 输出固定为纯文本——HTML 正文通过正则去标签提取，不引入 Markdown 转换。不提供 `format` 参数让用户选择输出格式（参考 Claude Code 做法：固定输出，不选择；OpenCode 做法：提供 text/markdown/html 三选一）。

**背景**：HTML→Markdown 转换需要引入外部 Go 库或 CLI 工具。两个候选 Go 库：`github.com/JohannesKaufmann/html-to-markdown`（最流行，可插拔规则，类似 TurndownService）和 `github.com/lunny/html2md`（轻量，API 简单）。另可走 CLI 工具注册机制调用 `pandoc`（功能最强但体积大 ~100MB）。

**选择**：当前纯文本够用——LLM 对纯文本的理解和 Markdown 几乎一样好。后续需要时引入 `html-to-markdown` Go 库，在 deck 内置工具集迭代中单独规划。

**未来方向**：加 `format` 参数支持 text/markdown 选择。

## DEC-015：WebSearch MVP 阶段只支持 JSON 响应解析，暂不处理 SSE

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：MVP 阶段 `callSearchMCP` 只做 `json.Unmarshal` 解析 MCP 响应，不处理 SSE（Server-Sent Events）格式。

**背景**：MCP 协议允许服务端以 SSE 格式返回流式响应（`Content-Type: text/event-stream`，一行一个 `data:` 前缀的 JSON）。OpenCode/KiloCode 的 `parseResponse` 做了双重容错——先整体 JSON 解析，失败后逐行扫描 `data:` 前缀提取内容。当前 Parallel 和 Exa 的 MCP 端点实际返回的是标准 JSON，SSE 容错并非硬需求。

**选择**：MVP 先不处理。如果后续搜索端点变更返回格式导致 JSON 解析失败，再加 SSE 容错逻辑。

**优先级**：低于 DEC-017（授权/询问能力优先于 SSE 解析）。

## DEC-016：WebSearch MVP 阶段只暴露 query 参数

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：MVP 阶段 WebSearch 只接受 `query` 一个参数。`numResults`、`type`、`livecrawl`、`contextMaxCharacters` 等控制在 handler 内部写死默认值。

**背景**：OpenCode 的 `websearch` 暴露了五个参数——`query`、`numResults`（默认8）、`type`（auto/fast/deep）、`livecrawl`（fallback/preferred）、`contextMaxCharacters`（默认10000）。这些参数让 LLM 能根据搜索场景调优（如快速浏览用 fast，深度研究用 deep）。Argo webSearchHandler 内部写死 `type=auto, numResults=8, livecrawl=fallback`，LLM 无法控制。

**选择**：保持参数极简。理由：① MVP 阶段先验证搜索能力可用，默认参数已覆盖大多数场景；② 参数越多 LLM 的 system prompt 越冗长；③ 后续如果需要暴露参数，在 Tool 接口不变的前提下在 handler 内部提取即可。

**未来方向**：按需暴露 `type` 和 `numResults`，`livecrawl` 和 `contextMaxCharacters` 用户极少需要调整。

## DEC-017：deck MVP 阶段无权限/授权机制

**状态**：❌ 已废弃（替代者 [DEC-027](#dec-027brig-规则引擎门控设计))

**日期**：2026-07-03

**决策**：MVP 阶段 deck 不提供工具执行前的权限确认机制。所有内置工具直接执行，不改动现有 Tool 接口。

**背景**：OpenCode/KiloCode 的权限系统通过 `ctx.ask()` 在工具执行前弹出确认框——WebFetch 按 URL 匹配规则、WebSearch 按 query 匹配。Claude Code 有 `canUseTool → allow/deny/prompt` 三级模型。Argo 的 deck 当前没有任何中断机制，所有 handler 被调用即执行。

**选择**：MVP 暂不加。权限系统涉及 Tool 接口扩展（需在 Execute 签名中加入 ask 回调或中断信号）、用户交互通道（CLI/网关模式不同）、规则持久化（配置文件或 session 级记忆），范围超出 deck 单模块。

**未来方向**：在 deck 或 helm 层引入 ask 机制，参考 OpenCode 的 `ctx.ask({permission, patterns, always})` 模式。**优先于 SSE 解析等网络层增强。**

## DEC-018：WebSearch 使用免费 MCP 端点，不自建爬虫

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：WebSearch 通过 Parallel（`https://search.parallel.ai/mcp`）和 Exa（`https://mcp.exa.ai/mcp`）两个免费 MCP 端点实现搜索，不采用 DuckDuckGo 等 HTML 爬虫方案。

**背景**：实现网页搜索有三种路线——A）自写爬虫抓取搜索引擎 HTML 结果页，B）调用付费搜索 API（博查、Tavily 等），C）调用免费 MCP 端点。A 的脆弱性在于目标网站改版即挂，B 需要用户注册拿 API Key 违背零配置原则，C 的 Parallel 和 Exa 均提供无需鉴权的免费 tier。

**选择**：方案 C。两个端点零配置、免费、结构化输出。无国内方案（国内无类似免费免 Key 的搜索 API）。

**替代方案**：DuckDuckGo HTML 爬虫。因维护成本高、反爬风险被否决。

## DEC-019：WebSearch 直接 JSON-RPC 调用，不引入 MCP SDK

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：`callSearchMCP` 直接构造 JSON-RPC 2.0 请求体（`tools/call`），POST 到 MCP 端点 URL，不引入完整的 MCP 协议栈。

**背景**：KiloCode/OpenCode 在通用 MCP 工具场景使用官方 MCP SDK 做完整协议握手（initialize → tools/list → tools/call），但在 WebSearch 场景特意绕过了这套基础设施——`mcp-websearch.ts` 只 97 行、仅 import `effect` 和 `effect/unstable/http`。原因：对于已知的固定 MCP 端点，`initialize` 握手和 `tools/list` 动态发现是多余的——端点的工具名已知为 `web_search`、传输方式是 Streamable HTTP。

**选择**：Argo 沿用同样的思路。`callSearchMCP` 纯 Go 标准库实现（`net/http` + `encoding/json`），约 45 行，零外部依赖。

**替代方案**：引入完整 MCP 客户端。因增加复杂度、为已知端点不需要握手的场景过度设计而被否决。

## DEC-020：WebSearch 分流使用 query hash 而非 session ID

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：WebSearch 在 Parallel 和 Exa 两个端点间使用 `fnvHash(query) % 2` 做确定性分流。

**背景**：KiloCode 使用 `checksum(sessionID) % 2` 分流——同一 session 始终用同一 provider，会话内搜索结果风格一致。Argo 的 deck handler 签名是 `func(ctx, params)`，没有 session ID 传入。Tool 接口的 `Execute` 方法只接受 context 和参数 map，无法获取会话标识。

**选择**：用 query 字符串的 FNV-1a 哈希做分流。同一 query 始终走同一 provider（确定性），近似实现负载分散。

**未来方向**：如果 Tool 接口扩展支持传入 session 上下文，切换为 session-based 分流。

## DEC-021：WebFetch MIME 分发策略

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：WebFetch 按响应的 `Content-Type` 头做 MIME 分发——`image/*` 转 base64 Data URL，`text/html` 用正则去标签提取纯文本，其余类型（纯文本、Markdown、JSON、XML、未知等）直接返回原文。不拒绝任何非 HTML 类型。

**背景**：OpenCode 的 WebFetch 有更精细的三路分发——图片转 base64 attachment、HTML 按 format 参数转 text/markdown/html、其余类型解码后直接返回。它不拒绝 PDF、ZIP 等二进制类型（直接 TextDecoder 解码返回乱码给 LLM）。

**选择**：沿用"不拒绝"策略。未知类型即使解码为乱码，LLM 自身会忽略。拒绝反而损失信息（如 `application/json` 返回的 API 数据正好是 LLM 需要的）。与 OpenCode 的差异：Argo 不做 format 参数（DEC-014）、不做 Markdown 转换（DEC-014）。

## DEC-022：Adapter 一份 SSE 实现 + 两个入口函数

**状态**：✅ 现行

**日期**：2026-07-04

**决策**：sail 的 adapter 层只有一份 SSE 解析实现（`parseSSE` + `convertDelta` + `buildOpenAIBody`），`OpenAIChat` 和 `CompatibleChat` 是两个薄入口函数，区别仅在于 `baseURL` 来源不同。不拆两套独立代码。

**背景**：handoff 中描述了两个 adapter——`openaiAdapter`（ChatGPT）和 `compatibleAdapter`（DeepSeek/MiniMax/Kimi/豆包）。但调研发现：OpenAI、DeepSeek、Kimi、MiniMax、豆包全部共用 OpenAI Chat Completions SSE 格式，协议完全一致。pi 的 provider 设计也是如此——每个 provider 只是一个配置对象（baseUrl + api + models），底层 API handler 是同一个 `openAICompletionsApi`。

**选择**：`adapter.go` 放共享逻辑（`doChat`、`parseSSE`、`convertDelta`、`buildOpenAIBody`），`openai.go` 和 `compatible.go` 各一个入口函数，分别内置 `https://api.openai.com` 和从 `Options["base_url"]` 取值。未来 OpenAI 加专属功能时，各自入口函数内部再分叉。

**替代方案**：两个 adapter 各自一套独立代码。因 MVP 阶段无分歧点、拆开等于维护同一段逻辑两个副本而被否决。

## DEC-023：`context.AfterFunc` 替代手动 goroutine 监听 ctx 取消

**状态**：✅ 现行

**日期**：2026-07-04

**决策**：`parseSSE` 中响应 ctx 取消不手动开 goroutine 监听 `<-ctx.Done()`，改用 `context.AfterFunc(ctx, body.Close)` + `defer stop()`。

**背景**：DEC-S02 要求 goroutine 在 ctx 取消时退出，不泄漏。常规做法是 `go func() { <-ctx.Done(); body.Close() }()`——但该 goroutine 会常驻等待，如果 scanner 先正常结束而 ctx 生命周期很长，goroutine 就一直挂着。

**选择**：`AfterFunc` 将回调挂载到 ctx 的取消传播链上（children map），ctx 取消时由取消发出方同步调用，不额外占用 goroutine。`defer stop()` 在 scanner 先结束时摘除回调，避免双 Close。Go 1.21+ 标准库内置，项目 Go 1.25 可用。

**替代方案**：
- `go func() { <-ctx.Done(); body.Close() }()` — goroutine 泄漏。
- `select { case <-ctx.Done() }` — scanner 阻塞在 `Read()` 期间无法响应。

## DEC-024：请求体 tools 字段先占位不填，待测试后决定

**状态**：✅ 现行

**日期**：2026-07-04

**决策**：`openAIReq` 结构体已加 `Tools []openAITool` 字段（`omitempty`），但 `buildOpenAIBody` 暂不填充。等测试后再决定是否填入 tool 列表。

**背景**：OpenAI API 的 `tools` 字段让模型以结构化 `tool_calls` 形式返回工具调用。但 Argo 的工具发现链路是 `List → Lookup → --help`（DEC-002），LLM 通过 system prompt 中的说明自行推断参数。加不加 `tools` 字段影响模型行为——加了就走 function calling，不加就走纯文本输出。

**选择**：先定义好类型占位，不影响流式链路（`streamState` 已能解析 `tool_calls` delta）。通过实际测试对比两种模式的表现再决定。

**替代方案**：
- 现在就填 `tools` —— 过早优化，可能和 DEC-002 的 `--help` 机制产生冲突。
- 不加字段——如果后续需要，改结构体 + 填逻辑耦合在一起，改动量大。

## DEC-025：模型元数据从 models.dev 运行时拉取，本地 JSON 缓存

**状态**：✅ 现行

**日期**：2026-07-05

**决策**：`Window()` 的 context window 数据从 `https://models.dev/api.json` 运行时拉取，缓存到 `~/.argo/metadata.json`。三级查找：用户配置 > models.dev 缓存 > 保守默认 128000。

**背景**：`Window()` 需要返回准确的 context window 供 reef 判断压缩时机。pi 用构建时脚本 `generate-models.ts` 从 models.dev + OpenRouter + Vercel AI Gateway + NVIDIA NIM 四个源拉数据，生成 22K 行的静态 TS 文件编译进二进制。Argo MVP 阶段不需要那么复杂。

**选择**：运行时方案。首次调用 `Window()` 时通过 `sync.Once` 触发——先读本地缓存立即可用，后台 goroutine 拉取 models.dev 覆盖更新。models.dev 免费、免认证、覆盖 20+ 厂商。查找时先精确匹配 key，再对模型名部分做前缀匹配（加 `-` / `.` 版本分隔符边界），防止 `gpt-4` 误匹配 `gpt-4o`。

**替代方案**：
- 构建时脚本生成 Go 源码 — 每次启动不能自动更新数据，需开发者手动跑 `go generate`。
- 完全硬编码内置表 — 手动维护，新模型需要发版。
- OpenRouter API — 需要 API Key 认证，违背零配置原则。

## DEC-026：6 类错误分类 + adapter 层短延迟重试

**状态**：✅ 现行

**日期**：2026-07-05

**决策**：`classifyError` 将 HTTP 响应分为 6 类（auth/quota/rate_limit/server_error/context_overflow/network），返回 `APIError{Retryable, RetryAfterSeconds}`。重试在 `doChat` adapter 层：网络/429/5xx 自动重试（3 次指数退避 1s/2s/4s），429 优先使用 Retry-After 头。5xx 不触发换模型——换模型需上层征求用户同意。

**背景**：kilocode 在 executor 层做短延迟重试（MAX_RETRIES=2, 500ms 基数），pi 在 agent-session 层做长延迟重试（baseDelay 可配置）。两者都在 adapter/executor 层做基本的可重试状态码处理，session/agent 层再做更复杂的 fallback 决策。

**选择**：adapter 层处理确定性的短延迟重试（网络瞬断、429 rate limit、5xx 临时故障），`Retryable` 标记由 `classifyError` 返回。不重试的（auth、quota、context_overflow）直接返回错误给 helm。换模型由上层决定（当前未实现）。

**替代方案**：
- 重试全部放 `Chat()` 层 — 需要改变 `doChat` 签名为同步 HTTP 调用 + 在 `Chat()` 中做重试循环，增加复杂度。
- 重试全部放 helm 层 — helm 需要解析 EventError、判断重试、重新调 `sail.Chat()`，循环控制复杂。

## DEC-027：brig 规则引擎门控设计

**状态**：✅ 现行

**日期**：2026-07-05

**决策**：新建 `brig` 包，在 helm.RunLoop 工具执行前插入权限审批门控。核心设计：(1) 三态裁决 Allow/Ask/Deny；(2) Rule 统一结构 `{Tool, Pattern, Action, Source}` 仅用于静态规则；(3) 用户确认 Ask 后以精确 `ToolCall` 指纹（工具名 + scopeKey 转换后的参数 + 工作目录）存入 `userApprovals` map，同指纹再次出现自动 Allow；(4) `AskUser` 回调代替 Approver 接口；(5) brig 独立于 deck，不侵入 Tool 接口；(6) 各 session 持有私有 Engine 实例，动态审批不跨 session；(7) 工作目录在 `knot.LoadConfig` 时通过 `os.Getwd()` 捕获并缓存，作为 `ToolCall` 指纹的一部分。

**背景**：DEC-017 判定 deck MVP 阶段不做权限机制，把审批留到后续迭代。kilocode 和 opencode 都用规则列表做"已审批记忆"——用户选 "always" 追加一条 allow 规则，而不是用 allowedDirs/allowedDomains 等独立缓存 map。这两个参考项目中权限系统均在 agent 循环层实现，不侵入工具接口。

**选择**：brig 在 helm 层做门控。RunLoop 签名扩展为 `RunLoop(ctx, state, ask, eng)`，ask 和 eng 为 session 级依赖注入。Evaluate 分两阶段——先遍历 5 层静态规则（denyRules → safeCommands → dangerCommands → interpreterCommands → sensitiveFiles），首命中即返回；静态规则返回 Ask 时再查 `userApprovals` map，命中则自动 Allow。`Approve` 以 `scopeKey` 处理后的指纹存入 map，不做泛化。Snapshot/Restore 预留接口，当前为空桩。

**替代方案**：
- 在 deck 层做权限（Tool 接口加 Ask 方法）— 侵入工具接口，所有工具实现都需改动。
- Approver 接口（含 Allow/Ask/Deny 方法）— 比 AskUser 重，kilocode/opencode 都用回调而非接口。

## DEC-028：`WorkingDir` 暂放在 Config 中，待 Session 模块提取

**状态**：✅ 现行

**日期**：2026-07-06

**决策**：工作目录 `Config.WorkingDir` 当前通过 `knot.LoadConfig` 中 `os.Getwd()` 获取并缓存，标记 `json:"-"` 不序列化。已知这是不正确的分层——`Config` 是 JSON 配置文件模型，工作目录属于运行时会话状态。待 Session 模块（vault/reef 之后的计划）建立后，将 `WorkingDir` 迁移到 Session 对象，`Config` 恢复为纯 JSON 派生模型。

**背景**：参考 pi、opencode、kilocode 三个项目，cwd 均存储在 Session 层而非 Config 层：

| 项目 | cwd 存放位置 | 传递方式 |
|------|-------------|---------|
| pi | `SessionHeader.cwd`（JSONL session 文件头部） | `SessionManager.create(cwd)` 构造时传入 |
| opencode | `session.directory` | `session.create()` 服务端自动捕获，返回时带 `directory` |
| kilocode | 同 opencode | `session.directory` 在 `SessionInfo` 中 |

Argo 当前尚无 Session 模块，且 `WorkingDir` 需要作为 `ToolCall` 指纹的一部分参与审批匹配。暂时的妥协是在 `Config` 中承载，后续迁移路径明确：`Config.WorkingDir` → `Engine.workingDir`（构造参数）→ 未来 `Session.WorkingDir`。

**替代方案**：
- 直接在 `Engine.NewEngine(workingDir)` 传入 — 语义正确，但 entry point（cmd/run）尚未开发，脚手架不完整，暂时不值得为这一个字段多写初始化代码。
- `sync.Once` 内缓存错误的设计 — 已知隐患：`os.Getwd()` 失败后 `globalErr` 永久非 nil，会导致 `brig.Evaluate/Approve` 访问 nil cfg panic。因为当前执行流中 `sail.Chat` 先于 brig 调用 `GetConfig()` 并检查错误，暂时受保护。Session 模块建立时一并修复。

## DEC-029：Event 模型简化——删除 DonePayload，平铺 Usage + 补齐生命周期事件

**状态**：✅ 现行

**日期**：2026-07-06

**决策**：删除 `DonePayload` 结构体，将 `TokenUsage` 平铺到 `Event.Usage` 字段；同时补齐 `EventStart` 和 `EventDone` 两个生命周期边界事件。

**背景**：原设计在 Event 上嵌套 `Done DonePayload`，其中 `DonePayload.Text` 存放最终文本回复。但流式事件链路是 `textStart → textDelta → ... → textDone`，消费者在 `done` 之前已通过所有 `textDelta` 收完全文。`Text` 字段是消费者自己已经持有的信息，helm 不该替消费者做拼装缓存。

**选择**：
- 删除 `DonePayload` 结构体（`Text` + `TokenUsage`），在 `Event` 上新增 `Usage TokenUsage` 字段——`done` 事件中唯一有增量价值的是消耗统计
- 新增 `EventStart`（goroutine 进入 ReAct 循环前发送一次）和 `EventDone`（`len(toolUses) == 0` 退出前发送），形成 `start → ... → done` 的生命周期括号
- `TokenUsage` 当前暂不填充（sail adapter 的 SSE 解析尚未提取 OpenAI usage 字段），留零值占位

**替代方案**：
- 保留 `DonePayload` 仅删 `Text` — 嵌套结构无意义，一个字段不值得单独一个结构体
- 不发 `EventStart` — 消费者无法区分"channel 刚打开还没数据"和"channel 打开后无事件"，没有 start 事件就缺少明确的就绪信号
