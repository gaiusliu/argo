// Package server 请求级装配层，创建各域模块实例并注入 Voyage。
package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"argo/src2/board"
	"argo/src2/deck"
	"argo/src2/lore"
	"argo/src2/pact"
	"argo/src2/press"
	"argo/src2/sight"
	"argo/src2/voyage"
)

// Server 请求级装配器，每 prompt 创建全新域模块实例。
type Server struct {
	scfg   ServerCfg
	client *http.Client
	router *chi.Mux
}

// New 创建装配器，初始化 chi router 并注册 /prompt 路由。
func New(scfg ServerCfg, client *http.Client) *Server {
	s := &Server{scfg: scfg, client: client}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Post("/prompt", s.handlePrompt)
	s.router = r
	return s
}

// handlePrompt 处理单次 prompt 请求：加载 AgentCfg → 装配 → SetSail → SSE。
func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	cl, err := NewFileConfigLoader()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var cfg pact.AgentCfg
	if err := cl.Load("agent.json", &cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sight, err := sight.New(cfg.Sight, cfg.Sight.DefaultModel, nil, s.client)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// TODO: 传入 builtin hands + clip
	d := deck.NewDeck(nil, nil)
	// TODO: 传入用户/项目技能目录
	l := lore.New("", "")
	// TODO: 传入规则 + checkpoint + journal
	b := board.New(nil, nil, nil)
	p, _ := press.New(cfg.Press.Strategy, sight)
	v := voyage.New(r.Context(), sight, b, nil, d, l, p)
	for ev := range v.SetSail() {
		if ev.Type == pact.EventTypeDone {
			w.WriteHeader(http.StatusOK)
			return
		}
	}
}

// ListenAndServe 启动 HTTP 服务。
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.scfg.Addr, s.router)
}
