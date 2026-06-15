package deck

import (
	"testing"
)

// ============================================================
// List 测试
//
// 职责：返回当前所有已注册工具的列表，排序稳定。
// 签名：func List() []Tool
//
// 设计决策：
//   1. 空注册表时应返回空切片（非 nil），避免调用方 range nil panic。
//   2. 返回的切片应是 tools map 的副本，调用方不得影响内部状态。
//   3. 多次调用结果顺序一致（排序稳定）。
//   4. 返回所有已注册的工具，不受来源类型（CLI/builtin/MCP）限制。
// ============================================================

// TestList_EmptyRegistry 验证空注册表的行为。
// 目的：Register 未被调用过（tools map 为空）时，List 应返回空切片而非 nil，
// 确保调用方可以安全地 range 而不 panic。
// 方法：在测试前清理所有残留注册，直接调用 List。
// 预期：返回空切片（len 为 0），且不为 nil。
// 注：本测试替换了包级 tools 引用，期间旧的 tools 引用在 Cleanup 前不可见。
// 如加 t.Parallel() 需加锁保护。
func TestList_EmptyRegistry(t *testing.T) {
	saved := tools
	tools = make(map[string]Tool)
	t.Cleanup(func() { tools = saved })

	got := List()
	if got == nil {
		t.Fatal("List() should return empty slice, not nil")
	}
	if len(got) != 0 {
		t.Fatalf("List() should return empty slice, got len=%d", len(got))
	}
}

// TestList_SingleTool 验证注册 1 个工具后 List 返回正确。
// 目的：注册一个 Tool 后，List 返回的切片长度为 1，且包含该工具。
// 方法：注册一个 mockTool，调用 List 检查长度和内容。
// 预期：返回的切片长度为 1，第一个元素即为已注册的工具。
func TestList_SingleTool(t *testing.T) {
	name := t.Name()
	t.Cleanup(func() { delete(tools, name) })
	tool := &mockTool{name: name, description: "single tool", concurrency: Concurrent}
	if err := Register(tool); err != nil {
		t.Fatalf("Register should succeed, got: %v", err)
	}

	got := List()
	if len(got) != 1 {
		t.Fatalf("List() should return 1 tool, got %d", len(got))
	}
	if got[0].Name() != name {
		t.Fatalf("List()[0].Name() = %q, want %q", got[0].Name(), name)
	}
}

// TestList_MultipleTools 验证注册多个工具后 List 返回全部。
// 目的：注册多个不同名的工具，List 应全部返回。
// 方法：连续注册 3 个不同名的 mockTool，检查 List 长度。
// 预期：返回的切片长度为 3，包含所有已注册的工具。
func TestList_MultipleTools(t *testing.T) {
	prefix := t.Name()
	names := []string{prefix + "-c", prefix + "-a", prefix + "-b"}
	for _, n := range names {
		n := n
		t.Cleanup(func() { delete(tools, n) })
	}
	for _, n := range names {
		err := Register(&mockTool{name: n, description: "tool " + n, concurrency: Concurrent})
		if err != nil {
			t.Fatalf("Register %s should succeed, got: %v", n, err)
		}
	}

	got := List()
	if len(got) != len(names) {
		t.Fatalf("List() should return %d tools, got %d", len(names), len(got))
	}

	// 验证所有注册的工具都在返回列表中
	gotNames := make(map[string]bool)
	for _, tool := range got {
		gotNames[tool.Name()] = true
	}
	for _, n := range names {
		if !gotNames[n] {
			t.Fatalf("List() should include tool %q", n)
		}
	}
}


// TestList_OrderByRegistration 验证 List 按注册顺序返回（先注册的先返回）。
// 目的：List 应保持插入顺序，而非按 name 字母序或其他不稳定排序。
// 方法：按特定顺序注册三个工具，验证 List 返回顺序与注册顺序一致。
// 预期：返回的切片保持注册顺序。
func TestList_OrderByRegistration(t *testing.T) {
	prefix := t.Name()
	names := []string{prefix + "-z", prefix + "-a", prefix + "-m"}
	for _, n := range names {
		n := n
		t.Cleanup(func() { delete(tools, n) })
	}
	for _, n := range names {
		err := Register(&mockTool{name: n, description: "tool " + n, concurrency: Concurrent})
		if err != nil {
			t.Fatalf("Register %s should succeed, got: %v", n, err)
		}
	}

	got := List()
	if len(got) != len(names) {
		t.Fatalf("List() should return %d tools, got %d", len(names), len(got))
	}
	for i, n := range names {
		if got[i].Name() != n {
			t.Fatalf("List()[%d].Name() = %q, want %q (order should match registration order)", i, got[i].Name(), n)
		}
	}

}

// TestList_ReturnCopy 验证 List 返回的是副本，篡改返回切片不影响注册表。
// 目的：调用方替换返回切片中的元素，后续 List 调用仍返回原始工具。
// 方法：调用 List，将 first[0] 替换为另一个 Tool，再次 List 确认原工具仍在。
// 预期：第二次 List[0] 仍是注册时的原始工具，非被篡改的。
func TestList_ReturnCopy(t *testing.T) {
	name := t.Name()
	t.Cleanup(func() { delete(tools, name) })
	original := &mockTool{name: name, description: "original", concurrency: Concurrent}
	if err := Register(original); err != nil {
		t.Fatalf("Register should succeed, got: %v", err)
	}

	first := List()
	// 将第一个元素替换为不同的 tool
	first[0] = &mockTool{name: "intruder", description: "intruder", concurrency: Concurrent}

	second := List()
	if len(second) != 1 {
		t.Fatalf("second List should still return 1 tool, got %d", len(second))
	}
	if second[0].Name() != name {
		t.Fatalf("List()[0].Name() = %q, want %q (original should not be replaced)", second[0].Name(), name)
	}
}

// TestList_MixedSourceTypes 验证 List 返回所有来源类型的工具。
// 目的：CLI、builtin、MCP 三种来源的工具都应被 List 返回。
// 方法：注册一个 CLI 工具、一个 builtin 工具、一个 MCP 工具，调用 List。
// 预期：List 长度为 3，包含所有三种来源的工具。
func TestList_MixedSourceTypes(t *testing.T) {
	prefix := t.Name()
	names := []string{prefix + "-cli", prefix + "-builtin", prefix + "-mcp"}
	for _, n := range names {
		n := n
		t.Cleanup(func() { delete(tools, n) })
	}
	// 注册三种来源的工具
	if err := Register(&mockTool{name: prefix + "-cli", description: "CLI tool", concurrency: Concurrent}); err != nil {
		t.Fatalf("Register CLI tool should succeed, got: %v", err)
	}
	if err := Register(&builtinMockTool{mockTool: mockTool{name: prefix + "-builtin", description: "builtin tool", concurrency: Concurrent}}); err != nil {
		t.Fatalf("Register builtin tool should succeed, got: %v", err)
	}
	if err := Register(&mcpMockTool{mockTool: mockTool{name: prefix + "-mcp", description: "MCP tool", concurrency: Concurrent}}); err != nil {
		t.Fatalf("Register MCP tool should succeed, got: %v", err)
	}

	got := List()
	if len(got) != 3 {
		t.Fatalf("List() should return 3 tools, got %d", len(got))
	}
	sources := map[ToolSource]int{}
	for _, tool := range got {
		sources[tool.Source()]++
	}
	if sources[ToolSourceCLI] != 1 || sources[ToolSourceBuiltin] != 1 || sources[ToolSourceMCP] != 1 {
		t.Fatalf("expected 1 of each source, got %v", sources)
	}
}

// TestList_AfterFailedRegister 验证 Register 失败时失败的工具不污染 List。
func TestList_AfterFailedRegister(t *testing.T) {
	name := t.Name()
	t.Cleanup(func() { delete(tools, name) })

	first := &builtinMockTool{mockTool: mockTool{name: name, concurrency: Concurrent}}
	if err := Register(first); err != nil {
		t.Fatalf("first Register should succeed: %v", err)
	}
	// 同名外部工具注册应失败
	if err := Register(&mockTool{name: name, concurrency: Concurrent}); err == nil {
		t.Fatal("second Register (CLI over builtin) should fail")
	}

	got := List()
	if len(got) != 1 {
		t.Fatalf("List should still have 1 tool after failed Register, got %d", len(got))
	}
	if got[0] != first {
		t.Fatal("List should still return the first tool, not the rejected one")
	}
}

// TestList_DoesNotIncludeDeletedTool 验证 delete 后工具不出现在 List 中。
func TestList_DoesNotIncludeDeletedTool(t *testing.T) {
	name := t.Name()
	t.Cleanup(func() { delete(tools, name) })
	Register(&mockTool{name: name, concurrency: Concurrent})
	delete(tools, name)

	got := List()
	for _, tool := range got {
		if tool.Name() == name {
			t.Fatal("List should not include deleted tool")
		}
	}
}

// TestList_DoesNotIncludeNilInterface 验证脏 nil 条目不出现在 List 中。
// 注意：此测试依赖 List 返回非空切片，当 stub 被替换为真实实现后才有实际验证效果。
func TestList_DoesNotIncludeNilInterface(t *testing.T) {
	name := t.Name()
	t.Cleanup(func() { delete(tools, name) })

	tools[name] = nil
	got := List()
	for _, tool := range got {
		if tool == nil {
			t.Fatal("List should not include nil entries")
		}
	}
}
