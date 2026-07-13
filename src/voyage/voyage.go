// Package voyage 管理会话生命周期——创建、恢复、列表、改名、删除。
// Session 聚合 Vault（纯存储）+ brig.Gate（权限门控），为 RunLoop 提供运行时上下文。
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
	"argo/src/knot"
	"argo/src/vault"
)

type Voyage interface {
	// 持久化保存
	Save() error
	// 加载会话记录
	Load() ([]knot.Message, error)
	// 会话重命名
	Rename(name string)
	// 返回ID
	ID() string
	// 工具使用规则审核
	Evaluate(knot.ToolUse) (int, string)
	// 当前工作路径
	WorkingDir() string
	// 添加对话记录
	Append([]vault.Record) error
	// 用户审批记录
	UserApprovals() []brig.ApprovalEntry
	// 用户审批通过
	Approve(tu knot.ToolUse)
}

// SessionInfo 会话摘要，供 List 展示。
type SessionInfo struct {
	// 会话ID
	ID string
	// 用户级配置目录
	ArgoDir string
	// 当前工作目录
	WorkingDir string
	// 创建时间
	CreatedAt time.Time
	// 最近变更时间
	LastActiveAt time.Time
	// 会话名
	Name string
}

// Session 会话运行时上下文。嵌入 SessionInfo 自动提升元数据字段。
type Session struct {
	SessionInfo
	Vault vault.Vault
	Gate  brig.Gate
}

// SessionOption 函数选项，用于注入 Vault / Gate 等依赖。
type SessionOption func(*Session)

// WithVault 覆盖默认的 FileVault。
func WithVault(v vault.Vault) SessionOption {
	return func(s *Session) { s.Vault = v }
}

// WithGate 覆盖默认的 Warden + BuiltinRuleLoader。
func WithGate(g brig.Gate) SessionOption {
	return func(s *Session) { s.Gate = g }
}

// NewSession 创建新会话：生成 ID → 建目录 → 写 session.json → 组装 Session。
func NewSession(argoDir, workingDir string, opts ...SessionOption) (*Session, error) {
	id := genSessionID()

	// 创建会话目录
	sessionDir := sessionDir(argoDir, id)
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		slog.Error("new session mkdir failed", "error", err.Error(), "dir", sessionDir)
		return nil, fmt.Errorf("new session failed: %w", err)
	}

	now := time.Now()
	info := SessionInfo{
		ID:           id,
		WorkingDir:   workingDir,
		CreatedAt:    now,
		LastActiveAt: now,
		ArgoDir:      argoDir,
	}
	if err := saveSessionInfo(sessionDir, info); err != nil {
		slog.Error("save session info failed", "error", err.Error(), "session dir", sessionDir)
		return nil, err
	}

	s := &Session{
		SessionInfo: info,
		Vault:       vault.NewFileVault(sessionDir),
		Gate:        brig.NewWarden(workingDir, &brig.BuiltinRuleLoader{}),
	}
	// 应用 Option 覆盖默认值
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// Resume 恢复已有会话为完整 Session（含 Vault + Engine）。
// 从 session.json 读取元数据，WorkingDir 自动用于创建 Engine。
func ResumeSession(argoDir, id string, opts ...SessionOption) (*Session, error) {
	dir := sessionDir(argoDir, id)
	info, err := loadSessionInfo(dir)
	if err != nil {
		return nil, fmt.Errorf("voyage resume: %w", err)
	}
	session := &Session{
		SessionInfo: info,
		Vault:       vault.NewFileVault(dir),
		Gate:        brig.NewWarden(info.WorkingDir, &brig.BuiltinRuleLoader{}),
	}
	// 应用 Option 覆盖默认值
	for _, opt := range opts {
		opt(session)
	}

	// 恢复已持久化的用户审批记录（新会话文件不存在为正常情况）
	approvals, err := loadApprovals(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("voyage resume: load approvals: %w", err)
	}

	if len(approvals) > 0 {
		session.Gate.Restore(approvals)
	}
	return session, nil
}

// Resume 恢复已有会话为完整 Session（含 Vault + Engine）。
// 从 session.json 读取元数据，WorkingDir 自动用于创建 Engine。
func ResumeVoyage(id string, opts ...SessionOption) (Voyage, error) {
	dir := sessionDir(knot.ArgoDir(), id)
	info, err := loadSessionInfo(dir)
	if err != nil {
		return nil, fmt.Errorf("voyage resume: %w", err)
	}
	session := &Session{
		SessionInfo: info,
		Vault:       vault.NewFileVault(dir),
		Gate:        brig.NewWarden(info.WorkingDir, &brig.BuiltinRuleLoader{}),
	}
	// 应用 Option 覆盖默认值
	for _, opt := range opts {
		opt(session)
	}

	// 恢复已持久化的用户审批记录（新会话文件不存在为正常情况）
	approvals, err := loadApprovals(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("voyage resume: load approvals: %w", err)
	}

	if len(approvals) > 0 {
		session.Gate.Restore(approvals)
	}
	return session, nil
}

func (s *Session) Approve(tu knot.ToolUse) {
	s.Gate.Approve(tu)
}

func (s *Session) UserApprovals() []brig.ApprovalEntry {
	return s.Gate.Snapshot()
}

func (s *Session) ID() string {
	return s.SessionInfo.ID
}

func (s *Session) Save() error {
	sessionDir := sessionDir(knot.ArgoDir(), s.SessionInfo.ID)
	return saveSessionInfo(sessionDir, s.SessionInfo)
}

func (s *Session) Load() ([]knot.Message, error) {
	return s.Vault.Load()
}

func (s *Session) Rename(name string) {
	s.SessionInfo.Name = name
}

func (s *Session) Evaluate(tu knot.ToolUse) (int, string) {
	return s.Gate.Evaluate(tu)
}

func (s *Session) WorkingDir() string {
	return s.SessionInfo.WorkingDir
}

// SaveApprovals 持久化 Gate 中的用户审批记录到 session 目录。
func SaveApprovals(v Voyage) error {
	return saveApprovals(sessionDir(knot.ArgoDir(), v.ID()), v.UserApprovals())
}

// List 列出所有会话信息，按最后活跃时间倒序。
func ListSessions(argoDir string) ([]SessionInfo, error) {
	root := filepath.Join(argoDir, "sessions")
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
	return saveSessionInfo(sessionDir(knot.ArgoDir(), s.SessionInfo.ID), s.SessionInfo)
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

func sessionDir(argoDir, id string) string {
	return filepath.Join(argoDir, "sessions", id)
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
