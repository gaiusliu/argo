package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"argo/src/crew"
	"argo/src/helm"
	"argo/src/knot"
	"argo/src/voyage"
)

// handlePrompt 状态机统一入口。无 ToolUseVerdicts 为新消息，有则为 Ask 回复。
func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	var req NewPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	slog.Info("handle prompt", "req", req)

	// 从磁盘恢复 Voyage（内部已加载 Tale）
	v, err := voyage.Resume(req.SessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if len(req.ToolUseVerdicts) > 0 {
		// Ask 回复路径：恢复控制流状态
		if err := v.LoadState(); err != nil {
			http.Error(w, "failed to load state: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// 将审批结果写入 ToolUses
		verdictsByIDs := make(map[string]int)
		for _, verdict := range req.ToolUseVerdicts {
			verdictsByIDs[verdict.ID] = verdict.VerdictResult
		}
		for i, tu := range v.ToolUses {
			if verdict, ok := verdictsByIDs[tu.ID]; ok {
				tu.VerdictResult = verdict
				v.ToolUses[i] = tu
			}
		}
	} else {
		// 新消息路径：设置初始状态 + 追加 user 消息
		v.Phase = voyage.PhaseModel
		v.Model = req.Model

		msg := resolveSlashCommand(req.Message)
		v.Tale().AppendUser(msg)
		slog.Info("new user message", "content", req.Message)
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	events := helm.Run(ctx, v)
	streamAndSave(w, events)
}

// resolveSlashCommand 检测 /skill-name 模式的消息。
// 如果消息以 "/" 开头且对应技能存在，则拼接技能全文后返回；
// 技能不存在时保持原样，交给 LLM 处理。
func resolveSlashCommand(msg string) string {
	if !strings.HasPrefix(msg, "/") {
		return msg
	}

	parts := strings.SplitN(msg, " ", 2)
	skillName := strings.TrimPrefix(parts[0], "/")
	content, err := crew.Instructions(skillName)
	if err != nil {
		return msg // 技能不存在，保持原样
	}

	// 拼合技能全文和用户原话
	userPart := ""
	if len(parts) > 1 {
		userPart = parts[1]
	}
	return fmt.Sprintf("[Loaded skill %q]\n\n%s\n\n---\n%s", skillName, content, userPart)
}

// streamAndSave SSE 转发所有事件，不做持久化（持久化由状态机 checkpoint 负责）。
func streamAndSave(w http.ResponseWriter, events <-chan knot.Event) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	for ev := range events {
		if err := writeSSE(w, ev); err != nil {
			slog.Error("SSE 写入失败", "error", err)
			return
		}
		flusher.Flush()
	}
}
