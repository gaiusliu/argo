package deck

import (
	"testing"
)

// ============================================================
// Lookup 测试
//
// 职责：按名精确查找工具。命中返回 (Tool, true)，未命中返回 (nil, false)。
//
// 签名：func Lookup(name string) (Tool, bool)
//
// 依赖：
//   - tools map（包级 unexported map）
//   - Tool 接口
//
// 覆盖场景：
//   1. 命中：已注册的工具名 → 返回 (Tool, true)
//   2. 未命中：未注册的工具名 → 返回 (nil, false)
//   3. 空 map：没有任何注册工具 → 任意 name → (nil, false)
//   4. 空字符串 name：有工具注册但传空字符串 → (nil, false)
//   5. 多工具注册后精确查找：多个工具注册后能精确命中指定工具
//   6. 删除后查找：工具被删除后再次查找 → (nil, false)
// ============================================================

// TestLookup_Hit 验证已注册的工具能被 Lookup 找到。
// 目的：Register 一个 mockTool 后，Lookup 应返回该工具实例和 true。
// 方法：Register 一个 mockTool，调用 Lookup 并验证返回值和 bool。
// 预期：返回注册的 tool 实例和 true。
func TestLookup_Hit(t *testing.T) {
	name := t.Name()
	t.Cleanup(func() { delete(tools, name) })
	Register(&mockTool{
		name:        name,
		description: "test tool for lookup hit",
		concurrency: Concurrent,
	})

	got, ok := Lookup(name)
	if !ok {
		t.Fatal("Lookup should return true for a registered tool")
	}
	if got == nil {
		t.Fatal("Lookup should return a non-nil Tool for a registered name")
	}
	if got.Name() != name {
		t.Fatalf("Lookup returned tool with name %q, want %q", got.Name(), name)
	}
}

// TestLookup_Miss 验证未注册的工具名返回 (nil, false)。
// 目的：对未注册到 tools map 中的工具名进行 Lookup，应返回 (nil, false)。
// 方法：在一个随机的不存在的 name 上调用 Lookup。
// 预期：返回 (nil, false)。
func TestLookup_Miss(t *testing.T) {
	got, ok := Lookup("nonexistent-tool-" + t.Name())
	if ok {
		t.Fatal("Lookup should return false for an unregistered tool name")
	}
	if got != nil {
		t.Fatalf("Lookup should return nil Tool for an unregistered name, got %v", got)
	}
}

// TestLookup_EmptyName 验证空 name 无论 map 状态如何都返回 (nil, false)。
func TestLookup_EmptyName(t *testing.T) {
	t.Run("empty map", func(t *testing.T) {
		got, ok := Lookup("")
		if ok {
			t.Fatal("empty map: Lookup(\"\") should return false")
		}
		if got != nil {
			t.Fatal("empty map: Lookup(\"\") should return nil Tool")
		}
	})

	t.Run("populated map", func(t *testing.T) {
		name := t.Name() + "-real"
		t.Cleanup(func() { delete(tools, name) })
		Register(&mockTool{name: name, description: "real tool", concurrency: Concurrent})

		got, ok := Lookup("")
		if ok {
			t.Fatal("populated map: Lookup(\"\") should return false")
		}
		if got != nil {
			t.Fatal("populated map: Lookup(\"\") should return nil Tool")
		}
	})
}

// TestLookup_MultipleTools 验证多工具注册后能精确命中目标工具。
// 目的：注册多个同名不同描述的工具后，Lookup 应精确按 name 匹配，返回正确的工具实例。
// 方法：注册多个工具，分别 Lookup 每个名字。
// 预期：每个 name 都命中对应的工具，且工具身份正确。
func TestLookup_MultipleTools(t *testing.T) {
	prefix := t.Name()
	names := []string{prefix + "-bash", prefix + "-read", prefix + "-write"}
	for _, n := range names {
		n := n
		t.Cleanup(func() { delete(tools, n) })
	}

	// 注册三个不同的工具
	Register(&mockTool{name: names[0], description: "bash tool", concurrency: Concurrent})
	Register(&mockTool{name: names[1], description: "read tool", concurrency: Sequential})
	Register(&mockTool{name: names[2], description: "write tool", concurrency: Concurrent})

	// 验证每个工具都能精确找到
	for i, n := range names {
		got, ok := Lookup(n)
		if !ok {
			t.Fatalf("Lookup(%q) should return true for registered tool", n)
		}
		if got.Name() != n {
			t.Fatalf("Lookup(%q) returned tool with name %q, want %q", n, got.Name(), n)
		}
		if got.Description() != []string{"bash tool", "read tool", "write tool"}[i] {
			t.Fatalf("Lookup(%q) returned wrong tool instance (description mismatch)", n)
		}
	}
}

// TestLookup_AfterDelete 验证工具从 map 删除后 Lookup 返回 (nil, false)。
// 目的：工具被删除后不应再被 Lookup 找到。
// 方法：注册一个工具，删除它，然后 Lookup。
// 预期：删除后 Lookup 返回 (nil, false)。
func TestLookup_AfterDelete(t *testing.T) {
	name := t.Name()
	t.Cleanup(func() { delete(tools, name) })

	Register(&mockTool{
		name:        name,
		description: "temporary tool",
		concurrency: Concurrent,
	})

	// 删除工具
	delete(tools, name)

	got, ok := Lookup(name)
	if ok {
		t.Fatal("Lookup should return false after tool is deleted")
	}
	if got != nil {
		t.Fatal("Lookup should return nil after tool is deleted")
	}
}

// TestLookup_DifferentSources 验证不同来源的工具都能被 Lookup 找到。
// 目的：CLI、builtin、MCP 三种来源的工具注册后都应该能被 Lookup 找到。
// 方法：分别注册三种来源的工具，验证每种都能被 Lookup 命中。
// 预期：每种来源的工具都能被精确找到，且 Source() 返回正确的来源类型。
func TestLookup_DifferentSources(t *testing.T) {
	prefix := t.Name()

	// 分别测试三种来源的工具
	t.Run("CLI", func(t *testing.T) {
		name := prefix + "-cli"
		t.Cleanup(func() { delete(tools, name) })
		Register(&mockTool{name: name, description: "CLI tool", concurrency: Concurrent})

		got, ok := Lookup(name)
		if !ok {
			t.Fatal("Lookup should find CLI tool")
		}
		if got.Source() != ToolSourceCLI {
			t.Fatalf("expected Source CLI, got %s", got.Source())
		}
		if got.Description() != "CLI tool" {
			t.Fatalf("expected Description 'CLI tool', got %s", got.Description())
		}
		if got.Concurrency() != Concurrent {
			t.Fatalf("expected Concurrency Concurrent, got %s", got.Concurrency())
		}
	})

	t.Run("Builtin", func(t *testing.T) {
		name := prefix + "-builtin"
		t.Cleanup(func() { delete(tools, name) })
		Register(&builtinMockTool{
			mockTool: mockTool{name: name, description: "builtin tool", concurrency: Concurrent},
		})

		got, ok := Lookup(name)
		if !ok {
			t.Fatal("Lookup should find builtin tool")
		}
		if got.Source() != ToolSourceBuiltin {
			t.Fatalf("expected Source Builtin, got %s", got.Source())
		}
		if got.Description() != "builtin tool" {
			t.Fatalf("expected Description 'builtin tool', got %s", got.Description())
		}
		if got.Concurrency() != Concurrent {
			t.Fatalf("expected Concurrency Concurrent, got %s", got.Concurrency())
		}
	})

	t.Run("MCP", func(t *testing.T) {
		name := prefix + "-mcp"
		t.Cleanup(func() { delete(tools, name) })
		Register(&mcpMockTool{
			mockTool: mockTool{name: name, description: "MCP tool", concurrency: Sequential},
		})

		got, ok := Lookup(name)
		if !ok {
			t.Fatal("Lookup should find MCP tool")
		}
		if got.Source() != ToolSourceMCP {
			t.Fatalf("expected Source MCP, got %s", got.Source())
		}
		if got.Description() != "MCP tool" {
			t.Fatalf("expected Description 'MCP tool', got %s", got.Description())
		}
		if got.Concurrency() != Sequential {
			t.Fatalf("expected Concurrency Sequential, got %s", got.Concurrency())
		}
	})
}

// TestLookup_RegisterConflictPreservesOriginal 验证冲突拒绝后 Lookup 仍返回原工具。
// 目的：Register 拒绝同名外部工具后，Lookup 应仍返回首次注册的原工具。
// 方法：builtin 注册 → 外部同名注册被拒 → Lookup 返回 builtin。
func TestLookup_RegisterConflictPreservesOriginal(t *testing.T) {
	name := t.Name()
	t.Cleanup(func() { delete(tools, name) })

	builtin := &builtinMockTool{mockTool: mockTool{name: name, concurrency: Concurrent}}
	if err := Register(builtin); err != nil {
		t.Fatalf("Register builtin should succeed: %v", err)
	}

	external := &mockTool{name: name, concurrency: Concurrent}
	if err := Register(external); err == nil {
		t.Fatal("Register external with same name should fail")
	}

	got, ok := Lookup(name)
	if !ok {
		t.Fatal("Lookup should still find the original builtin tool")
	}
	if got != builtin {
		t.Fatal("Lookup must return the original builtin, not the rejected external")
	}
}

// TestLookup_NilEntry 验证 tools map 中意外存在 nil 条目的防御。
// 目的：防止脏 nil 条目导致 Lookup 返回 (nil, true) 引发调用方 panic。
func TestLookup_NilEntry(t *testing.T) {
	name := t.Name()
	t.Cleanup(func() { delete(tools, name) })

	tools[name] = nil
	_, ok := Lookup(name)
	if ok {
		t.Fatal("Lookup with nil entry should return false")
	}
}
