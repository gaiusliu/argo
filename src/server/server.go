package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"argo/src/knot"
	"argo/src/voyage"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server Argo HTTP 后端，无状态路由分发。
// 每次请求自行从磁盘加载状态，结束后持久化。
type Server struct {
	argoDir  string
	addr     string
	certFile string
	keyFile  string
	router   *chi.Mux
}

// Config Server 启动配置。
type Config struct {
	Addr     string
	CertFile string
	KeyFile  string
	WorkDir  string
}

// New 创建 Server 实例，注册 chi 路由。
func New(cfg Config) *Server {
	s := &Server{
		argoDir:  knot.ArgoDir(),
		addr:     cfg.Addr,
		certFile: cfg.CertFile,
		keyFile:  cfg.KeyFile,
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Post("/session/new", s.handleNewSession)
	r.Post("/session/prompt", s.handlePrompt)
	r.Post("/session/resume", s.handleResumeSession)
	r.Get("/session/list", s.handleListSessions)
	r.Post("/session/delete", s.handleDeleteSession)

	s.router = r
	return s
}

// ListenAndServe 启动 HTTP server，支持可选的 TLS。
func (s *Server) ListenAndServe() error {
	if s.certFile != "" && s.keyFile != "" {
		return http.ListenAndServeTLS(s.addr, s.certFile, s.keyFile, s.router)
	}
	return http.ListenAndServe(s.addr, s.router)
}

// ---- Session CRUD handlers ----

func (s *Server) handleNewSession(w http.ResponseWriter, r *http.Request) {
	var req NewSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid req", http.StatusBadRequest)
		return
	}

	slog.Info("handling new session", "req", req)

	v, err := voyage.New(s.argoDir, req.CWD)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, NewSessionResponse{SessionID: v.ID()})
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	infos, err := voyage.List(knot.ArgoDir())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionInfos := make([]SessionInfoJSON, 0, len(infos))
	for _, info := range infos {
		sessionInfos = append(sessionInfos, ToSessionInfoJSON(info))
	}
	respondJSON(w, http.StatusOK, ListSessionsResponse{SessionInfos: sessionInfos})
}

func (s *Server) handleResumeSession(w http.ResponseWriter, r *http.Request) {
	var body ResumeSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	v, err := voyage.Resume(body.SessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	respondJSON(w, http.StatusOK, GetSessionResponse{SessionInfo: ToSessionInfoJSON(v.Info)})
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	var body DeleteSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if err := voyage.Delete(knot.ArgoDir(), body.SessionID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, DeleteSessionResponse{})
}

func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
