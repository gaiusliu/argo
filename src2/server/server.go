// Package server 请求级装配层，创建各域模块实例并注入 Voyage。
package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"argo/src2/voyage"
)

// PromptRequest /prompt 请求体。
// 新对话传 prompt，审批请求传 decisions。
type PromptRequest struct {
	Prompt    string         `json:"prompt,omitempty"`
	Decisions map[string]int `json:"decisions,omitempty"`
}

// Server 请求级装配器，每 prompt 创建全新域模块实例。
type Server struct {
	scfg   ServerCfg
	router *chi.Mux
}

// New 创建装配器，初始化 chi router 并注册 /prompt 路由。
func New(scfg ServerCfg) *Server {
	s := &Server{scfg: scfg}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Post("/prompt", s.handlePrompt)
	s.router = r
	return s
}

// handlePrompt 处理单次 prompt/审批请求：加载配置 → 装配 Voyage → SSE 输出。
func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	var req PromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cl, err := NewFileConfigLoader()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var cfg voyage.AgentCfg
	if err := cl.Load("agent.json", &cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v, err := voyage.New(r.Context(), cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := v.LoadMessages(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(req.Decisions) > 0 {
		if err := v.Resume(req.Decisions); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		v.AppendUserMessage(req.Prompt)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)
	for ev := range v.SetSail() {
		data, _ := json.Marshal(ev)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// ListenAndServe 启动 HTTP 服务。
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.scfg.Addr, s.router)
}
