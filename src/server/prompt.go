package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"argo/src/helm"
	"argo/src/knot"
	"argo/src/vault"
	"argo/src/voyage"
)

// handlePrompt 状态机统一入口。当 AskID 为空时处理新消息，非空时处理 EventAsk 回复。
func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	var body PromptRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	// 从磁盘恢复 session
	session, err := voyage.ResumeSession(knot.ArgoDir(), body.SessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// 统一从 Vault 加载对话历史
	msgs, err := session.Vault.Load()
	if err != nil {
		http.Error(w, "failed to load history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var st *helm.LoopState
	var reply knot.AskResult = knot.AskDefault

	if body.AskID != "" {
		// Ask 回复路径：加载暂停的控制流状态，注入回复
		st, err = loadLoopState(knot.ArgoDir(), body.SessionID)
		if err != nil {
			http.Error(w, "no pending ask for session", http.StatusNotFound)
			return
		}
		if st.Phase != helm.PhaseWaitAsk {
			http.Error(w, "session not waiting for ask", http.StatusConflict)
			return
		}
		if st.AskID != body.AskID {
			http.Error(w, "ask ID mismatch", http.StatusBadRequest)
			return
		}
		if body.Allow {
			reply = knot.AskAllowed
		}
	} else {
		// 新消息路径：追加 user 消息到 vault + 对话历史
		userMsg := knot.Message{Role: knot.MessageRoleUser, Content: body.Message}
		msgs = append(msgs, userMsg)

		records := vault.MessagesToRecords([]knot.Message{userMsg}, nil, "")
		if err := session.Append(records); err != nil {
			slog.Error("vault 写入 user record 失败", "error", err)
		}

		st = helm.NewPromptState("")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := helm.Run(ctx, st, session, &msgs, reply)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	streamAndSave(w, knot.ArgoDir(), body.SessionID, events, st, session)
}

// streamAndSave SSE 转发所有事件。通道关闭后持久化 LoopState 和审批记录。
func streamAndSave(w http.ResponseWriter, baseDir, sessionID string, events <-chan knot.Event, st *helm.LoopState, session *voyage.Session) {
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

	// 通道关闭，持久化控制流状态和审批记录
	if st.Phase == helm.PhaseWaitAsk {
		if err := saveLoopState(baseDir, sessionID, st); err != nil {
			slog.Error("保存 LoopState 失败", "error", err)
		}
	}
	if err := voyage.SaveApprovals(session); err != nil {
		slog.Error("保存审批记录失败", "error", err)
	}
}
