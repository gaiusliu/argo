package press

import (
	"context"
	"strings"
	"time"

	"argo/src2/pact"
	"argo/src2/sight"
)

// llmSummary 使用 LLM 生成结构化摘要的压缩策略。
type llmSummary struct {
	sight sight.Sight
}

// Full 仅判断阈值，不执行压缩。
func (c *llmSummary) Full(inputTokens int) bool {
	return inputTokens >= 128000*compactThreshold/100
}

// Press 保护头部 + 尾部，用 LLM 压缩中间消息为结构化摘要。
func (c *llmSummary) Press(ctx context.Context, messages []pact.Message, inputTokens int) (string, int, int, error) {
	if !c.Full(inputTokens) {
		return "", 0, 0, nil
	}
	// 保护头部
	from := compactHeadKept
	if len(messages) <= from {
		return "", 0, 0, nil
	}
	// 尾部 token 倒推
	to := len(messages)
	acc := 0
	for i := len(messages) - 1; i >= from; i-- {
		// 桩化 token 估算：每 4 字节 ≈ 1 token
		acc += len(messages[i].Content) / 4
		if acc >= tailTokenBudget {
			to = i
			break
		}
	}
	if to <= from {
		return "", 0, 0, nil
	}

	// 构造压缩请求：system 模板 + user（待压缩消息序列化）
	compactMsgs := []pact.Message{
		{
			Role:      pact.RoleSystem,
			Content:   summaryTemplate,
			Timestamp: time.Now().Format(time.RFC3339),
		},
		{
			Role:      pact.RoleUser,
			Content:   serialize(messages[from:to]),
			Timestamp: time.Now().Format(time.RFC3339),
		},
	}
	events := c.sight.Take(ctx, compactMsgs)

	// 收集 LLM 返回的摘要文本
	var buf strings.Builder
	for ev := range events {
		if ev.Type == pact.EventTypeTextDelta {
			buf.WriteString(ev.Delta)
		}
		if ev.Type == pact.EventTypeError {
			return "", 0, 0, ev.Err
		}
	}
	summary := strings.TrimSpace(buf.String())
	if summary == "" {
		return "", 0, 0, nil
	}
	return summary, from, to, nil
}
