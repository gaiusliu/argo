---
module: deck
function: truncation
layer: 0
status: code_done
depends_on:
  internal: []
  external:
    - os（标准库）
    - path/filepath（标准库）
    - crypto/rand（标准库）
    - time（标准库）
    - sync（标准库）
---

# deck::truncation

## 职责

提供 truncation 目录管理能力：写入完整工具输出（截断前）+ 后台定时清理超期文件。是 postProcess 截断流程的前置依赖（详见 [DEC-012](../../deck/decisions.md#dec-012truncation-目录管理归属)）。

## 接口

```go
package truncation

import "time"

// Writer 抽象 truncation 目录的文件写入能力。
type Writer interface {
    // Dir 返回 Writer 关联的存储目录绝对路径。
    Dir() string
    // Write 把 content 写入 truncation 目录，返回完整绝对路径。
    // 失败时返回 ("", error)，不 panic。
    Write(content string) (path string, err error)
}

// NewDefault 返回默认 Writer，写入 ~/.argo/truncation/ 目录。
// 文件名格式：tool-<unix-nano>-<random>.log（详见 DEC-007 第 212 行）。
func NewDefault() Writer

// StartCleanup 启动后台 ticker 清理超期文件。
// retention: 文件保留期（默认 7 天）
// interval: 清理检查间隔（默认 1 小时）
// 返回 stop() 停止清理 + ticked 通道（每次清理 tick 完成后发送信号，stop 后关闭）。
func StartCleanup(w Writer, retention time.Duration, interval time.Duration) (stop func(), ticked <-chan struct{})

// Logf 是清理日志记录函数，默认使用 log.Printf。
// 调用方可替换为自己的 logger（如替换为 truncation.Logf = myLogger.Infof）。
var Logf = log.Printf
```
```

## 依赖详情

### 已就绪（可 import 使用）
- `os` / `path/filepath` / `crypto/rand` / `time` / `sync` — Go 标准库
- `~/.argo/` 目录约定（DEC-007 第 212 行）

### 本 task 内新增
- `truncation.Writer` 接口
- `truncation.NewDefault()` 构造函数
- `truncation.StartCleanup()` 启动后台清理

### 待实现（需等待）
- 无

## 行为

```
NewDefault():
  ├─ 计算 home 目录（os.UserHomeDir）
  ├─ 失败时回退到 os.TempDir() 并通过 Logf 记录警告
  └─ 返回 defaultWriter 实例（持有路径，不预创建目录）

defaultWriter.Write(content):
  ├─ 拼路径 dw.dir/tool-<unix-nano>-<random>.log
  ├─ 若目录不存在 → os.MkdirAll 创建
  ├─ os.Create 文件 + os.WriteFile 写入 content
  ├─ 成功 → 返回绝对路径（filepath.Abs）
  └─ 失败 → 返回 ("", err)，不 panic

StartCleanup(w, retention=7*24h, interval=1h):
  ├─ 启动 goroutine + time.Ticker(interval)
  ├─ 每次 tick：
  │   ├─ 扫描 w.Dir() 下所有 tool-*.log
  │   ├─ 检查 mtime，若 now - mtime > retention → os.Remove
  │   ├─ 单个文件删除失败 → 通过 Logf 打印一行日志，继续处理其余文件（best-effort）
  │   ├─ 目录不存在 → 无文件可清理，不报错
  │   └─ 向 ticked 通道发送信号
  ├─ 返回 stop()：停止 ticker、close(done)，goroutine 退出。stop() 幂等（sync.Once 保护），重复调用无副作用
  └─ 返回 ticked：每次 tick 完成后发送 struct{}{}，stop 后关闭
```

## 测试场景

测试文件：`deck/truncation/truncation_test.go`（package `truncation`）

| # | 测试方法 | 场景 |
|---|---------|------|
| 1 | `TestWriter_Write_Success` | 正常写入返回绝对路径，文件存在且内容等于原始 content |
| 2 | `TestWriter_Write_EmptyContent` | 空字符串 content 也能写入，返回有效路径 |
| 3 | `TestWriter_Write_UniquePath` | 多次 Write 返回不同路径（unix-nano 精度 + random 兜底）|
| 4 | `TestWriter_Write_DirectoryNotExist` | 目录不存在时自动创建（os.MkdirAll）|
| 5 | `TestWriter_Write_Failure` | 模拟 IO 错误（阻塞目录创建）返回 err 且 path="" |
| 6 | `TestCleanup_DeletesOldFiles` | mtime > retention 的文件被删除 |
| 7 | `TestCleanup_KeepsRecentFiles` | mtime < retention 的文件保留（同时验证过期文件被删）|
| 8 | `TestCleanup_Stop` | stop() 后 ticker 停止（第一批删除后，第二批保留）|

辅助函数（`truncation_test.go` 内部）：
- `createFileWithMtime` — 创建文件并设置修改时间
- `countLogFiles` — 统计目录下 `tool-*.log` 文件数量

## 实现状态

> 本节由 gen-code subagent 在实现代码完成后填充。

| 项目 | 状态 |
|------|------|
| 测试文件 | `deck/truncation/truncation_test.go` — ✅ gen-test 已完成（8 个测试）|
| 桩代码 | `deck/truncation/truncation.go` — ✅ gen-code 已替换为真实实现 |
| 实现文件 | `deck/truncation/truncation.go` — ✅ gen-code 已完成（83 行） |
| 测试通过 | ✅ 8/8 PASS（`go test -v ./deck/truncation/...`） |

### 覆盖场景

| # | 测试 | 场景 |
|---|------|------|
| 1 | `TestWriter_Write_Success` | 正常写入返回绝对路径，内容匹配 |
| 2 | `TestWriter_Write_EmptyContent` | 空字符串写入，文件存在且长度为 0 |
| 3 | `TestWriter_Write_UniquePath` | 连续 Write 返回不同路径 |
| 4 | `TestWriter_Write_DirectoryNotExist` | 目录不存在时 os.MkdirAll 自动创建 |
| 5 | `TestWriter_Write_Failure` | 目录创建被文件阻塞，返回 err 且 path="" |
| 6 | `TestCleanup_DeletesOldFiles` | 过期文件（30天前）被删除 |
| 7 | `TestCleanup_KeepsRecentFiles` | 混合场景：过期文件被删，近期文件保留 |
| 8 | `TestCleanup_Stop` | stop() 后 ticker 停止，新过期文件不被删除 |

## 与 postProcess 的集成

postProcess 通过 `truncation.Writer` 单例调用 Write（详见 [postProcess.md](postProcess.md)）：

```go
// deck/post_process.go 中的全局单例
var defaultWriter = truncation.NewDefault()

func postProcess(result ToolResult, cfg TruncationConfig) ToolResult {
    if len(result.Output) <= MaxOutputChars {
        return /* 不截断 */
    }
    // 写完整输出到 truncation 目录
    path, _ := defaultWriter.Write(result.Output)
    // 失败降级：path == "" 时 marker 不含 path 段
    // ...
}
```
