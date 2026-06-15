package deck

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// execShell 执行 shell 命令，返回 ToolResult。
// 封装 exec.CommandContext，处理超时、非零退出码、二进制不存在等场景。
func execShell(ctx context.Context, binary string, args ...string) ToolResult {
	// 空二进制名：直接返回 error，不 panic
	if binary == "" {
		return ToolResult{
			Status: "error",
			Output: "binary name must not be empty",
		}
	}

	// 创建带 context 的命令
	cmd := exec.CommandContext(ctx, binary, args...)

	// 执行并捕获 stdout + stderr
	output, runErr := cmd.CombinedOutput()
	outputStr := string(output)

	// 无错误：成功
	if runErr == nil {
		return ToolResult{
			Status: "success",
			Output: outputStr,
		}
	}

	// 分类错误并确定状态
	// 先检查 context 状态：在 Windows 上 exec.CommandContext 超时时直接杀进程，
	// 产生 *exec.ExitError 而非 context 错误，因此需通过 ctx.Err() 判断。
	if ctxErr := ctx.Err(); ctxErr != nil {
		status := "timeout"
		info := ctxErr.Error()
		if outputStr == "" {
			outputStr = info
		} else {
			outputStr = fmt.Sprintf("%s\n%s", outputStr, info)
		}
		return ToolResult{
			Status:   status,
			Output:   outputStr,
			Metadata: map[string]any{"exec_error": runErr.Error(), "ctx_error": ctxErr.Error()},
		}
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		// 命令执行了但返回非零退出码，在输出中附加退出码信息
		exitCode := exitErr.ExitCode()
		outputStr = fmt.Sprintf("%sexit code %d: %s", outputStr, exitCode, exitErr.Error())
	} else {
		// 其他错误（如二进制不存在）
		outputStr = fmt.Sprintf("%s%s", outputStr, runErr.Error())
	}

	// 确保 Output 非空（至少包含错误信息）
	if outputStr == "" {
		outputStr = runErr.Error()
	}

	return ToolResult{
		Status:   "error",
		Output:   outputStr,
		Metadata: map[string]any{"exec_error": runErr.Error()},
	}
}
