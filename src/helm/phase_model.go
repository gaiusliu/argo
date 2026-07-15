package helm

import (
	"argo/src/crew"
	"argo/src/deck"
	"argo/src/knot"
	"argo/src/sail"
	"argo/src/voyage"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
)

type PhaseRunnerModel struct {
	Provider     sail.Provider
	Tools        []knot.Tool
	SystemPrompt string
	Compactor    Compactor
}

func NewPhaseRunnerModel() *PhaseRunnerModel {
	tools := deck.List()
	skills := crew.List()
	sp := buildSystemPrompt(tools, skills)

	return &PhaseRunnerModel{
		SystemPrompt: sp,
		Tools:        tools,
	}
}

// resolveCompactor 构造 LLM Summary Compactor。
// provider 为摘要生成所用的模型连接，由 Run() 在 Provider 就绪后传入。
func resolveCompactor(provider sail.Provider) Compactor {
	return &LLMSummaryCompactor{Provider: provider}
}

func (pr *PhaseRunnerModel) Run(ctx context.Context,
	v *voyage.Voyage, emit func(knot.Event)) bool {
	slog.Info("phase model", "session id", v.ID())

	// 根据模型名解析 Provider
	var err error
	pr.Provider, err = sail.NewProvider(v.Model)
	if err != nil {
		emit(knot.Event{Type: knot.EventError, Err: err})
		v.Phase = voyage.PhaseDone
		return false
	}
	v.Model = pr.Provider.Model()

	// 在 Provider 就绪后解析 Compactor
	pr.Compactor = resolveCompactor(pr.Provider)

	// 系统提示词不下沉到 Tale 中，由 Tale.BuildRequest 前插
	slog.Info("build system prompt", "system prompt", pr.SystemPrompt)
	msgs := v.Tale().BuildRequest(pr.SystemPrompt)

	// Compact 触发时替换消息列表并通知 Tale
	summary, from, to, err := pr.Compactor.Compact(ctx, msgs, v.LastUsage.InputTokens)
	if err == nil && from < to {
		msgs = append(
			append(msgs[:from], knot.Message{Role: knot.MessageRoleSystem, Content: summary}),
			msgs[to:]...,
		)
		v.Tale().MarkCompact(from, to, summary)
	}

	events := pr.Provider.Chat(ctx, msgs, pr.Tools)

	// 流式消费 + 收集
	var toolUses []knot.ToolUse
	var textBuf strings.Builder
	for evt := range events {
		if evt.Type == knot.EventToolUseDelta {
			if evt.ToolUse.ID != "" {
				toolUses = append(toolUses, evt.ToolUse)
				continue
			}
		} else if evt.Type == knot.EventTextDelta {
			textBuf.WriteString(evt.Delta)
		} else if evt.Type == knot.EventTextDone {
			evt.Usage.ContextWindow = sail.Window(pr.Provider.Model()).MaxTokens
			v.LastUsage = evt.Usage
		} else if evt.Type == knot.EventError {
			emit(evt)
			v.Phase = voyage.PhaseDone
			return false
		}
		emit(evt)
	}

	// 无工具调用 → 结束
	if len(toolUses) == 0 {
		v.Tale().AppendAssistant(textBuf.String(), nil)
		slog.Info("new message generated", "content", textBuf.String())
		v.Phase = voyage.PhaseDone
		return false
	}

	// 有工具调用 → 构建 assistant 消息 + 转入执行阶段
	toolCalls := make([]knot.ToolCall, len(toolUses))
	for i, tu := range toolUses {
		argsBytes, err := json.Marshal(tu.Parameters)
		if err != nil {
			slog.Error("marshal tool args failed", "tool", tu.Name, "error", err)
			argsBytes = []byte("{}")
		}
		toolCalls[i] = knot.ToolCall{ID: tu.ID, Name: tu.Name, Arguments: string(argsBytes)}
	}

	v.Tale().AppendAssistant(textBuf.String(), toolCalls)
	slog.Info("new message generated", "content", textBuf.String(), "toolCalls", len(toolCalls))

	v.ToolUses = toolUses
	v.Phase = voyage.PhaseExecTools
	return false
}

// checkpoint 断点持久化——Asking / Done 两个终点调用。
func checkpoint(v *voyage.Voyage) {
	// 增量消息写入 transcript（AppendMessages 内部完成 Record 转换 + MarkSaved）
	delta := v.Tale().Unsaved()
	if len(delta) > 0 {
		toolUsesByIDs := make(map[string]knot.ToolUse)
		for _, tu := range v.ToolUses {
			toolUsesByIDs[tu.ID] = tu
		}
		if err := v.AppendMessages(delta, toolUsesByIDs, v.Model); err != nil {
			slog.Error("vault 写入失败", "error", err)
			return
		}
		slog.Info("transcript 增量写入成功", "delta", len(delta), "saved", v.Tale().Saved())
	}

	// 持久化审批记录
	if err := voyage.SaveApprovals(v); err != nil {
		slog.Error("保存审批记录失败", "error", err)
	}
}
