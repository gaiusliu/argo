package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

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
		v.Tale().AppendUser(req.Message)
		slog.Info("new user message", "content", req.Message)
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	events := helm.Run(ctx, v)
	streamAndSave(w, events)
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
