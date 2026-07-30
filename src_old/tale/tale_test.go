package tale

import (
	"testing"

	"argo/src_old/knot"
)

// makeMsg 快捷构造消息的辅助函数。
func makeMsg(role knot.MessageRole, content string) knot.Message {
	return knot.Message{Role: role, Content: content}
}

// makeCompact 快捷构造 compact 消息，供 BuildRequest 验证使用。
func makeCompact(from, to int, summary string) knot.Message {
	return knot.Message{
		Role:        knot.MessageRoleCompact,
		Content:     summary,
		CompactFrom: from,
		CompactTo:   to,
	}
}

// TestMarkCompact 验证 MarkCompact 将 compact 标记追加到消息列表末尾。
// 方法：构造 Tale，调用 MarkCompact，通过 BuildRequest 验证压缩效果。
// 预期：追加的 compact 消息 Role/Content/From/To 均正确。
func TestMarkCompact(t *testing.T) {
	tale := New([]knot.Message{
		makeMsg(knot.MessageRoleUser, "写一个 web server"),
		makeMsg(knot.MessageRoleAssistant, "好的，我来写"),
	})

	// 执行 compact
	tale.MarkCompact(0, 1, "## Goal\n- 写 web server")

	// BuildRequest 应应用 compact：跳过 m0，插入摘要
	result := tale.BuildRequest("")
	if len(result) != 2 {
		t.Fatalf("期望 2 条消息, 实际 %d", len(result))
	}
	// result[0] = 摘要(system), result[1] = 保留的 assistant 消息
	if result[0].Role != knot.MessageRoleSystem || result[0].Content != "## Goal\n- 写 web server" {
		t.Errorf("result[0]: 期望 system 摘要, 实际 role=%s content=%s", result[0].Role, result[0].Content)
	}
	if result[1].Role != knot.MessageRoleAssistant || result[1].Content != "好的，我来写" {
		t.Errorf("result[1]: 期望 assistant 消息, 实际 role=%s content=%s", result[1].Role, result[1].Content)
	}
}

// TestBuildRequestNoCompact 验证无 compact 标记时返回完整消息列表。
// 方法：构造 Tale 含 3 条消息，不调用 MarkCompact，BuildRequest 带 system prompt。
// 预期：system prompt 前置 + 全部 3 条消息，顺序不变。
func TestBuildRequestNoCompact(t *testing.T) {
	msgs := []knot.Message{
		makeMsg(knot.MessageRoleUser, "hello"),
		makeMsg(knot.MessageRoleAssistant, "hi"),
		makeMsg(knot.MessageRoleUser, "帮我查一下"),
	}
	tale := New(msgs)

	result := tale.BuildRequest("你是助手")
	if len(result) != 4 {
		t.Fatalf("期望 4 条消息 (system + 3), 实际 %d", len(result))
	}
	if result[0].Role != knot.MessageRoleSystem || result[0].Content != "你是助手" {
		t.Errorf("system prompt 不正确: role=%s content=%s", result[0].Role, result[0].Content)
	}
	for i, m := range msgs {
		if result[i+1].Role != m.Role || result[i+1].Content != m.Content {
			t.Errorf("消息[%d] 不匹配: 期望 role=%s, 实际 role=%s", i, m.Role, result[i+1].Role)
		}
	}
}

// TestBuildRequestSingleCompact 验证单个 compact 标记正确应用。
// 方法：构造 8 条消息，MarkCompact(2,6)，BuildRequest。
// 预期：[from,to) 替换为摘要 system 消息，其余消息保留，system prompt 前置。
func TestBuildRequestSingleCompact(t *testing.T) {
	msgs := []knot.Message{
		makeMsg(knot.MessageRoleUser, "U1"),
		makeMsg(knot.MessageRoleAssistant, "A1"),
		makeMsg(knot.MessageRoleUser, "U2"),
		makeMsg(knot.MessageRoleAssistant, "A2"),
		makeMsg(knot.MessageRoleTool, "T1"),
		makeMsg(knot.MessageRoleUser, "U3"),
		makeMsg(knot.MessageRoleAssistant, "A3"),
		makeMsg(knot.MessageRoleUser, "U4"),
	}
	tale := New(msgs)

	tale.MarkCompact(2, 6, "## 摘要\n压缩了 U2~U3")

	result := tale.BuildRequest("SP")
	// 预期: [SP, U1, A1, SYSTEM(摘要), A3, U4]
	if len(result) != 6 {
		t.Fatalf("期望 6 条消息, 实际 %d", len(result))
	}
	expected := []struct {
		role    knot.MessageRole
		content string
	}{
		{knot.MessageRoleSystem, "SP"},
		{knot.MessageRoleUser, "U1"},
		{knot.MessageRoleAssistant, "A1"},
		{knot.MessageRoleSystem, "## 摘要\n压缩了 U2~U3"},
		{knot.MessageRoleAssistant, "A3"},
		{knot.MessageRoleUser, "U4"},
	}
	for i, exp := range expected {
		if result[i].Role != exp.role || result[i].Content != exp.content {
			t.Errorf("result[%d]: 期望 role=%s content=%s, 实际 role=%s content=%s",
				i, exp.role, exp.content, result[i].Role, result[i].Content)
		}
	}
}

// TestBuildRequestMultipleCompact 验证两次 compact 按时间顺序依次应用。
// 方法：模拟两轮 compact：第一轮压缩 [2,6)，继续加消息，第二轮对压缩视图压缩 [2,6)。
// 预期：C1 先应用，C2 后应用，最终得到正确的压缩视图。
func TestBuildRequestMultipleCompact(t *testing.T) {
	tale := New([]knot.Message{
		makeMsg(knot.MessageRoleUser, "U1"),
		makeMsg(knot.MessageRoleAssistant, "A1"),
		makeMsg(knot.MessageRoleUser, "U2"),
		makeMsg(knot.MessageRoleAssistant, "A2"),
		makeMsg(knot.MessageRoleTool, "T1"),
		makeMsg(knot.MessageRoleUser, "U3"),
		makeMsg(knot.MessageRoleAssistant, "A3"),
		makeMsg(knot.MessageRoleUser, "U4"),
	})

	// 第一次 compact
	tale.MarkCompact(2, 6, "S1")

	// 模拟又聊了 3 轮
	tale.AppendUser("U5")
	tale.AppendAssistant("A5", nil)
	tale.AppendTool("call_1", "T2")

	// 第二次 compact——Compactor 看到压缩视图 [U1,A1,S1,A3,U4,U5,A5,T2]
	// 压缩 [2,6) = [S1,A3,U4,U5]
	tale.MarkCompact(2, 6, "S2")

	// BuildRequest 最终输出
	result := tale.BuildRequest("SP")
	// 手动推演:
	// result = [U1,A1,U2,A2,T1,U3,A3,U4,U5,A5,T2] (去掉 compact)
	// 应用 C1(from=2,to=6): [U1,A1,S1,A3,U4,U5,A5,T2]
	// 应用 C2(from=2,to=6): [U1,A1,S2,A5,T2]
	// 前置 SP: [SP,U1,A1,S2,A5,T2]
	if len(result) != 6 {
		t.Fatalf("期望 6 条消息, 实际 %d", len(result))
	}
	expectedPairs := [][2]string{
		{"SP", string(knot.MessageRoleSystem)},
		{"U1", string(knot.MessageRoleUser)},
		{"A1", string(knot.MessageRoleAssistant)},
		{"S2", string(knot.MessageRoleSystem)},
		{"A5", string(knot.MessageRoleAssistant)},
		{"T2", string(knot.MessageRoleTool)},
	}
	for i, ep := range expectedPairs {
		if result[i].Content != ep[0] {
			t.Errorf("result[%d]: 期望 content=%s, 实际 content=%s", i, ep[0], result[i].Content)
		}
		if string(result[i].Role) != ep[1] {
			t.Errorf("result[%d]: 期望 role=%s, 实际 role=%s", i, ep[1], string(result[i].Role))
		}
	}
}
