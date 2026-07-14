// Package voyage 管理会话运行时——创建、恢复、持久化、门控、状态机。
// Voyage 聚合 Vault（存储）+ Gate（门控）+ Tale（对话历史）+ Helm 控制流状态。
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
	"argo/src/tale"
	"argo/src/vault"
)

// 状态机 Phase 常量
const (
	PhaseModel = iota + 1
	PhaseExecTools
	PhaseAsking
	PhaseDone
)

// 会话摘要，供 List 展示。
type Info struct {
	ID           string
	ArgoDir      string
	WorkingDir   string
	CreatedAt    time.Time
	LastActiveAt time.Time
	Name         string
}

// Voyage 会话运行时上下文。嵌入 Info 自动提升元数据字段。
type Voyage struct {
	Info
	Vault vault.Vault
	Gate  brig.Gate
	tale  *tale.Tale // 对话历史内存态

	// 状态机控制流（原 HelmState 字段）
	Phase           int
	Model           string
	ToolUses        []knot.ToolUse
	LastUsage       knot.TokenUsage // 最近一次 Chat API 的 token 消耗
	Saved           int             // 持久化游标（SaveState/LoadState 时与 Tale 同步）
	PendingMessages []knot.Message  // Asking 阶段尚未落盘的消息（user + assistant）
}

// Option 函数选项，用于注入 Vault / Gate 等依赖。
type Option func(*Voyage)

// WithVault 覆盖默认的 FileVault。
func WithVault(v vault.Vault) Option {
	return func(voy *Voyage) { voy.Vault = v }
}

// WithGate 覆盖默认的 Warden + BuiltinRuleLoader。
func WithGate(g brig.Gate) Option {
	return func(voy *Voyage) { voy.Gate = g }
}

// New 创建新会话：生成 ID → 建目录 → 写 session.json → 组装 Voyage。
func New(argoDir, workingDir string, opts ...Option) (*Voyage, error) {
	id := genSessionID()

	sessionDir := sessionDir(argoDir, id)
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		slog.Error("new session mkdir failed", "error", err.Error(), "dir", sessionDir)
		return nil, fmt.Errorf("voyage new: %w", err)
	}

	now := time.Now()
	info := Info{
		ID:           id,
		WorkingDir:   workingDir,
		CreatedAt:    now,
		LastActiveAt: now,
		ArgoDir:      argoDir,
	}
	if err := saveInfo(sessionDir, info); err != nil {
		slog.Error("save session info failed", "error", err.Error(), "session dir", sessionDir)
		return nil, err
	}

	v := &Voyage{
		Info:  info,
		Vault: vault.NewFileVault(sessionDir),
		Gate:  brig.NewWarden(workingDir, &brig.BuiltinRuleLoader{}),
	}
	for _, opt := range opts {
		opt(v)
	}
	// 从 vault 加载消息构造 Tale（新会话为空列表）
	msgs, err := v.Vault.Load()
	if err != nil {
		return nil, fmt.Errorf("voyage new: load tale: %w", err)
	}
	v.tale = tale.New(msgs, "")
	return v, nil
}

// Resume 从 session.json 恢复完整 Voyage。
func Resume(id string, opts ...Option) (*Voyage, error) {
	dir := sessionDir(knot.ArgoDir(), id)
	info, err := loadInfo(dir)
	if err != nil {
		return nil, fmt.Errorf("voyage resume: %w", err)
	}
	v := &Voyage{
		Info:  info,
		Vault: vault.NewFileVault(dir),
		Gate:  brig.NewWarden(info.WorkingDir, &brig.BuiltinRuleLoader{}),
	}
	for _, opt := range opts {
		opt(v)
	}

	// 从 vault 加载消息构造 Tale
	msgs, err := v.Vault.Load()
	if err != nil {
		return nil, fmt.Errorf("voyage resume: load tale: %w", err)
	}
	v.tale = tale.New(msgs, "")

	// 恢复已持久化的用户审批记录
	approvals, err := loadApprovals(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("voyage resume: load approvals: %w", err)
	}
	if len(approvals) > 0 {
		v.Gate.Restore(approvals)
	}
	return v, nil
}

// ---- 元数据操作 ----

func (v *Voyage) ID() string         { return v.Info.ID }
func (v *Voyage) WorkingDir() string { return v.Info.WorkingDir }
func (v *Voyage) Rename(name string) { v.Info.Name = name }
func (v *Voyage) Tale() *tale.Tale   { return v.tale }

// SaveInfo 持久化 session.json。
func (v *Voyage) SaveInfo() error {
	return saveInfo(sessionDir(knot.ArgoDir(), v.Info.ID), v.Info)
}

// ---- 门控委托 ----

// Evaluate 委托 Gate 裁决工具调用。
func (v *Voyage) Evaluate(tu knot.ToolUse) (int, string) {
	return v.Gate.Evaluate(tu)
}

// Approve 委托 Gate 记录用户审批。
func (v *Voyage) Approve(tu knot.ToolUse) { v.Gate.Approve(tu) }

// UserApprovals 返回 Gate 中的审批快照。
func (v *Voyage) UserApprovals() []brig.ApprovalEntry {
	return v.Gate.Snapshot()
}

// ---- 持久化 ----

// AppendMessages 将消息差量转为 Record 写入 transcript，自动 MarkSaved。
func (v *Voyage) AppendMessages(delta []knot.Message, toolUses map[string]knot.ToolUse, model string) error {
	records := vault.MessagesToRecords(delta, toolUses, model)
	if err := v.Vault.Append(records); err != nil {
		return fmt.Errorf("voyage append: %w", err)
	}
	v.tale.MarkSaved()
	v.LastActiveAt = time.Now()
	return saveInfo(sessionDir(knot.ArgoDir(), v.Info.ID), v.Info)
}

// SaveState 将控制流状态持久化到 state.json（Ask/Done 断点时调用）。
func (v *Voyage) SaveState() error {
	// 序列化前从 Tale 同步游标
	v.Saved = v.tale.Saved()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(knot.ArgoDir(), v.Info.ID), data, 0644)
}

// LoadState 从 state.json 恢复控制流状态（Ask 回复路径调用）。
func (v *Voyage) LoadState() error {
	data, err := os.ReadFile(statePath(knot.ArgoDir(), v.Info.ID))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	// 恢复游标到 Tale
	v.tale.SetSaved(v.Saved)
	// 将 PendingMessages（user + assistant）恢复到 Tale 中
	if len(v.PendingMessages) > 0 {
		v.tale.AppendMessages(v.PendingMessages)
		v.PendingMessages = nil
	}
	return nil
}

// ---- 审批持久化 ----

// SaveApprovals 持久化 Gate 中的用户审批记录。
func SaveApprovals(v *Voyage) error {
	return saveApprovals(sessionDir(knot.ArgoDir(), v.ID()), v.UserApprovals())
}

// ---- 列表 / 改名 / 删除 ----

// List 列出所有会话信息，按最后活跃时间倒序。
func List(argoDir string) ([]Info, error) {
	root := filepath.Join(argoDir, "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("voyage list: %w", err)
	}

	var infos []Info
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := loadInfo(filepath.Join(root, e.Name()))
		if err != nil {
			slog.Error("voyage list: skip corrupt dir", "dir", e.Name(), "error", err)
			continue
		}
		infos = append(infos, info)
	}

	slices.SortFunc(infos, func(a, b Info) int {
		return b.LastActiveAt.Compare(a.LastActiveAt)
	})
	return infos, nil
}

// RenameByID 修改会话名称。
func RenameByID(baseDir, id, name string) error {
	info, err := loadInfo(sessionDir(baseDir, id))
	if err != nil {
		return fmt.Errorf("voyage rename: %w", err)
	}
	info.Name = name
	return saveInfo(sessionDir(baseDir, id), info)
}

// Delete 物理删除会话目录。
func Delete(baseDir, id string) error {
	return os.RemoveAll(sessionDir(baseDir, id))
}

// DeleteState 删除控制流状态文件，用于 Asking 阶段终止。
// 删除后下次 Resume 自然回到断点前的干净状态。
func DeleteState(id string) error {
	path := statePath(knot.ArgoDir(), id)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete state: %w", err)
	}
	return nil
}

// ---- 内部辅助 ----

func approvalsPath(dir string) string      { return filepath.Join(dir, "approvals.json") }
func sessionDir(argoDir, id string) string { return filepath.Join(argoDir, "sessions", id) }
func statePath(argoDir, sessionID string) string {
	return filepath.Join(argoDir, "sessions", sessionID, "state.json")
}
func genSessionID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("sess_%d_%x", time.Now().UTC().UnixMilli(), b)
}

func saveInfo(dir string, info Info) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("save info: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "session.json"), data, 0644)
}

func loadInfo(dir string) (Info, error) {
	data, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		return Info{}, fmt.Errorf("load info: %w", err)
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return Info{}, fmt.Errorf("load info: %w", err)
	}
	return info, nil
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
