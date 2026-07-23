# 006：wake 日志模块 MVP

**日期**：2026-07-06 ~
**状态**：🚧 进行中
**涉及模块**：wake、knot、cli

## 目标

实现 `LogWriter`——一个自定义 `io.Writer`，对接 slog 完成日志文件持久化。支持按日期（优先）+ 大小（其次）自动轮转日志文件，并自动清理过期日志。

## 范围

### 包含

- `LogWriter` 结构体 + `Write` / `rotate` / `cleanup` / `Close` 方法
- `NewLogWriter(cfg knot.LogConfig)` 构造函数：解析路径、创建目录、清理旧文件
- 文件命名规则：`wake-2006-01-02.log`，同日超额切出 `.001`、`.002`…
- 轮转策略：跨天优先，同日超过 MaxSize 追加序号
- 过期清理：启动时删除修改时间超过 MaxAge 的 `.log` 文件
- CLI 集成：`src/cli/main.go` 中 LoadConfig → NewLogWriter → 喂给 slog MultiHandler

### 不含

- 日志级别过滤（slog 内置 LevelVar 已覆盖）
- 日志格式化（slog TextHandler / JSONHandler 已覆盖）
- 旧文件压缩（首版不实现）
- 远程日志、日志查询等高级功能

## 公共设计约束

- 零第三方依赖，仅用标准库
- `LogWriter` 实现 `io.Writer`，兼容 slog Handler
- 并发安全：slog Handler 串行调用 `Write`，无需额外加锁
- 硬编码常量：文件命名前缀 `wake`、同日序号后缀 `.%03d`

## 后续优化方向

当前实现为同步直写模式。以下优化方向已明确但暂不实施，待性能需求驱动时逐一评估。

| 优化 | 动机 | 复杂度 |
|------|------|--------|
| `bufio.Writer` 批处理 | 每次 `Write` 都是一次 syscall；包一层 `bufio.NewWriter` 攒满 4KB 才刷，syscall 次数降 ~500x | 低 |
| 定时 Flush + Close Flush | bufio 引入后崩溃会丢缓冲区未刷数据；5 秒定时刷新 + 退出强制刷新，数据丢失窗口压缩到 ≤5s | 低 |
| 环形队列异步写入 | 将写入从应用 goroutine 剥离到专用消费者，热路径只剩无锁队列 push，纳秒级完成 | 高 |
| mmap 零拷贝 | 绕过内核缓冲区，直接映射文件到用户态地址空间写入 | 中 |

## 整体开发状态

| 功能 | 状态 | 子文档 | 关联测试 |
|------|------|--------|---------|
| LogWriter 核心实现 | ✅ 完成 | — | — |
| CLI 日志集成 | ✅ 完成 | — | — |

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-07-06 | 核心实现 + CLI 集成完成；记录后续优化方向 |
| 2026-07-06 | 迭代开始 |
