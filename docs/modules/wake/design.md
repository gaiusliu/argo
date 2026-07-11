# wake

## 概述

wake 是 Argo 的日志模块。提供 `LogWriter`——实现 `io.Writer` 的日志文件写入器，按日期+大小自动轮转，自动清理过期文件。通过 `slog.NewTextHandler` 桥接为 Argo 的结构化日志输出。

wake 仅依赖标准库 + knot（`LogConfig`），不依赖其他 Argo 模块。

## 核心概念

| 概念 | 说明 |
|------|------|
| LogWriter | 实现 `io.Writer`，日期优先+大小其次的自动轮转策略 |
| 轮转策略 | 跨天立即切新文件；同日文件超 `maxSize` 追加序号 `.001`、`.002`… |
| 过期清理 | 启动时扫描目录，删除 `mtime` 超过 `maxAge` 天的日志文件 |
| 序号续接 | 启动时扫描当天已有日志文件，取最大序号继续追加（不覆盖） |

## 日志文件命名

```
wake-2026-07-12.log        ← 当天首个文件
wake-2026-07-12.001.log    ← 同日超大小后第 1 个轮转文件
wake-2026-07-12.002.log    ← 同日超大小后第 2 个轮转文件
```

## 公共 API

### NewLogWriter

```go
func NewLogWriter(cfg knot.LogConfig) (*LogWriter, error)
```

1. 解析并创建日志目录（默认 `./logs/`）
2. 应用默认值：`maxSize` 默认 100MB，`maxAge` 默认 7 天
3. 清理过期文件（失败不影响启动）
4. 打开当天日志文件（追加模式，续接已有序号）

### Write

```go
func (w *LogWriter) Write(p []byte) (n int, err error)
```

先检查轮转条件：
- 跨天 → 切新文件，序号重置
- 同日且当前文件即将超过 `maxSize` → 序号 +1 切新文件

然后写入并累加字节数。

### Close

```go
func (w *LogWriter) Close() error
```

关闭当前日志文件。

## 行为合约

### 轮转

- **日期优先**：跨天必然切新文件，不受大小限制
- **大小其次**：同日 `curSize + len(p) > maxSize` 触发序号递增
- **首次启动**：扫描当天已有文件，从最大序号继续追加，不覆盖已有内容

### 清理

- 启动时执行一次 `cleanup()`
- 删除 `wake-*.log` 中 `mtime` 超过 `maxAge` 天的文件
- 清理失败不影响日志写入

### 使用方式

```go
w, _ := wake.NewLogWriter(cfg.Log)
defer w.Close()
slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{...})))
```

## 关键文件

| 文件 | 职责 |
|------|------|
| `writer.go` | LogWriter：轮转、写入、序号管理、过期清理 |
