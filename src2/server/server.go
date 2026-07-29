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
	r.Post("/prompt", func(w http.ResponseWriter, r *http.Request) {
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
		// TODO: 装配 sight/deck/lore/board/voyage → SSE 推送
		sight, err := sight.New(cfg.Sight, cfg.Sight.DefaultModel, nil, s.client)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = sight
		// 创建 deck、lore、board、voyage
		d := deck.NewDeck(nil, nil)   // TODO: 传入 builtin hands + clip
		l := lore.New("", "")         // TODO: 传入用户/项目技能目录
		b := board.New(nil, nil, nil) // TODO: 传入规则 + checkpoint
		// TODO: 传入 journal
		v := voyage.New(r.Context(), sight, b, nil, d, l)
		for ev := range v.SetSail() {
			if ev.Kind == pact.KindDone {
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		http.Error(w, "not implemented", http.StatusNotImplemented)
	})
	s.router = r
	return s
}

// ListenAndServe 启动 HTTP 服务。
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.scfg.Addr, s.router)
}
