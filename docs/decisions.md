# 技术决策

> 每条技术取舍以 DEC-xxx 编号，append-only。废弃标注替代者，不删除历史。

## DEC-001：grep 工具 MVP 阶段使用纯 Go 实现

**状态**：✅ 现行

**日期**：2026-07-03

**决策**：grepHandler 使用纯 Go 实现（filepath.Walk + regexp），零外部依赖。

**背景**：主流 Coding Agent（OpenCode、Claude Code、KiloCode）的 grep 工具底层都调 `ripgrep`（rg），速度快、内置目录排除、跨平台兼容。但 rg 是外部二进制，用户需自行安装。

**选择**：MVP 阶段不引入额外依赖。纯 Go 实现在小至中型仓库性能足够。接口和参数 schema 固定——将来如果遇到大仓库性能瓶颈，内部替换为 `exec.CommandContext(ctx, "rg", ...)` 即可，helm 侧无感知。

**替代方案**：直接集成 ripgrep。因引入外部依赖被否决。
