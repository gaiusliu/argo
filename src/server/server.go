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
	// argoDir Argo 全局配置目录，通常为 ~/.argo/。
	argoDir string
	// addr HTTP 监听地址，例如 :4090。
	addr string
	// certFile TLS 证书文件路径，空字符串表示不启用 HTTPS。
	certFile string
	// keyFile TLS 私钥文件路径，空字符串表示不启用 HTTPS。
	keyFile string
	// router chi 路由多路复用器，持有全部路由注册。
	router *chi.Mux
}

// Config Server 启动配置。
type Config struct {
	// Addr HTTP 监听地址，默认 :4090。
	Addr string
	// CertFile TLS 证书文件路径，空字符串表示不启用 HTTPS。
	CertFile string
	// KeyFile TLS 私钥文件路径，空字符串表示不启用 HTTPS。
	KeyFile string
	// WorkDir 服务端启动时的工作目录，保留以备后续使用。
	WorkDir string
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
	r.Use(middleware.Recoverer) // panic 兜底

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

// handleCreateSession 创建新 session 目录并返回 sessionID。
func (s *Server) handleNewSession(w http.ResponseWriter, r *http.Request) {
	var req NewSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid req", http.StatusBadRequest)
		return
	}

	slog.Info("handling new session", "req", req)

	session, err := voyage.NewSession(s.argoDir, req.CWD)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, NewSessionResponse{SessionID: session.ID()})
}

// handleListSessions 列出所有已持久化的 session。
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := voyage.ListSessions(knot.ArgoDir())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionInfos := make([]SessionInfoJSON, 0, len(sessions))
	for _, session := range sessions {
		sessionInfos = append(sessionInfos, ToSessionInfoJSON(session))
	}
	respondJSON(w, http.StatusOK, ListSessionsResponse{SessionInfos: sessionInfos})
}

// handleResumeSession 恢复已有 session，返回元数据供前端展示。
func (s *Server) handleResumeSession(w http.ResponseWriter, r *http.Request) {
	var body ResumeSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	session, err := voyage.ResumeSession(knot.ArgoDir(), body.SessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	respondJSON(w, http.StatusOK, GetSessionResponse{SessionInfo: ToSessionInfoJSON(session.SessionInfo)})
}

// handleDeleteSession 删除 session 目录及其所有数据。
func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	var body DeleteSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if err := voyage.DeleteSession(knot.ArgoDir(), body.SessionID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, DeleteSessionResponse{})
}

// respondJSON 设置 Content-Type 为 application/json 并序列化 v 写入 w。
func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
