// Package press 定义消息历史压缩策略，在上下文接近窗口上限时用 LLM 生成摘要。
// 按 plan.md 约束，press 可额外 import sight。
package press

import (
	"context"

	"argo/src2/pact"
	"argo/src2/sight"
)

// Press 消息压缩器。Press 失败时返回零值（summary="", from=0, to=0），不中断 agent loop。
type Press interface {
	// Full 判断输入 token 数是否达到压缩阈值。
	Full(inputTokens int) bool
	// Press 压缩 messages[from:to] 为一段结构化摘要。返回的 from/to 为被覆盖的区间。
	Press(ctx context.Context, messages []pact.Message, inputTokens int) (summary string, from, to int, err error)
}

// New 按策略名构造 Press。name 为空时默认 llm-summary。
func New(cfg PressCfg, s sight.Sight) (Press, error) {
	name := cfg.Strategy
	if name == "" {
		name = "llm-summary"
	}
	switch name {
	case "llm-summary":
		return &llmSummary{sight: s}, nil
	default:
		return &llmSummary{sight: s}, nil
	}
}

// PressCfg 上下文压缩配置。
type PressCfg struct {
	// Strategy 压缩策略名，默认 "llm-summary"
	Strategy string `json:"strategy,omitempty"`
	// Threshold 触发压缩的上下文窗口百分比（1-100），默认 80
	Threshold int `json:"threshold,omitempty"`
}
