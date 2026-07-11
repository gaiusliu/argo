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

// handlePrompt 状态机统一入口。当 AskID 为空时处理新消息，非空时处理 EventAsk 回复。
func (s *Server) handlePromptNew(w http.ResponseWriter, r *http.Request) {
	var req NewPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	// 从磁盘恢复 session 元数据
	session, err := voyage.ResumeVoyage(req.SessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// 加载会话历史记录
	msgs, err := session.Load()
	if err != nil {
		http.Error(w, "failed to load history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 生成新的状态机状态，从phase model开始
	st := helm.NewHelmState(req.Model, msgs)

	if len(req.ToolUseVerdicts) > 0 {
		// 恢复helm state状态, phase会变更为exec tools
		// Messages 由 transcript 提供（json:"-"，不被 state.json 覆盖）
		err := st.Load(session.ID())
		if err != nil {
			http.Error(w, "failed to load helm state: "+err.Error(), http.StatusInternalServerError)
			return
		}

		verdictsByIDs := make(map[string]int)
		for _, verdict := range req.ToolUseVerdicts {
			verdictsByIDs[verdict.ID] = verdict.VerdictResult
		}

		// 将审批结果写入helm state
		for i, tu := range st.ToolUses {
			if v, ok := verdictsByIDs[tu.ID]; ok {
				tu.VerdictResult = v
				st.ToolUses[i] = tu
			}
		}
	} else {
		// 新消息路径：追加 user 消息到对话历史
		userMsg := knot.Message{Role: knot.MessageRoleUser, Content: req.Message}
		st.Messages = append(st.Messages, userMsg)
		slog.Info("new user message", "msg", userMsg)
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	events := helm.RunNew(ctx, st, session)
	streamAndSaveNew(w, events)
}

// streamAndSave SSE 转发所有事件，不做持久化（持久化由状态机 checkpoint 负责）。
func streamAndSaveNew(w http.ResponseWriter, events <-chan knot.Event) {
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
