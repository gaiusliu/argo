package deck

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestExecShell_Success 测试 execShell 正常执行命令并返回成功结果。
// 方法: 调用 execShell 执行 "echo hello"（通过 sh -c），检查返回的 ToolResult。
// 预期结果: Status 为 "success"，Output 包含 "hello"。
func TestExecShell_Success(t *testing.T) {
	ctx := context.Background()
	result := execShell(ctx, "sh", "-c", "echo hello")

	if result.Status != "success" {
		t.Errorf("expected status 'success', got %q", result.Status)
	}
	if !strings.Contains(result.Output, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", result.Output)
	}
}

// TestExecShell_NonZeroExit 测试 execShell 处理命令非零退出码的场景。
// 方法: 调用 execShell 执行 "exit 1"（通过 sh -c），检查返回的 ToolResult。
// 预期结果: Status 为 "error"，Output 或 Metadata 中应包含非零退出码相关信息。
func TestExecShell_NonZeroExit(t *testing.T) {
	ctx := context.Background()
	result := execShell(ctx, "sh", "-c", "exit 1")

	if result.Status != "error" {
		t.Errorf("expected status 'error' for non-zero exit, got %q", result.Status)
	}
	if result.Output == "" {
		t.Error("expected non-empty output containing exit code info")
	}
}

// TestExecShell_CommandNotFound 测试 execShell 处理二进制不存在的场景。
// 方法: 调用 execShell 执行一个不存在的二进制名 "nonexistent_binary_xyz"，检查返回的 ToolResult。
// 预期结果: Status 为 "error"，Output 应包含二进制无法找到的错误描述。
func TestExecShell_CommandNotFound(t *testing.T) {
	ctx := context.Background()
	result := execShell(ctx, "nonexistent_binary_xyz")

	if result.Status != "error" {
		t.Errorf("expected status 'error' for missing binary, got %q", result.Status)
	}
	if result.Output == "" {
		t.Error("expected non-empty output with error detail")
	}
}

// TestExecShell_ContextTimeout 测试 execShell 在 context 超时或取消时的行为。
// 方法: 创建一个短时间内超时的 context，执行一个会阻塞的命令（sleep 10），
//       检查返回的 ToolResult。
// 预期结果: Status 为 "timeout"。
func TestExecShell_ContextTimeout(t *testing.T) {
	// 创建一个 10 毫秒后超时的 context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// 执行会阻塞超过超时时间的命令
	result := execShell(ctx, "sh", "-c", "sleep 10")

	if result.Status != "timeout" {
		t.Errorf("expected status 'timeout', got %q", result.Status)
	}
}

// TestExecShell_Stderr 测试 execShell 正确捕获命令 stderr 输出到 Output 中。
// 方法: 调用 execShell 执行向 stderr 写入内容的命令，验证 Output 非空。
// 预期结果: Output 包含 stderr 产出内容（不为空字符串）。
func TestExecShell_Stderr(t *testing.T) {
	ctx := context.Background()
	result := execShell(ctx, "sh", "-c", "echo stderr_content >&2")

	if result.Output == "" {
		t.Error("expected non-empty output containing stderr content")
	}
}

// TestExecShell_ContextCancel 测试 context 被主动取消时的行为。
// 方法: 创建 context 并立即 cancel，执行会阻塞的命令。
// 预期结果: Status 非空，Output 非空。
func TestExecShell_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := execShell(ctx, "sh", "-c", "sleep 10")

	if result.Status == "" {
		t.Error("expected non-empty status for cancelled context")
	}
	if result.Output == "" {
		t.Error("expected non-empty output with cancellation error info")
	}
}

// TestExecShell_EmptyBinary 测试 execShell 传入空二进制名的边界场景。
// 方法: 调用 execShell(ctx, "")。
// 预期结果: Status 为 "error"，不应 panic。
func TestExecShell_EmptyBinary(t *testing.T) {
	ctx := context.Background()
	result := execShell(ctx, "")

	if result.Status != "error" {
		t.Errorf("expected status 'error' for empty binary, got %q", result.Status)
	}
}
