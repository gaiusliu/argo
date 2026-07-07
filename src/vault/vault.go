// Package vault 是 Argo 的统一存储层，实现会话 transcript 读写、框架级记忆存取、session CRUD。
// MVP 阶段使用文件系统后端，Vault 接口预留后端切换能力。
package vault

import (
	"time"

	"argo/src/knot"
)

// Vault 统一存储接口。各后端（文件、SQLite 等）实现此接口。
type Vault interface {
	// ---- Session 管理 ----

	// CreateSession 创建新会话，返回 session ID。
	// workingDir 为启动时 os.Getwd() 捕获的工作目录。
	CreateSession(workingDir string) (string, error)

	// ListSessions 列出所有会话，按时间倒序。
	ListSessions() ([]SessionInfo, error)

	// GetSession 获取单个会话信息。
	GetSession(sessionID string) (SessionInfo, error)

	// DeleteSession 删除会话及其 transcript。
	DeleteSession(sessionID string) error

	// ---- Session Transcript ----

	// Append 追加本轮产生的 records 到 transcript.jsonl。
	Append(sessionID string, records []Record) error

	// Load 读取 transcript，过滤 user/llm/tool record，映射为 knot.Message。
	Load(sessionID string) ([]knot.Message, error)
}

// SessionInfo 会话摘要信息，用于列表展示。
type SessionInfo struct {
	ID           string        // 会话唯一标识
	WorkingDir   string        // 项目路径
	CreatedAt    time.Time     // 创建时间
	LastActiveAt time.Time     // 最后活跃时间（每次 Append 更新）
	MessageCount int           // transcript 中 user/llm/tool record 总数
	Title        string        // 会话标题，默认取第一条用户消息
	Tokens       SessionTokens // 累计 token 用量，供 reef Compact 参考
}

// SessionTokens 会话 token 累计统计。
type SessionTokens struct {
	Input  int64 // 累计输入 token
	Output int64 // 累计输出 token
}
