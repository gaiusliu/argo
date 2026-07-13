package helm

import (
	"argo/src/deck"
	"argo/src/knot"
	"argo/src/sail"
	"argo/src/vault"
	"argo/src/voyage"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
)

type PhaseRunnerModel struct {
	Provider     sail.Provider // 每轮 Run() 根据 st.Model 重建
	Tools        []knot.Tool
	SystemPrompt string
}

func NewPhaseRunnerModel() *PhaseRunnerModel {
	tools := deck.List()
	sp := buildSystemPrompt(tools)
	return &PhaseRunnerModel{
		SystemPrompt: sp,
		Tools:        tools,
	}
}

func (pr *PhaseRunnerModel) Run(ctx context.Context, st *HelmState, emit func(knot.Event), session voyage.Voyage) {
	slog.Info("phase model", "session id", session.ID(), "helm state", st)

	// 根据模型名解析 Provider（空 → 配置默认，子代理各自独立）
	var err error
	pr.Provider, err = sail.NewProvider(st.Model)
	if err != nil {
		emit(knot.Event{Type: knot.EventError, Err: err})
		st.Phase = HelmPhaseDone
		return
	}

	// 同步兜底模型名回 st
	st.Model = pr.Provider.Model()

	// 写入系统提示词 - 不可以把系统提示词添加到 st.Messages 中
	systemPrompt := knot.Message{Role: knot.MessageRoleSystem, Content: pr.SystemPrompt}
	slog.Info("build system prompt", "system prompt", pr.SystemPrompt)
	msgs := append([]knot.Message{systemPrompt}, st.Messages...)

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
		} else if evt.Type == knot.EventError {
			emit(evt)
			st.Phase = HelmPhaseDone
			return
		}

		emit(evt)
	}

	// 无工具调用
	if len(toolUses) == 0 {
		newMsg := knot.Message{
			Role:    knot.MessageRoleAssistant,
			Content: textBuf.String(),
		}
		st.Messages = append(st.Messages, newMsg)
		slog.Info("new message generated", "msg", newMsg)

		st.Phase = HelmPhaseDone
		return
	}

	// 有工具调用 → 构建 assistant 消息 + 转入执行阶段
	toolCalls := make([]knot.ToolCall, len(toolUses))
	for i, tu := range toolUses {
		argsBytes, err := json.Marshal(tu.Parameters)
		if err != nil {
			slog.Error("marshal tool args failed", "tool", tu.Name, "error", err)
			// 降级为空对象，保持消息链完整
			argsBytes = []byte("{}")
		}
		toolCalls[i] = knot.ToolCall{ID: tu.ID, Name: tu.Name, Arguments: string(argsBytes)}
	}

	newMsg := knot.Message{
		Role:      knot.MessageRoleAssistant,
		Content:   textBuf.String(),
		ToolCalls: toolCalls,
	}
	st.Messages = append(st.Messages, newMsg)
	slog.Info("new message generated", "msg", newMsg)

	// exec tools阶段需要执行/审批的工具使用列表
	st.ToolUses = toolUses
	st.Phase = HelmPhaseExecTools
}

// checkpoint 断点持久化——状态机在 Asking / Done 两个终点调用，将本轮状态快照落盘。
func checkpoint(session voyage.Voyage, st *HelmState) {
	// 增量消息写入 transcript
	delta := st.Messages[st.Saved:]
	if len(delta) > 0 {
		toolUsesByIDs := make(map[string]knot.ToolUse)
		for _, tu := range st.ToolUses {
			toolUsesByIDs[tu.ID] = tu
		}
		records := vault.MessagesToRecords(delta, toolUsesByIDs, st.Model)
		if err := session.Append(records); err != nil {
			slog.Error("vault 写入失败", "error", err)
			return
		}
		st.Saved = len(st.Messages)
		slog.Info("transcript 增量写入成功", "delta", len(delta), "saved", st.Saved)
	}

	// 持久化审批记录
	if err := voyage.SaveApprovals(session); err != nil {
		slog.Error("保存审批记录失败", "error", err)
	}
}
