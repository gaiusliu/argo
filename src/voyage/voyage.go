// Package voyage 管理会话生命周期——创建、恢复、列表、改名、删除。
// Session 聚合 Vault（纯存储）+ brig.Engine（权限门控），为 RunLoop 提供运行时上下文。
package voyage

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	"argo/src/brig"
	"argo/src/vault"
)

// SessionInfo 会话摘要，供 List 展示。
type SessionInfo struct {
	ID           string
	WorkingDir   string
	CreatedAt    time.Time
	LastActiveAt time.Time
	MessageCount int
	Name         string
	Tokens       SessionTokens
}

// SessionTokens 会话 token 累计统计。
type SessionTokens struct {
	Input  int64
	Output int64
}

// Session 会话运行时上下文。嵌入 SessionInfo 自动提升元数据字段。
type Session struct {
	SessionInfo
	Vault   vault.Vault
	Guard   brig.Guard
	baseDir string
}

// CreateSession 创建新会话：生成 ID → 建目录 → 写 session.json → 组装 Session。
func CreateSession(baseDir, cwd string) (*Session, error) {
	id := genSessionID()
	dir := sessionDir(baseDir, id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("voyage create: %w", err)
	}

	now := time.Now()
	info := SessionInfo{
		ID: id, WorkingDir: cwd, CreatedAt: now, LastActiveAt: now,
	}
	if err := saveSessionInfo(dir, info); err != nil {
		return nil, err
	}

	return &Session{
		SessionInfo: info,
		Vault:       vault.NewFileVault(dir),
		Guard:       brig.NewFileGuard(cwd),
		baseDir:     baseDir,
	}, nil
}

// Resume 恢复已有会话为完整 Session（含 Vault + Engine）。
// 从 session.json 读取元数据，WorkingDir 自动用于创建 Engine。
func ResumeSession(baseDir, id string) (*Session, error) {
	dir := sessionDir(baseDir, id)
	info, err := loadSessionInfo(dir)
	if err != nil {
		return nil, fmt.Errorf("voyage resume: %w", err)
	}
	session := &Session{
		SessionInfo: info,
		Vault:       vault.NewFileVault(dir),
		Guard:       brig.NewFileGuard(info.WorkingDir),
		baseDir:     baseDir,
	}

	// 恢复已持久化的用户审批记录（新会话文件不存在为正常情况）
	approvals, err := loadApprovals(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("voyage resume: load approvals: %w", err)
	}

	if len(approvals) > 0 {
		session.Guard.Restore(approvals)
	}
	return session, nil
}

// SaveApprovals 持久化 Guard 中的用户审批记录到 session 目录。
func SaveApprovals(s *Session) error {
	return saveApprovals(sessionDir(s.baseDir, s.ID), s.Guard.Snapshot())
}

// List 列出所有会话信息，按最后活跃时间倒序。
func ListSessions(baseDir string) ([]SessionInfo, error) {
	root := filepath.Join(baseDir, "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("voyage list: %w", err)
	}

	var sessions []SessionInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := loadSessionInfo(filepath.Join(root, e.Name()))
		if err != nil {
			slog.Error("voyage list: skip corrupt dir", "dir", e.Name(), "error", err)
			continue
		}
		sessions = append(sessions, info)
	}

	slices.SortFunc(sessions, func(a, b SessionInfo) int {
		return b.LastActiveAt.Compare(a.LastActiveAt)
	})
	return sessions, nil
}

// RenameSession 修改会话名称，更新 session.json。
func RenameSession(baseDir, id, name string) error {
	info, err := loadSessionInfo(sessionDir(baseDir, id))
	if err != nil {
		return fmt.Errorf("voyage rename: %w", err)
	}
	info.Name = name
	return saveSessionInfo(sessionDir(baseDir, id), info)
}

// DeleteSession 物理删除会话目录及其所有数据。
func DeleteSession(baseDir, id string) error {
	return os.RemoveAll(sessionDir(baseDir, id))
}

// Append 追加 records 到 transcript.jsonl，并更新 session.json 动态字段。
func (s *Session) Append(records []vault.Record) error {
	if err := s.Vault.Append(records); err != nil {
		return fmt.Errorf("voyage append: %w", err)
	}

	// 直接更新嵌入的 SessionInfo
	s.LastActiveAt = time.Now()
	s.MessageCount += len(records)
	for _, r := range records {
		s.Tokens.Input += int64(r.InputTokens)
		s.Tokens.Output += int64(r.OutputTokens)
	}
	return saveSessionInfo(sessionDir(s.baseDir, s.ID), s.SessionInfo)
}

// ---- 内部辅助 ----

func approvalsPath(dir string) string {
	return filepath.Join(dir, "approvals.json")
}

func saveApprovals(dir string, entries []brig.ApprovalEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(approvalsPath(dir), data, 0644)
}

func loadApprovals(dir string) ([]brig.ApprovalEntry, error) {
	data, err := os.ReadFile(approvalsPath(dir))
	if err != nil {
		return nil, err
	}
	var entries []brig.ApprovalEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func sessionDir(baseDir, id string) string {
	return filepath.Join(baseDir, "sessions", id)
}

func genSessionID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("sess_%d_%x", time.Now().UTC().UnixMilli(), b)
}

func saveSessionInfo(dir string, info SessionInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "session.json"), data, 0644)
}

func loadSessionInfo(dir string) (SessionInfo, error) {
	data, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		return SessionInfo{}, fmt.Errorf("get session: %w", err)
	}
	var info SessionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return SessionInfo{}, fmt.Errorf("get session: %w", err)
	}
	return info, nil
}
