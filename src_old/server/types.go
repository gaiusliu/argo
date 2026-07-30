package server

import (
	"time"

	"argo/src_old/knot"
	"argo/src_old/voyage"
)

// ---- 请求体 ----

// NewSessionRequest POST /session/create 请求体。
type NewSessionRequest struct {
	CWD string `json:"cwd"`
}

// PromptRequest POST /session/prompt 请求体。
// 当 AskID 为空时表示新消息；非空时表示对 EventAsk 的回复。
type PromptRequest struct {
	SessionID string `json:"sessionID"`
	Message   string `json:"message,omitempty"`
	AskID     string `json:"askID,omitempty"`
	Allow     bool   `json:"allow,omitempty"`
}

type NewPromptRequest struct {
	SessionID       string         `json:"sessionID"`
	Message         string         `json:"message,omitempty"`
	Model           string         `json:"model,omitempty"`
	ToolUseVerdicts []knot.ToolUse `json:"toolUseVerdicts,omitempty"`
}

// ResumeSessionRequest POST /session/resume 请求体。
type ResumeSessionRequest struct {
	SessionID string `json:"sessionID"`
	CWD       string `json:"cwd"`
}

// DeleteSessionRequest POST /session/delete 请求体。
type DeleteSessionRequest struct {
	SessionID string `json:"sessionID"`
}

// InterruptRequest POST /session/interrupt 请求体。
// 用于 Asking 阶段终止：删除 state.json，下次 Resume 回到断点前状态。
type InterruptRequest struct {
	SessionID string `json:"sessionID"`
}

// ---- JSON 视图类型（与 voyage 内部模型解耦） ----

// TokensJSON token 统计的 JSON 序列化结构。
type TokensJSON struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
}

// SessionInfoJSON 单个 session 的 JSON 序列化结构。
type SessionInfoJSON struct {
	ID           string     `json:"id"`
	WorkingDir   string     `json:"workingDir"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastActiveAt time.Time  `json:"lastActiveAt"`
	MessageCount int        `json:"messageCount"`
	Name         string     `json:"name"`
	Tokens       TokensJSON `json:"tokens"`
}

// ToSessionInfoJSON 将 voyage.Info 转为 API 响应的 JSON 结构。
func ToSessionInfoJSON(info voyage.Info) SessionInfoJSON {
	return SessionInfoJSON{
		ID:           info.ID,
		WorkingDir:   info.WorkingDir,
		CreatedAt:    info.CreatedAt,
		LastActiveAt: info.LastActiveAt,
		Name:         info.Name,
	}
}

// ---- 响应体 ----

// NewSessionResponse POST /session/create 响应体。
type NewSessionResponse struct {
	SessionID string `json:"sessionID"`
}

// ListSessionsResponse GET /session/list 响应体。
type ListSessionsResponse struct {
	SessionInfos []SessionInfoJSON `json:"sessions"`
}

// ResumeSessionResponse POST /session/resume 响应体。
type ResumeSessionResponse struct {
	SessionInfo SessionInfoJSON `json:"session"`
	Messages    []knot.Message  `json:"messages"`
}

// DeleteSessionResponse DELETE /session/{id} 响应体。
type DeleteSessionResponse struct{}

// AskReplyResponse POST /ask/reply 响应体。
type AskReplyResponse struct{}
