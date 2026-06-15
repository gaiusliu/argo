package deck

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ============================================================
// cliTool 测试
//
// 职责：验证 cliTool 正确实现 Tool 接口，Execute 内部调用：
//   paramsToArgs 转换参数 → execShell 执行命令 → postProcess 截断输出
//
// 设计决策：
//   1. Name / Description / Concurrency 直接返回构造时传入的值
//   2. Source() 始终返回 ToolSourceCLI
//   3. Execute() 将 params 通过 paramsToArgs 转为 --key=value 参数，再调 execShell，最后 postProcess
//   4. Execute() 在 paramsToArgs 或 execShell 失败时正确传播 error
// ============================================================

// ============================================================
// Name
// ============================================================

// TestCliTool_Name 验证 Name() 返回构造时传入的 name。
// 目的：确认 Name() 方法正确读取并返回 cliTool 内部存储的名称。
// 方法：以不同 name 值创建 cliTool 实例，调用 Name()。
// 预期：返回值与构造时传入的 name 完全一致。
func TestCliTool_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		constructor string
	}{
		{"普通英文名", "git_diff"},
		{"中文名", "命令行工具"},
		{"空字符串边界", ""},
		{"带特殊字符", "tool-v2@cli/runner"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ct := &cliTool{name: tt.constructor}
			got := ct.Name()

			if got != tt.constructor {
				t.Errorf("Name() = %q, want %q", got, tt.constructor)
			}
		})
	}
}

// ============================================================
// Description
// ============================================================

// TestCliTool_Description 验证 Description() 返回构造时传入的 description。
// 目的：确认 Description() 方法正确读取并返回内部存储的描述。
// 方法：以不同 description 值创建 cliTool 实例，调用 Description()。
// 预期：返回值与构造时传入的 description 完全一致。
func TestCliTool_Description(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		description string
	}{
		{"普通英文描述", "Runs git diff to show changes"},
		{"中文描述", "执行 git diff 显示变更内容"},
		{"空字符串边界", ""},
		{"长文本", strings.Repeat("这是一个很长的工具描述", 50)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ct := &cliTool{description: tt.description}
			got := ct.Description()

			if got != tt.description {
				t.Errorf("Description() = %q, want %q", got, tt.description)
			}
		})
	}
}

// ============================================================
// Source
// ============================================================

// TestCliTool_Source 验证 Source() 始终返回 ToolSourceCLI。
// 目的：确认 cliTool 的 Source 固定为 ToolSourceCLI，与构造参数无关。
// 方法：创建多个不同参数的 cliTool 实例，调用 Source()。
// 预期：所有实例的 Source() 都返回 ToolSourceCLI（"cli"）。
func TestCliTool_Source(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ct   *cliTool
	}{
		{"正常实例", &cliTool{name: "tool1", binary: "echo"}},
		{"空字段实例", &cliTool{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.ct.Source()

			if got != ToolSourceCLI {
				t.Errorf("Source() = %q, want %q", got, ToolSourceCLI)
			}
		})
	}
}

// ============================================================
// Concurrency
// ============================================================

// TestCliTool_Concurrency 验证 Concurrency() 返回构造时传入的并发模式。
// 目的：确认 cliTool 正确存储并返回 ConcurrencyMode。
// 方法：分别以 Concurrent 和 Sequential 创建实例，调用 Concurrency()。
// 预期：返回值与构造时传入的模式一致。
func TestCliTool_Concurrency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode ConcurrencyMode
	}{
		{"并发模式", Concurrent},
		{"串行模式", Sequential},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ct := &cliTool{concurrency: tt.mode}
			got := ct.Concurrency()

			if got != tt.mode {
				t.Errorf("Concurrency() = %q, want %q", got, tt.mode)
			}
		})
	}
}

// ============================================================
// Execute — 正常路径
// ============================================================

// TestCliTool_Execute_Success 验证 Execute() 的正常执行路径：
//   paramsToArgs → execShell → postProcess。
// 目的：确认 Execute() 在正常输入下能正确串起三个内部调用。
// 方法：使用 go 二进制（所有平台可用），传入空 params，
//       go 无参数默认打印帮助信息并返回 0。
// 预期：无 error 返回，Status 为 "success"，Output 非空（经过 postProcess）。
func TestCliTool_Execute_Success(t *testing.T) {
	ct := &cliTool{
		name:        "go-help",
		description: "execute go help",
		binary:      "go",
		concurrency: Concurrent,
	}

	result, err := ct.Execute(context.Background(), nil)

	// 不应返回 error
	if err != nil {
		t.Fatalf("Execute() 不应返回 error: %v", err)
	}

	// Status 应为 success
	if result.Status != "success" {
		t.Errorf("Status = %q, want %q", result.Status, "success")
	}

	// Output 非空（go 帮助文本经过 postProcess）
	if result.Output == "" {
		t.Error("Output 不应为空")
	}
}

// TestCliTool_Execute_EmptyParams 验证空 params map 也能正常执行。
// 目的：确认 params 为空时 paramsToArgs 返回空切片，execShell 执行无参数命令。
// 方法：传入空 map[string]any（非 nil），执行 go 命令。
// 预期：正常执行，无 error。
func TestCliTool_Execute_EmptyParams(t *testing.T) {
	ct := &cliTool{
		name:        "go-empty-params",
		description: "test empty params",
		binary:      "go",
		concurrency: Concurrent,
	}

	result, err := ct.Execute(context.Background(), map[string]any{})

	if err != nil {
		t.Fatalf("空 params 时 Execute() 不应返回 error: %v", err)
	}
	if result.Status != "success" {
		t.Errorf("空 params 时 Status = %q, want %q", result.Status, "success")
	}
	if result.Output == "" {
		t.Error("空 params 时 Output 不应为空")
	}
}

// TestCliTool_Execute_NilParams 验证 nil params 不会引发 panic。
// 目的：确认传入 nil params 时 paramsToArgs 返回空切片（按现有实现 nil/empty 都返回空），
//       Execute() 不 panic。
// 方法：传入 params=nil，执行 go 命令。
// 预期：正常执行无 panic，无 error。
func TestCliTool_Execute_NilParams(t *testing.T) {
	ct := &cliTool{
		name:        "go-nil-params",
		description: "test nil params",
		binary:      "go",
		concurrency: Concurrent,
	}

	// nil params 不应 panic
	result, err := ct.Execute(context.Background(), nil)

	if err != nil {
		t.Fatalf("nil params 时 Execute() 不应返回 error: %v", err)
	}
	if result.Status != "success" {
		t.Errorf("nil params 时 Status = %q, want %q", result.Status, "success")
	}
}

// TestCliTool_Execute_PassParamsToBinary 验证 paramsToArgs → execShell 端到端集成：
//   params 被正确转换为 --key=value 格式且按字母序排序后传给命令行。
// 目的：确认 Execute() 内部确实调用了 paramsToArgs，并将转换结果正确传递给 execShell。
//       覆盖字段类型：string、int、bool。
// 方法：编译一个打印 os.Args[1:] 的辅助 Go 程序，传入 map[string]any{"alpha":"first","beta":42,"gamma":true}。
// 预期：Output 包含按字母序的 --alpha=first、--beta=42、--gamma=true。
func TestCliTool_Execute_PassParamsToBinary(t *testing.T) {
	// 步骤 1：编译辅助程序（打印命令行参数）
	tmpDir := t.TempDir()
	helperSrc := filepath.Join(tmpDir, "main.go")
	helperBin := filepath.Join(tmpDir, "printer")
	if runtime.GOOS == "windows" {
		helperBin += ".exe"
	}

	code := `package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	fmt.Print(strings.Join(os.Args[1:], "\n"))
}
`
	if err := os.WriteFile(helperSrc, []byte(code), 0644); err != nil {
		t.Fatalf("写入辅助源码失败: %v", err)
	}

	build := exec.Command("go", "build", "-o", helperBin, helperSrc)
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("跳过：无法编译辅助二进制: %v\n%s", err, out)
	}

	// 步骤 2：通过 cliTool.Execute 传入多种类型的 params
	ct := &cliTool{
		name:        "params-pass-test",
		description: "test params passing end-to-end",
		binary:      helperBin,
		concurrency: Concurrent,
	}

	params := map[string]any{
		"alpha": "first",
		"beta":  42,
		"gamma": true,
	}

	result, err := ct.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("正常 params 时 Execute() 不应返回 error: %v", err)
	}
	if result.Status != "success" {
		t.Errorf("Status = %q, want %q", result.Status, "success")
	}

	// 步骤 3：验证每个参数都以 --key=value 格式出现在输出中
	if !strings.Contains(result.Output, "--alpha=first") {
		t.Error("Output 应包含 --alpha=first")
	}
	if !strings.Contains(result.Output, "--beta=42") {
		t.Error("Output 应包含 --beta=42")
	}
	if !strings.Contains(result.Output, "--gamma=true") {
		t.Error("Output 应包含 --gamma=true")
	}

	// 额外验证：参数按字母序排列（alpha < beta < gamma）
	alphaIdx := strings.Index(result.Output, "--alpha=first")
	betaIdx := strings.Index(result.Output, "--beta=42")
	gammaIdx := strings.Index(result.Output, "--gamma=true")
	if !(alphaIdx >= 0 && betaIdx > alphaIdx && gammaIdx > betaIdx) {
		t.Errorf("参数未按字母序排列：alpha=%d beta=%d gamma=%d", alphaIdx, betaIdx, gammaIdx)
	}
}

// ============================================================
// Execute — postProcess 截断
// ============================================================

// TestCliTool_Execute_Truncation 验证 Execute() 输出经过 postProcess 截断。
// 目的：确认 Execute() 内部调用链完整——execShell 的原始输出 > 50200 字节时，
//       postProcess 截断生效。
// 方法：编译一个辅助 Go 程序，输出 100000 字节的 'x'；通过 cliTool 执行该程序。
//       （辅助程序编译失败时 skip 而非失败，因为依赖外部 go build。）
// 预期：Output 长度 ≤ MaxOutputChars + 标记长度，包含截断标记 "输出被截断"。
func TestCliTool_Execute_Truncation(t *testing.T) {
	// 步骤 1：在临时目录编译辅助程序（输出 100K 字节的 'x'）
	tmpDir := t.TempDir()
	helperSrc := filepath.Join(tmpDir, "main.go")
	helperBin := filepath.Join(tmpDir, "gen_output")
	if runtime.GOOS == "windows" {
		helperBin += ".exe"
	}

	code := `package main

import (
	"fmt"
	"strings"
)

func main() {
	fmt.Print(strings.Repeat("x", 100000))
}
`
	if err := os.WriteFile(helperSrc, []byte(code), 0644); err != nil {
		t.Fatalf("写入辅助源码失败: %v", err)
	}

	// 编译辅助二进制
	build := exec.Command("go", "build", "-o", helperBin, helperSrc)
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("跳过：无法编译辅助二进制（go build 不可用？）: %v\n%s", err, out)
	}

	// 步骤 2：通过 cliTool.Execute 调用辅助程序
	ct := &cliTool{
		name:        "trunc-test",
		description: "truncation test tool",
		binary:      helperBin,
		concurrency: Concurrent,
	}

	result, err := ct.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() 不应返回 error: %v", err)
	}

	// 步骤 3：验证输出已被截断
	if result.Status != "success" {
		t.Errorf("Status = %q, want %q", result.Status, "success")
	}

	// postProcess 截断后长度应 ≤ MaxOutputChars + 标记文本
	if len(result.Output) > MaxOutputChars+1000 {
		t.Errorf("输出未被截断：len=%d，预期 ≤ %d", len(result.Output), MaxOutputChars+1000)
	}

	// 截断标记应包含中文 "输出被截断"
	if !strings.Contains(result.Output, "输出被截断") {
		t.Error("截断后的输出应包含截断标记 '输出被截断'")
	}
}

// ============================================================
// Execute — 错误路径
// ============================================================

// TestCliTool_Execute_ShellError 验证 execShell 失败时 error 正确传播。
// 目的：确认当 execShell 执行失败（如二进制不存在）时，Execute() 返回 error。
// 方法：使用不存在的二进制名 "nonexistent_binary_xyz_123"，传入空 params。
// 预期：Execute() 返回非 nil error。
func TestCliTool_Execute_ShellError(t *testing.T) {
	ct := &cliTool{
		name:        "bad-binary",
		description: "binary that does not exist",
		binary:      "nonexistent_binary_xyz_123",
		concurrency: Concurrent,
	}

	_, err := ct.Execute(context.Background(), nil)

	if err == nil {
		t.Error("二进制不存在时 Execute() 应返回 error，但得到 nil")
	}
}

// TestCliTool_Execute_ParamsError 验证 paramsToArgs 失败时 error 正确传播。
// 目的：确认当参数转换失败（如含 nil value）时，Execute() 返回 error。
// 方法：传入含 nil value 的 params（paramsToArgs 定义为拒收 nil value）。
// 预期：Execute() 返回非 nil error，不应 panic。
func TestCliTool_Execute_ParamsError(t *testing.T) {
	ct := &cliTool{
		name:        "params-error-tool",
		description: "test paramsToArgs failure",
		binary:      "go",
		concurrency: Concurrent,
	}

	params := map[string]any{"bad_key": nil}

	_, err := ct.Execute(context.Background(), params)

	if err == nil {
		t.Error("params 含 nil value 时 Execute() 应返回 error，但得到 nil")
	}
}

// TestCliTool_Execute_UnsupportedParamType 验证 paramsToArgs 遇到不支持类型时错误传播。
// 目的：确认 params 中包含不支持的类型（如 []int）时，Execute() 返回 error。
// 方法：传入含 []int 类型的 params。
// 预期：Execute() 返回非 nil error。
func TestCliTool_Execute_UnsupportedParamType(t *testing.T) {
	ct := &cliTool{
		name:        "unsupported-type-tool",
		description: "test unsupported param type",
		binary:      "go",
		concurrency: Concurrent,
	}

	params := map[string]any{"list": []int{1, 2, 3}}

	_, err := ct.Execute(context.Background(), params)

	if err == nil {
		t.Error("params 含不支持类型时 Execute() 应返回 error，但得到 nil")
	}
}

// ============================================================
// Execute — context 取消/超时
// ============================================================

// TestCliTool_Execute_ContextCancelled 验证 context 被取消时错误正确处理。
// 目的：确认当传入已取消的 context 时，Execute() 能正确传播错误。
// 方法：创建 context 并立即 cancel，传入 Execute()。
// 预期：Execute() 返回非 nil error（且 ToolResult.Status 非 success）。
func TestCliTool_Execute_ContextCancelled(t *testing.T) {
	ct := &cliTool{
		name:        "ctx-cancel-tool",
		description: "test context cancellation",
		binary:      "go",
		concurrency: Concurrent,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	result, err := ct.Execute(ctx, nil)

	// context 已取消时，Execute() 应返回 error
	if err == nil {
		t.Error("context 已取消时 Execute() 应返回 error，但得到 nil")
	}
	// Status 不应为 success
	if result.Status == "success" {
		t.Errorf("context 已取消时 Status 不应为 success，got %q", result.Status)
	}
}

// TestCliTool_Execute_ContextTimeout 验证 context 超时时错误正确处理。
// 目的：确认当 context 超时时，execShell 的超时状态能被 Execute() 正确传播。
// 方法：编译一个 sleep 10 秒的辅助程序，使用 50ms 超时 context 执行。
//       （辅助程序编译失败时 skip 而非失败，因为依赖外部 go build。）
// 预期：Execute() 返回非 nil error，或 ToolResult.Status 为 "timeout"。
func TestCliTool_Execute_ContextTimeout(t *testing.T) {
	// 步骤 1：编译 sleep 辅助程序
	tmpDir := t.TempDir()
	helperSrc := filepath.Join(tmpDir, "main.go")
	helperBin := filepath.Join(tmpDir, "sleeper")
	if runtime.GOOS == "windows" {
		helperBin += ".exe"
	}

	code := `package main

import "time"

func main() {
	time.Sleep(10 * time.Second)
}
`
	if err := os.WriteFile(helperSrc, []byte(code), 0644); err != nil {
		t.Fatalf("写入辅助源码失败: %v", err)
	}

	build := exec.Command("go", "build", "-o", helperBin, helperSrc)
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("跳过：无法编译辅助二进制: %v\n%s", err, out)
	}

	// 步骤 2：用 50ms 超时 context 执行 sleep 辅助程序
	ct := &cliTool{
		name:        "ctx-timeout-tool",
		description: "test context timeout",
		binary:      helperBin,
		concurrency: Concurrent,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := ct.Execute(ctx, nil)

	// 超时 context 下，应返回 timeout status 或 error
	if err == nil && result.Status != "timeout" {
		t.Errorf("超时场景下应返回 timeout status 或 error，got err=%v status=%q", err, result.Status)
	}
}

// ============================================================
// Execute — 空 binary
// ============================================================

// TestCliTool_Execute_EmptyBinary 验证 binary 为空字符串时的边界行为。
// 目的：确认当 cliTool 的 binary 为空时，Execute() 不会 panic。
// 方法：创建 binary="" 的 cliTool，调用 Execute()。
// 预期：不 panic，返回非 nil error（且 ToolResult.Status 非 success）。
func TestCliTool_Execute_EmptyBinary(t *testing.T) {
	ct := &cliTool{
		name:        "empty-binary-tool",
		description: "test empty binary",
		binary:      "",
		concurrency: Concurrent,
	}

	result, err := ct.Execute(context.Background(), nil)

	// binary 为空时，应返回 error
	if err == nil {
		t.Error("binary 为空时 Execute() 应返回 error，但得到 nil")
	}
	// Status 不应为 success
	if result.Status == "success" {
		t.Errorf("binary 为空时 Status 不应为 success，got %q", result.Status)
	}
}

// ============================================================
// Tool 接口合规检查
// ============================================================

// TestCliTool_ImplementsToolInterface 编译时验证 cliTool 满足 Tool 接口。
// 目的：确保 *cliTool 类型实现了 Tool 接口的全部方法。
// 方法：编译时类型断言 var _ Tool = (*cliTool)(nil)。
// 预期：编译通过（如果缺少任一方法，编译器会报错）。
func TestCliTool_ImplementsToolInterface(t *testing.T) {
	// 编译时断言：*cliTool 必须实现 Tool 接口
	var _ Tool = (*cliTool)(nil)

	// 运行时也确认一下
	ct := &cliTool{name: "iface_check", binary: "go"}
	var iface Tool = ct

	// 运行时通过接口调用验证方法可达
	if iface.Name() != "iface_check" {
		t.Errorf("通过接口调用 Name() = %q, want %q", iface.Name(), "iface_check")
	}
}
