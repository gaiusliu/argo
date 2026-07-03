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

## DEC-005：ToolRegistry 封装——保留包级 API 的向后兼容

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：将包级全局变量 `regMu`/`tools`/`toolsIdx` 封装为 `ToolRegistry` 结构体，同时保留 `List()`/`Lookup()`/`RegisterTool()`/`RegisterTools()` 包级便捷函数，委托到 `defaultRegistry` 单例。暴露 `NewToolRegistry()` 供测试创建独立实例。

**背景**：包级全局可变状态导致：测试间状态污染不支持 `t.Parallel()`、无法创建多实例（如热重载场景）、并发测试数据竞争。

**选择**：折中方案——不追求完全消除全局状态（会破坏所有调用方），而是用结构体封装并发原语，包级函数保持向下兼容。测试可通过 `NewToolRegistry()` 获得完全隔离的注册表。
