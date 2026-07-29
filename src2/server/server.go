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
	scfg   pact.ServerCfg
	client *http.Client
	router *chi.Mux
}

// New 创建装配器，初始化 chi router 并注册 /prompt 路由。
func New(scfg pact.ServerCfg, client *http.Client) *Server {
	s := &Server{scfg: scfg, client: client}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Post("/prompt", func(w http.ResponseWriter, r *http.Request) {
		cl, err := NewConfigLoader()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		cfg, err := (&AgentConfigSource{Loader: cl}).Load()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// TODO: 装配 sight/deck/lore/board/voyage → SSE 推送
		_ = cfg
		http.Error(w, "not implemented", http.StatusNotImplemented)
	})
	s.router = r
	return s
}

// ListenAndServe 启动 HTTP 服务。
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.scfg.Addr, s.router)
}
