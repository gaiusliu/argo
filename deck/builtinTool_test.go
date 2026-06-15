package deck

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// 截断公式常量（来源：deck/postprocess.go:14-15 + docs/deck/decisions.md:150 决策 DEC-007）
//
// 截断规则：head = MaxOutputChars * 40% + tail = MaxOutputChars * 60%
//   head = 50000 * 0.4 = 20000
//   tail = 50000 * 0.6 = 30000
//
// 维护指引：如未来截断比例变更，**必须先改 deck/postprocess.go**，再同步更新本常量。
// 此外 deck/postprocess_test.go:26-28（expectedTruncatedOutput 注释）也有相同硬编码需要同步。
const (
	truncHeadChars = 20000 // postProcess head 字符数（MaxOutputChars * 40%）
	truncTailChars = 30000 // postProcess tail 字符数（MaxOutputChars * 60%）
)

// ============================================================
// builtinTool 测试
//
// 职责：验证 builtinTool 正确实现 Tool 接口，通过 handler 函数指针执行具体逻辑，
// Execute 内部调用 postProcess 做结果截断。
//
// 设计决策：
//   1. Name / Description / Source / Concurrency 直接返回构造时传入的值
//   2. Execute 调用 handler(ctx, params)，对返回的 ToolResult 执行 postProcess
//   3. Source() 始终返回 ToolSourceBuiltin
//   4. handler 为 nil 时应触发 panic（Go 调用 nil 函数指针的自然行为）
// ============================================================

// ============================================================
// Name
// ============================================================

// TestBuiltinTool_Name 验证 Name() 返回构造时传入的 name。
// 目的：确认 Name() 方法正确读取并返回 builtinTool 内部存储的名称。
// 方法：通过 NewBuiltinTool 以不同 name 值创建实例，调用 Name()。
// 预期：返回值与构造函数传入的 name 完全一致。
func TestBuiltinTool_Name(t *testing.T) {
	tests := []struct {
		name         string
		constructor  string
	}{
		{"普通英文名", "read_file"},
		{"中文名", "读取文件"},
		{"空字符串", ""},
		{"带特殊字符", "tool-v2@test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bt := NewBuiltinTool(tt.constructor, "desc", nil, Concurrent)
			got := bt.Name()

			if got != tt.constructor {
				t.Errorf("Name() = %q, want %q", got, tt.constructor)
			}
		})
	}
}

// ============================================================
// Description
// ============================================================

// TestBuiltinTool_Description 验证 Description() 返回构造时传入的 description。
// 目的：确认 Description() 方法正确读取并返回内部存储的描述。
// 方法：通过 NewBuiltinTool 以不同 description 值创建实例，调用 Description()。
// 预期：返回值与构造函数传入的 description 完全一致。
func TestBuiltinTool_Description(t *testing.T) {
	tests := []struct {
		name         string
		description  string
	}{
		{"普通英文描述", "Reads a file from disk"},
		{"中文描述", "从磁盘读取文件内容"},
		{"空字符串", ""},
		{"长文本", strings.Repeat("很长的描述文本", 100)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bt := NewBuiltinTool("some_tool", tt.description, nil, Concurrent)
			got := bt.Description()

			if got != tt.description {
				t.Errorf("Description() = %q, want %q", got, tt.description)
			}
		})
	}
}

// ============================================================
// Source
// ============================================================

// TestBuiltinTool_Source 验证 Source() 始终返回 ToolSourceBuiltin。
// 目的：确认 builtinTool 的 Source 固定为 builtin，与构造参数无关。
// 方法：创建多个不同参数的 builtinTool 实例，调用 Source()。
// 预期：所有实例的 Source() 都返回 ToolSourceBuiltin（"builtin"）。
func TestBuiltinTool_Source(t *testing.T) {
	tests := []struct {
		name        string
		constructor string
	}{
		{"name 为 read_file", "read_file"},
		{"name 为空", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bt := NewBuiltinTool(tt.constructor, "desc", nil, Concurrent)
			got := bt.Source()

			if got != ToolSourceBuiltin {
				t.Errorf("Source() = %q, want %q", got, ToolSourceBuiltin)
			}
		})
	}
}

// ============================================================
// Concurrency
// ============================================================

// TestBuiltinTool_Concurrency 验证 Concurrency() 返回构造时传入的并发模式。
// 目的：确认 builtinTool 正确存储并返回 ConcurrencyMode。
// 方法：分别以 Concurrent 和 Sequential 创建实例，调用 Concurrency()。
// 预期：返回值与构造时传入的模式一致。
func TestBuiltinTool_Concurrency(t *testing.T) {
	tests := []struct {
		name        string
		mode        ConcurrencyMode
	}{
		{"并发模式", Concurrent},
		{"串行模式", Sequential},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bt := NewBuiltinTool("tool", "desc", nil, tt.mode)
			got := bt.Concurrency()

			if got != tt.mode {
				t.Errorf("Concurrency() = %q, want %q", got, tt.mode)
			}
		})
	}
}

// ============================================================
// Execute — 正常路径
// ============================================================

// TestBuiltinTool_Execute_CallsHandler 验证 Execute 调用 handler 并返回其结果。
// 目的：确认 Execute() 正确调用构造时传入的 handler 函数指针，并将其返回值透传给调用方。
// 方法：构造一个 handler，设定返回值，通过闭包变量 called 验证 handler 被调用，
//       然后校验 Execute 返回的 ToolResult 和 error 与 handler 设定值一致。
// 预期：called=true，返回的 ToolResult 和 error 与 handler 设定值匹配。
func TestBuiltinTool_Execute_CallsHandler(t *testing.T) {
	expectedResult := ToolResult{
		Output:   "handler executed successfully",
		Status:   "success",
		Metadata: map[string]any{"key": "value"},
	}

	var called bool
	var receivedCtx context.Context
	var receivedParams map[string]any

	handler := func(ctx context.Context, params map[string]any) (ToolResult, error) {
		called = true
		receivedCtx = ctx
		receivedParams = params
		return expectedResult, nil
	}

	bt := NewBuiltinTool("test_tool", "测试工具", handler, Concurrent)

	ctx := context.Background()
	params := map[string]any{"input": "hello"}

	got, err := bt.Execute(ctx, params)

	// 验证 handler 被调用
	if !called {
		t.Error("handler 未被调用")
	}

	// 验证传入 handler 的 context 正确
	if receivedCtx != ctx {
		t.Error("传入 handler 的 context 与传入 Execute 的 context 不一致")
	}

	// 验证传入 handler 的 params 正确
	if receivedParams["input"] != "hello" {
		t.Errorf("传入 handler 的 params 不正确: got %v", receivedParams)
	}

	// 验证返回值正确
	if err != nil {
		t.Errorf("Execute() 不应返回 error，got: %v", err)
	}
	if got.Output != expectedResult.Output {
		t.Errorf("Execute().Output = %q, want %q", got.Output, expectedResult.Output)
	}
	if got.Status != expectedResult.Status {
		t.Errorf("Execute().Status = %q, want %q", got.Status, expectedResult.Status)
	}
	if got.Metadata["key"] != "value" {
		t.Errorf("Execute().Metadata = %v, want %v", got.Metadata, expectedResult.Metadata)
	}
}

// ============================================================
// Execute — handler 返回 error
// ============================================================

// TestBuiltinTool_Execute_HandlerError 验证 handler 返回 error 时被正确传播。
// 目的：确认 Execute() 不会吞噬 handler 返回的 error，而是原样返回给调用方。
// 方法：构造一个始终返回 error 的 handler，调用 Execute()。
// 预期：Execute() 返回的 error 非 nil 且与 handler 返回的 error 相同。
func TestBuiltinTool_Execute_HandlerError(t *testing.T) {
	expectedErr := errors.New("handler internal failure")

	handler := func(ctx context.Context, params map[string]any) (ToolResult, error) {
		return ToolResult{Status: "error"}, expectedErr
	}

	bt := NewBuiltinTool("failing_tool", "会失败的测试工具", handler, Concurrent)

	got, err := bt.Execute(context.Background(), nil)

	if err == nil {
		t.Error("Execute() 应返回 error，但得到 nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("Execute() error = %v, want %v", err, expectedErr)
	}
	if got.Status != "error" {
		t.Errorf("Execute().Status = %q, want %q", got.Status, "error")
	}
}

// TestBuiltinTool_Execute_HandlerError_OutputTruncated 验证 execute 出错时仍调用 postProcess。
// 目的：确认 Execute() 在 handler 返回 error 时仍然会调用 postProcess 截断大输出。
// 方法：构造 handler 返回极大 Output（超过 MaxOutputChars+TruncationMinSavings）和非 nil error，
//       调用 Execute()。
// 预期：Execute() 返回的 err 可通过 errors.Is 匹配到 handler 的 err，
//       且 result.Output 被截断（含截断标记），长度小于原始长度。
func TestBuiltinTool_Execute_HandlerError_OutputTruncated(t *testing.T) {
	expectedErr := errors.New("handler failed but output still large")
	outputLen := MaxOutputChars + TruncationMinSavings + 1000
	longOutput := strings.Repeat("x", outputLen)

	handler := func(ctx context.Context, params map[string]any) (ToolResult, error) {
		return ToolResult{
			Output: longOutput,
			Status: "error",
		}, expectedErr
	}

	bt := NewBuiltinTool("err_trunc_tool", "错误时截断测试工具", handler, Concurrent)

	got, err := bt.Execute(context.Background(), nil)

	// 验证 error 透传
	if err == nil {
		t.Error("Execute() 应返回 error，但得到 nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("Execute() error 应可通过 errors.Is 匹配到 handler 的 err: got %v, want %v", err, expectedErr)
	}

	// 验证输出被截断（确认 postProcess 被调用）
	if len(got.Output) >= outputLen {
		t.Errorf("输出应被截断（postProcess 应被调用），但长度未减少：got %d, original %d", len(got.Output), outputLen)
	}

	// 验证截断标记存在（确认是 postProcess 风格的截断）
	if !strings.Contains(got.Output, "输出被截断") {
		t.Error("截断后的输出应包含截断标记 '输出被截断'")
	}
}

// ============================================================
// Execute — postProcess 截断
// ============================================================

// TestBuiltinTool_Execute_PostProcess 验证 Execute 内部调用 postProcess 截断大输出。
// 目的：确认 Execute() 在 handler 返回结果后，会经过 postProcess 做输出截断。
// 方法：构造 handler 返回超过 MaxOutputChars+TruncationMinSavings 的超长 Output，
//       调用 Execute()。
// 预期：返回的 ToolResult.Output 被截断，长度远小于原始输出，且包含截断标记文本。
func TestBuiltinTool_Execute_PostProcess(t *testing.T) {
	// 远超截断阈值的输出（确保触发截断）
	outputLen := MaxOutputChars + TruncationMinSavings + 1000
	longOutput := strings.Repeat("x", outputLen)

	handler := func(ctx context.Context, params map[string]any) (ToolResult, error) {
		return ToolResult{
			Output:   longOutput,
			Status:   "success",
			Metadata: map[string]any{"size": outputLen},
		}, nil
	}

	bt := NewBuiltinTool("heavy_tool", "大输出工具", handler, Concurrent)

	got, err := bt.Execute(context.Background(), nil)

	if err != nil {
		t.Fatalf("Execute() 不应返回 error: %v", err)
	}

	// 用 expectedTruncatedOutput helper 做完整字符串比对
	// head/tail 长度引用文件顶部常量（truncHeadChars / truncTailChars），避免硬编码漂移
	head := strings.Repeat("x", truncHeadChars)
	tail := strings.Repeat("x", truncTailChars)
	omitted := outputLen - MaxOutputChars
	expected := expectedTruncatedOutput(head, tail, omitted, outputLen)
	if got.Output != expected {
		t.Errorf("输出截断结果不符合预期")
		t.Logf("  got:  %d 字符", len(got.Output))
		t.Logf("  want: %d 字符", len(expected))
	}

	// 显式断言 HasPrefix 锁定 head 是 truncHeadChars 字符
	if !strings.HasPrefix(got.Output, head) {
		t.Errorf("Output 应以 %d 个 'x' 开头", truncHeadChars)
	}

	// 显式断言 HasSuffix 锁定 tail 是 truncTailChars 字符
	if !strings.HasSuffix(got.Output, tail) {
		t.Errorf("Output 应以 %d 个 'x' 结尾", truncTailChars)
	}

	// 验证 Status 不被 postProcess 改变
	if got.Status != "success" {
		t.Errorf("截断后 Status = %q, want %q", got.Status, "success")
	}

	// 验证 Metadata 不被 postProcess 改变
	if got.Metadata["size"] != outputLen {
		t.Errorf("截断后 Metadata 应保持不变: got %v", got.Metadata)
	}
}

// TestBuiltinTool_Execute_PostProcess_NotTriggered 验证小输出不触发截断。
// 目的：确认 postProcess 只在输出超过阈值时截断，小输出原样返回。
// 方法：构造 handler 返回小输出（100 字符），调用 Execute()。
// 预期：返回的 ToolResult.Output 与 handler 的返回值完全一致。
func TestBuiltinTool_Execute_PostProcess_NotTriggered(t *testing.T) {
	smallOutput := strings.Repeat("s", 100)

	handler := func(ctx context.Context, params map[string]any) (ToolResult, error) {
		return ToolResult{
			Output: smallOutput,
			Status: "success",
		}, nil
	}

	bt := NewBuiltinTool("light_tool", "小输出工具", handler, Concurrent)

	got, err := bt.Execute(context.Background(), nil)

	if err != nil {
		t.Fatalf("Execute() 不应返回 error: %v", err)
	}

	if got.Output != smallOutput {
		t.Errorf("小输出不应被截断：\n  got:  %d 字符 (%q...)\n  want: %d 字符", len(got.Output), got.Output[:min(len(got.Output), 50)], len(smallOutput))
	}
}

// ============================================================
// Execute — context 取消
// ============================================================

// TestBuiltinTool_Execute_ContextCancelled 验证 context 取消时 Execute 主动检查 ctx。
// 目的：确认 Execute() 主动检查 ctx。测试不帮 Execute 检查 ctx，强制 Execute 自行担责。
// 方法：创建一个已取消的 context，handler 对 ctx 完全无感知（永远返回成功），
//       调用 Execute()。
// 预期：Execute() 返回 context.Canceled。
func TestBuiltinTool_Execute_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	handler := func(ctx context.Context, params map[string]any) (ToolResult, error) {
		return ToolResult{Output: "handler ran successfully"}, nil
	}

	bt := NewBuiltinTool("ctx_tool", "context 测试工具", handler, Concurrent)

	_, err := bt.Execute(ctx, nil)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Execute() 在 ctx 取消时应返回 context.Canceled，得到 %v", err)
	}
}

// ============================================================
// Execute — nil handler
// ============================================================

// TestBuiltinTool_Execute_NilHandler 验证 handler 为 nil 时的边界行为。
// 目的：确认当 handler 为 nil 时 Execute() 的行为（预期 Go 会 panic，
//       因为调用了 nil 函数指针；也接受返回 error 的防御性实现）。
// 方法：直接构造 handler=nil 的 builtinTool，调用 Execute()，
//       通过 recover 捕获可能的 panic。
// 预期：要么触发 panic（Go 默认行为），要么返回 error（防御性处理）。
func TestBuiltinTool_Execute_NilHandler(t *testing.T) {
	bt := NewBuiltinTool("nil_handler_tool", "handler 为 nil 的工具", nil, Concurrent)

	// 用 recover 捕获可能的 panic
	didPanic := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				didPanic = true
			}
		}()
		bt.Execute(context.Background(), nil)
	}()

	// handler 为 nil 时，调用 nil 函数指针应触发 panic
	// 但实现者也可能加入防御性检查返回 error
	// 此处验证至少有一种失败形式（panic 或 error）
	if !didPanic {
		// 如果没 panic，再次调用并检查 error
		_, err := bt.Execute(context.Background(), nil)
		if err == nil {
			t.Error("nil handler 应导致 panic 或返回 error，但两者都未发生")
		}
	}
}

// ============================================================
// Execute — 空 params
// ============================================================

// TestBuiltinTool_Execute_NilParams 验证 params 为 nil 时的行为。
// 目的：确认 Execute() 在 params 为 nil 时不会 panic，能正常执行。
// 方法：传入 params=nil，handler 正常返回。
// 预期：Execute() 正常返回 handler 的结果，无 panic。
func TestBuiltinTool_Execute_NilParams(t *testing.T) {
	handler := func(ctx context.Context, params map[string]any) (ToolResult, error) {
		// handler 应能处理 nil params
		return ToolResult{Output: "ok", Status: "success"}, nil
	}

	bt := NewBuiltinTool("nil_params_tool", "nil params 测试", handler, Concurrent)

	got, err := bt.Execute(context.Background(), nil)

	if err != nil {
		t.Fatalf("Execute() 不应返回 error: %v", err)
	}
	if got.Output != "ok" {
		t.Errorf("Execute().Output = %q, want %q", got.Output, "ok")
	}
}

// ============================================================
// Tool 接口合规检查
// ============================================================

// TestBuiltinTool_ImplementsToolInterface 编译时验证 builtinTool 满足 Tool 接口。
// 目的：确保 *builtinTool 类型实现了 Tool 接口的全部方法。
// 方法：编译时类型断言 var _ Tool = (*builtinTool)(nil)。
// 预期：编译通过（如果缺少任一方法，编译器会报错）。
func TestBuiltinTool_ImplementsToolInterface(t *testing.T) {
	// 编译时断言：*builtinTool 必须实现 Tool 接口
	// 若缺少任一方法，此处会编译失败，无需运行时判断
	var _ Tool = (*builtinTool)(nil)
}


