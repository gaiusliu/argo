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

	// 生成新的状态机状态，从PhaseLLM开始
	st := helm.NewHelmState(req.Model, msgs)

	if len(req.ToolUseVerdicts) > 0 {
		// helm phase: wait ask, 需恢复helm state状态
		err := st.Load(session.ID(), knot.ArgoDir())
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
		// 新消息路径：追加 user 消息到 vault + 对话历史
		userMsg := knot.Message{Role: knot.MessageRoleUser, Content: req.Message}
		msgs = append(msgs, userMsg)
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	events := helm.RunNew(ctx, st, session)
	streamAndSaveNew(w, events, st, session)
}

// streamAndSave SSE 转发所有事件。通道关闭后持久化 LoopState 和审批记录。
func streamAndSaveNew(w http.ResponseWriter, events <-chan knot.Event, st *helm.HelmState, session voyage.Voyage) {
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
	if st.Phase == helm.HelmPhaseAsking {
		if err := st.Save(session.ID(), knot.ArgoDir()); err != nil {
			slog.Error("保存 helm state 失败", "error", err)
		}
	}

	if err := voyage.SaveApprovalsNew(session); err != nil {
		slog.Error("保存审批记录失败", "error", err)
	}
}
