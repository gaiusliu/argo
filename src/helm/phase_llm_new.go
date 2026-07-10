package helm

import (
	"argo/src/deck"
	"argo/src/knot"
	"argo/src/sail"
	"argo/src/vault"
	"argo/src/voyage"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

type PhaseRunner interface {
	Run(ctx context.Context, st *HelmState, emit func(knot.Event), session *voyage.Session)
}

type PhaseRunnerModel struct {
	Model        string
	Provider     string
	APIKey       string
	BaseURL      string
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
	pr.Model = st.Model
	pr.getModelConfig()
	if st.Model != pr.Model {
		st.Model = pr.Model
	}

	// 写入系统提示词 - 不可以把系统提示词添加到st.Messages中
	systemPrompt := knot.Message{Role: knot.MessageRoleSystem, Content: pr.SystemPrompt}
	slog.Info("build system prompt", "system prompt", pr.SystemPrompt)
	msgs := append([]knot.Message{systemPrompt}, st.Messages...)

	s := sail.NewSailOpenAI(pr.Model, pr.Provider, pr.APIKey, pr.BaseURL, msgs, pr.Tools)
	events := s.Chat(ctx)

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
			writeRecordsNew(session, st)
			return
		}

		emit(evt)
	}

	// 无工具调用
	if len(toolUses) == 0 {
		st.Messages = append(st.Messages, knot.Message{
			Role:    knot.MessageRoleAssistant,
			Content: textBuf.String(),
		})

		st.Phase = HelmPhaseDone
		writeRecordsNew(session, st)
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
	st.Messages = append(st.Messages, knot.Message{
		Role:      knot.MessageRoleAssistant,
		Content:   textBuf.String(),
		ToolCalls: toolCalls,
	})

	// exec tools阶段需要执行/审批的工具使用列表
	st.ToolUses = toolUses
	st.Phase = HelmPhaseExecTools
}

var openAIBaseURL = "https://api.openai.com/v1/chat/completions"

func (pr *PhaseRunnerModel) getModelConfig() error {
	cfg, err := knot.GetConfig()
	if err != nil {
		return fmt.Errorf("phase runner llm: %w", err)
	}

	// 模型名为空时使用配置默认值
	if pr.Model == "" {
		pr.Model = cfg.Sail.Model
	}

	var pName string
	var pCfg knot.ProviderConfig
	for name, p := range cfg.Sail.Provider {
		if _, ok := p.Models[pr.Model]; ok {
			pName, pCfg = name, p
			break
		}
	}
	if pName == "" {
		return fmt.Errorf("phase runner llm: model %q not in any provider", pr.Model)
	}

	// 从环境变量读取 API Key
	if len(pCfg.Env) == 0 {
		return fmt.Errorf("phase runner llm: provider %q env not set", pName)
	}
	apiKey := os.Getenv(pCfg.Env[0])
	if apiKey == "" {
		return fmt.Errorf("phase runner llm: env %s empty", pCfg.Env[0])
	}

	pr.APIKey = apiKey

	if pr.isOpenAI() {
		pr.BaseURL = openAIBaseURL
	} else {
		pr.BaseURL, _ = pCfg.Options["base_url"].(string)
		if pr.BaseURL == "" {
			pr.BaseURL = openAIBaseURL
		}
	}
	return nil
}

func (pr *PhaseRunnerModel) isOpenAI() bool {
	return pr.Provider == "openai"
}

func writeRecordsNew(session voyage.Voyage, st *HelmState) {
	toolUsesByIDs := make(map[string]knot.ToolUse)
	for _, tu := range st.ToolUses {
		toolUsesByIDs[tu.ID] = tu
	}
	records := vault.MessagesToRecordsNew(st.Messages, toolUsesByIDs, st.Model)
	if err := session.Append(records); err != nil {
		slog.Error("vault 写入失败", "error", err)
	}
}
