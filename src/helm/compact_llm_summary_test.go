package helm

import (
	"context"
	"testing"

	"argo/src/knot"
	"argo/src/sail"
)

// fakeProvider 模拟 sail.Provider，返回可控的摘要文本。
// Chat 方法将 summary 逐字符拆为 TextDelta 事件发送到 channel。
type fakeProvider struct {
	name    string
	model   string
	summary string // 空字符串表示不返回任何内容
}

func (f *fakeProvider) Name() string  { return f.name }
func (f *fakeProvider) Model() string { return f.model }

func (f *fakeProvider) Chat(ctx context.Context, msgs []knot.Message, tools []knot.Tool) <-chan knot.Event {
	ch := make(chan knot.Event, len(f.summary)+1)
	go func() {
		defer close(ch)
		for _, r := range f.summary {
			ch <- knot.Event{Type: knot.EventTextDelta, Delta: string(r)}
		}
	}()
	return ch
}

// fakeMsg 快捷构造消息，填充足够的内容以使 token 估算触发 compact。
func fakeMsg(role knot.MessageRole, content string) knot.Message {
	return knot.Message{Role: role, Content: content}
}

// largeContent 生成约 2000 字符的内容，用于构造需要 compact 的消息列表。
func largeContent() string {
	const s = "这是一段较长的文本用于模拟对话内容。"
	var buf []byte
	for i := 0; i < 100; i++ {
		buf = append(buf, s...)
	}
	return string(buf)
}

// TestShouldCompact 验证阈值判断逻辑。
// 方法：使用默认窗口 128000（模型名未知时兜底），阈值 80%=102400。
// 预期：inputTokens ≥102400 时返回 true，<102400 时返回 false。
func TestShouldCompact(t *testing.T) {
	p := &fakeProvider{name: "test", model: "unknown-model"}
	c := &LLMSummaryCompactor{Provider: p}

	maxTokens := sail.Window(p.Model()).MaxTokens
	threshold := maxTokens * compactThreshold / 100
	t.Logf("default window=%d threshold=%d", maxTokens, threshold)

	tests := []struct {
		name       string
		tokens     int
		shouldFire bool
	}{
		{"低于阈值", threshold - 1, false},
		{"恰好等于阈值", threshold, true},
		{"远高于阈值", threshold * 2, true},
		{"零 token", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.ShouldCompact(tt.tokens)
			if got != tt.shouldFire {
				t.Errorf("ShouldCompact(%d)=%v, 期望 %v", tt.tokens, got, tt.shouldFire)
			}
		})
	}
}

// TestCompactNotTriggered 验证低于阈值时 Compact 不执行。
// 方法：inputTokens 低于阈值，传入足够消息。
// 预期：返回空值 (from==0, to==0, summary=="")。
func TestCompactNotTriggered(t *testing.T) {
	p := &fakeProvider{name: "test", model: "unknown-model"}
	c := &LLMSummaryCompactor{Provider: p}

	msgs := []knot.Message{
		fakeMsg(knot.MessageRoleUser, "hello"),
		fakeMsg(knot.MessageRoleAssistant, "hi"),
		fakeMsg(knot.MessageRoleUser, "how are you"),
	}
	summary, from, to, err := c.Compact(context.Background(), msgs, 1000)
	if err != nil {
		t.Fatalf("Compact 不应返回错误: %v", err)
	}
	if from != 0 || to != 0 || summary != "" {
		t.Errorf("期望 from=0 to=0 summary=\"\", 实际 from=%d to=%d summary=%q", from, to, summary)
	}
}

// TestCompactTriggered 验证超过阈值且消息充足时 Compact 正确执行。
// 方法：inputTokens 远高于阈值，构造 10 条大消息，mock Provider 返回预设摘要。
// 预期：返回非空摘要，from≥compactHeadKept，to≤len(msgs)。
func TestCompactTriggered(t *testing.T) {
	p := &fakeProvider{name: "test", model: "unknown-model", summary: "## 摘要\n压缩完成"}
	c := &LLMSummaryCompactor{Provider: p}

	// 构造 10 条大消息
	lc := largeContent()
	msgs := make([]knot.Message, 10)
	for i := range msgs {
		role := knot.MessageRoleUser
		if i%2 == 1 {
			role = knot.MessageRoleAssistant
		}
		msgs[i] = fakeMsg(role, lc)
	}

	// inputTokens 设为远超过阈值
	inputTokens := 200000
	summary, from, to, err := c.Compact(context.Background(), msgs, inputTokens)
	if err != nil {
		t.Fatalf("Compact 返回错误: %v", err)
	}

	if summary != "## 摘要\n压缩完成" {
		t.Errorf("期望摘要='## 摘要\\n压缩完成', 实际=%q", summary)
	}
	if from < compactHeadKept {
		t.Errorf("from(%d) 应 ≥ compactHeadKept(%d)", from, compactHeadKept)
	}
	if to > len(msgs) || to <= from {
		t.Errorf("to(%d) 应在 (%d, %d] 范围内", to, from, len(msgs))
	}
	t.Logf("compact range: [%d, %d), len=%d", from, to, len(msgs))
}

// TestCompactEmptyMessages 验证消息不足时 Compact 不触发。
// 方法：inputTokens 高于阈值，但消息数 ≤ compactHeadKept。
// 预期：返回空值。
func TestCompactEmptyMessages(t *testing.T) {
	p := &fakeProvider{name: "test", model: "unknown-model"}
	c := &LLMSummaryCompactor{Provider: p}

	// 消息数 = compactHeadKept = 2，不够压缩
	msgs := []knot.Message{
		fakeMsg(knot.MessageRoleUser, "hi"),
		fakeMsg(knot.MessageRoleAssistant, "hello"),
	}
	summary, from, to, err := c.Compact(context.Background(), msgs, 200000)
	if err != nil {
		t.Fatalf("Compact 不应返回错误: %v", err)
	}
	if from != 0 || to != 0 {
		t.Errorf("消息不足时应返回 from=0 to=0, 实际 from=%d to=%d", from, to)
	}
	if summary != "" {
		t.Errorf("消息不足时 summary 应为空, 实际=%q", summary)
	}
}
