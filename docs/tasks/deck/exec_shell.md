---
module: deck
function: exec_shell
layer: 0
status: code_done
depends_on:
  internal: []
  external:
    - ToolResult（deck 包内基础类型，调度器已生成）
    - os/exec（标准库）
---

# deck::exec_shell

## 职责

执行 shell 命令，返回 ToolResult。封装 `exec.CommandContext`，处理超时、非零退出码、二进制不存在等场景。

## 签名

```go
func execShell(ctx context.Context, binary string, args ...string) ToolResult
```

## 依赖详情

### 已就绪（可 import 使用）
- `ToolResult` — 基础类型，调度器已生成

### 待实现（需等待）
无

## 测试场景

- TestExecShell_Success — 正常执行 "echo hello"，验证 Status=success，Output 包含 "hello"
- TestExecShell_NonZeroExit — 执行 "exit 1"，验证 Status=error，Output 包含退出码信息
- TestExecShell_CommandNotFound — 执行不存在的二进制，验证 Status=error
- TestExecShell_ContextTimeout — 超时极短的 context，验证 Status=timeout
- TestExecShell_Stderr — 命令向 stderr 写入内容，验证 Output 非空
- TestExecShell_ContextCancel — context 被主动取消，验证 Status 非空且 Output 非空
- TestExecShell_EmptyBinary — 传入空二进制名，验证 Status=error 且不 panic

## 实现状态

- 实现文件: deck/exec_shell.go
- 测试文件: deck/exec_shell_test.go
- 测试结果: 7/7 全部通过
- 实现要点:
  - 空 binary 直接返回 Status="error"，不 panic
  - 使用 exec.CommandContext 创建命令，CombinedOutput 捕获 stdout + stderr
  - 错误分类: 先检查 ctx.Err() 判断超时/取消（适配 Windows 上 CommandContext 杀进程产生 ExitError 而非 context 错误的行为），再判断 *exec.ExitError（非零退出码），其他为普通错误
  - Metadata 中记录 exec_error 信息
