package deck

import (
	"context"
	"testing"
)

// ============================================================
// Register 测试
//
// 职责：将 Tool 实例注册到进程唯一的 unexported tools map 中。
// 同名冲突时返回 error（builtin > 外部，CLI 和 MCP 同名冲突报错）。
//
// 签名：func Register(t Tool) error
//
// 设计决策：
//   1. 注册成功时 tools[name] 指向该 Tool，Registed 返回 nil。
//   2. 同名重复注册：CLI/MCP 类工具与已有工具重名时返回 error。
//   3. 同一 name 下 builtin 优先：外部工具（CLI/MCP）不能覆盖 builtin。
//   4. Register(nil) 应返回 error，不 panic。
// ============================================================

// ============================================================
// mock 工具实现
// ============================================================

// mockTool 是一个简单的 Tool 接口实现，用于测试。Source 默认为 ToolSourceCLI。
type mockTool struct {
	name        string
	description string
	concurrency ConcurrencyMode
}


func (m *mockTool) Name() string                                    { return m.name }
func (m *mockTool) Description() string                             { return m.description }
func (m *mockTool) Source() ToolSource                              { return ToolSourceCLI }
func (m *mockTool) Execute(_ context.Context, _ map[string]any) (ToolResult, error) {
	return ToolResult{Status: "success", Output: "mock executed"}, nil
}
func (m *mockTool) Concurrency() ConcurrencyMode                    { return m.concurrency }

// builtinMockTool 模拟 builtin 工具，Source 返回 ToolSourceBuiltin。
type builtinMockTool struct {
	mockTool
}

func (b *builtinMockTool) Source() ToolSource { return ToolSourceBuiltin }

// mcpMockTool 模拟 MCP 工具，Source 返回 ToolSourceMCP。
type mcpMockTool struct {
	mockTool
}

func (m *mcpMockTool) Source() ToolSource { return ToolSourceMCP }

// ============================================================
// 测试用例
// ============================================================

// TestRegister_Success 验证正常注册流程。
// 目的：Register 一个 Tool 后，tools map 中存在该 name 的记录，
// 且记录与注册的 Tool 是同一个实例。
// 方法：构造一个 mockTool，调用 Register，之后检查 tools[name]。
// 预期：Register 返回 nil，tools[name] 为注册的 mockTool 实例。
func TestRegister_Success(t *testing.T) {
	name := t.Name()
	t.Cleanup(func() { delete(tools, name) })

	tool := &mockTool{
		name:        name,
		description: "A test echo tool",
		concurrency: Concurrent,
	}

	err := Register(tool)
	if err != nil {
		t.Fatalf("Register should succeed, got error: %v", err)
	}

	registered, ok := tools[name]
	if !ok {
		t.Fatal("tools map should contain the registered tool")
	}
	if registered != tool {
		t.Fatal("tools map should point to the same tool instance")
	}
}

// TestRegister_DuplicateExternal_ReturnsError 验证两个外部工具同名重复注册返回 error。
// 目的：同一 name 已存在外部工具时，再次注册同名的外部工具应返回 error，且不覆盖原有工具。
// 方法：先注册 mockTool（Source=CLI），再注册另一个同名的 mockTool。
// 预期：第二次 Register 返回 error，tools[name] 仍为首次注册的工具。
func TestRegister_DuplicateExternal_ReturnsError(t *testing.T) {
	name := t.Name()
	t.Cleanup(func() { delete(tools, name) })

	first := &mockTool{
		name:        name,
		description: "first tool",
		concurrency: Concurrent,
	}
	err := Register(first)
	if err != nil {
		t.Fatalf("first Register should succeed, got: %v", err)
	}

	second := &mockTool{
		name:        name,
		description: "second tool (duplicate)",
		concurrency: Sequential,
	}
	err = Register(second)
	if err == nil {
		t.Fatal("Register should return error for duplicate name")
	}

	if tools[name] != first {
		t.Fatal("original tool should not be overwritten")
	}
}

// TestRegister_NilTool_ReturnsError 验证注册 nil Tool 的行为。
// 目的：Register(nil) 不应 panic，而应返回 error。
// 方法：调用 Register(nil)。
// 预期：返回非 nil error。
func TestRegister_NilTool_ReturnsError(t *testing.T) {
	err := Register(nil)
	if err == nil {
		t.Fatal("Register(nil) should return error")
	}
}

// TestRegister_BuiltinPriority 验证 builtin 工具不能被外部工具覆盖。
// 目的：当已有 builtin 工具注册时，同名外部工具注册应返回 error。
// 方法：先用 builtinMockTool 注册一个 builtin 工具，再用 mockTool 同名注册。
// 预期：第二次 Register 返回 error，tools[name] 仍为 builtin 工具。
func TestRegister_BuiltinPriority(t *testing.T) {
	name := t.Name()
	t.Cleanup(func() { delete(tools, name) })

	builtin := &builtinMockTool{
		mockTool: mockTool{
			name:        name,
			description: "builtin tool",
			concurrency: Concurrent,
		},
	}
	err := Register(builtin)
	if err != nil {
		t.Fatalf("Register builtin should succeed, got: %v", err)
	}

	external := &mockTool{
		name:        name,
		description: "external tool trying to overwrite builtin",
		concurrency: Concurrent,
	}
	err = Register(external)
	if err == nil {
		t.Fatal("Register should return error when external tool conflicts with builtin")
	}

	if tools[name] != builtin {
		t.Fatal("builtin tool should not be overwritten by external tool")
	}
}

// TestRegister_MultipleDistinctNames 验证多个不同名的工具可以同时注册。
// 目的：注册多个不同名的工具不应互相影响。
// 方法：连续注册三个不同名的 mockTool。
// 预期：所有注册均成功，tools map 中包含所有三个工具。
func TestRegister_MultipleDistinctNames(t *testing.T) {
	prefix := t.Name()
	names := []string{prefix + "-a", prefix + "-b", prefix + "-c"}
	for _, n := range names {
		n := n // 捕获当前值，避免闭包引用循环变量
		t.Cleanup(func() { delete(tools, n) })
	}

	for _, n := range names {
		tool := &mockTool{name: n, description: "tool " + n, concurrency: Concurrent}
		err := Register(tool)
		if err != nil {
			t.Fatalf("Register %s should succeed, got: %v", n, err)
		}
	}

	for _, n := range names {
		if _, ok := tools[n]; !ok {
			t.Fatalf("tools map should contain %s", n)
		}
	}
}


// TestRegister_MCP_MCP_Conflict
// MCP vs MCP: same-name second register returns error.
func TestRegister_MCP_MCP_Conflict(t *testing.T) {
	name := t.Name()
	t.Cleanup(func() { delete(tools, name) })
	Register(&mcpMockTool{mockTool: mockTool{name: name}})
	if err := Register(&mcpMockTool{mockTool: mockTool{name: name}}); err == nil {
		t.Fatal("second MCP with same name should return error")
	}
}

// TestRegister_CLI_MCP_Conflict
// CLI vs MCP: peer, second register returns error.
func TestRegister_CLI_MCP_Conflict(t *testing.T) {
	name := t.Name()
	t.Cleanup(func() { delete(tools, name) })
	Register(&mockTool{name: name})
	if err := Register(&mcpMockTool{mockTool: mockTool{name: name}}); err == nil {
		t.Fatal("MCP should not override CLI, expected error")
	}
}

// TestRegister_TypedNil_ReturnsError
// typed nil should return error without panic.
func TestRegister_TypedNil_ReturnsError(t *testing.T) {
	var typedNil *mockTool
	if err := Register(typedNil); err == nil {
		t.Fatal("Register(typed nil) should return error")
	}
}
// TestRegister_EmptyName 验证空 name 的注册行为。
// 目的：name 为空的 Tool 注册应该返回 error。
// 方法：构造 name 为 "" 的 mockTool，调用 Register。
// 预期：返回非 nil error，tools map 中不应有空 name 的条目。
func TestRegister_EmptyName(t *testing.T) {
	t.Cleanup(func() { delete(tools, "") })

	tool := &mockTool{
		name:        "",
		description: "tool with empty name",
		concurrency: Concurrent,
	}
	err := Register(tool)
	if err == nil {
		t.Fatal("Register with empty name should return error")
	}
	if _, ok := tools[""]; ok {
		t.Fatal("tools map should not contain empty name entry")
	}
}
