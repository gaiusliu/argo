// Package server 请求级装配层，创建各域模块实例并注入 Voyage。
package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"argo/src2/pact"
)

// Server 请求级装配器，每 prompt 创建全新域模块实例。
type Server struct {
	cs     pact.ConfigSource
	client *http.Client
	router *chi.Mux
}

// New 创建装配器，注册 chi 路由。
func New(cs pact.ConfigSource, client *http.Client) *Server {
	s := &Server{cs: cs, client: client}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Post("/prompt", s.handlePrompt)
	s.router = r
	return s
}

// ListenAndServe 启动 HTTP 服务。
// TODO：从 pact.ServerCfg 读取 Addr
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(":8080", s.router)
}

// handlePrompt 处理单次 prompt 请求。
// TODO：实现装配 → voyage.SetSail() → SSE 推送
func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
