package vault

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	"argo/src/knot"
)

// FileVault Vault 接口的文件系统实现。
// 所有数据存储在 dir（~/.argo/）下：
//
//	~/.argo/sessions/{sessionID}/     — 会话数据
//	    ├── session.json              — SessionInfo 缓存
//	    └── transcript.jsonl          — 对话记录
type FileVault struct {
	dir string // ~/.argo/ 根目录
}

// NewFileVault 创建 FileVault 实例。dir 通常为 helm.ArgoDir() 返回值。
func NewFileVault(dir string) *FileVault {
	return &FileVault{dir: dir}
}

// genSessionID 生成唯一 session ID，格式：sess_<时间戳hex>_<随机hex>
func genSessionID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("sess_%x_%x", b, b)
}

// sessionRoot 返回所有 session 的根目录。
func (v *FileVault) sessionRoot() string {
	return filepath.Join(v.dir, "sessions")
}

// sessionDir 返回指定 session 的存储目录。
func (v *FileVault) sessionDir(sessionID string) string {
	return filepath.Join(v.sessionRoot(), sessionID)
}

// CreateSession 创建新会话，返回 session ID。
func (v *FileVault) CreateSession(workingDir string) (string, error) {
	// 1. 生成 ID + 创建目录
	id := genSessionID()
	dir := v.sessionDir(id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	// 2. 写入初始 SessionInfo
	now := time.Now()
	info := SessionInfo{
		ID:           id,
		WorkingDir:   workingDir,
		CreatedAt:    now,
		LastActiveAt: now,
	}

	return id, saveSessionInfo(dir, info)
}

// saveSessionInfo 将 SessionInfo 序列化写入 session.json。
func saveSessionInfo(dir string, info SessionInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "session.json"), data, 0644)
}

// loadSessionInfo 从 session.json 读取并反序列化 SessionInfo。
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

// GetSession 获取单个会话信息。
func (v *FileVault) GetSession(sessionID string) (SessionInfo, error) {
	return loadSessionInfo(v.sessionDir(sessionID))
}

// DeleteSession 立刻物理删除会话目录及其所有数据。
func (v *FileVault) DeleteSession(sessionID string) error {
	return os.RemoveAll(v.sessionDir(sessionID))
}

// Append 追加本轮 records 到 transcript.jsonl，并更新 session.json 动态字段。
func (v *FileVault) Append(sessionID string, records []Record) error {
	dir := v.sessionDir(sessionID)
	info, err := loadSessionInfo(dir)
	if err != nil {
		return fmt.Errorf("append: %w", err)
	}

	// 分配 seq + 写入 JSONL
	if err := writeRecords(dir, info.MessageCount, records); err != nil {
		return err
	}

	// 更新动态字段
	info.LastActiveAt = time.Now()
	info.MessageCount += len(records)
	for _, r := range records {
		info.Tokens.Input += int64(r.InputTokens)
		info.Tokens.Output += int64(r.OutputTokens)
	}
	return saveSessionInfo(dir, info)
}

// writeRecords 将 records 逐行追加写入 transcript.jsonl，seq 从 base 开始自增。
func writeRecords(dir string, baseSeq int, records []Record) error {
	f, err := os.OpenFile(filepath.Join(dir, "transcript.jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("append: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	now := time.Now().UTC()
	for i := range records {
		records[i].Seq = baseSeq + i + 1
		records[i].Timestamp = now
		if err := enc.Encode(records[i]); err != nil {
			return fmt.Errorf("append: %w", err)
		}
	}
	return nil
}

// ListSessions 列出所有会话，按最后活跃时间倒序。
func (v *FileVault) ListSessions() ([]SessionInfo, error) {
	entries, err := os.ReadDir(v.sessionRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	var sessions []SessionInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := loadSessionInfo(filepath.Join(v.sessionRoot(), e.Name()))
		if err != nil {
			slog.Error("list sessions: skip corrupt dir", "dir", e.Name(), "error", err)
			continue
		}
		sessions = append(sessions, info)
	}

	slices.SortFunc(sessions, func(a, b SessionInfo) int {
		return b.LastActiveAt.Compare(a.LastActiveAt)
	})
	return sessions, nil
}

func (v *FileVault) Load(sessionID string) ([]knot.Message, error) {
	records, err := readRecords(v.sessionDir(sessionID))
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}
	return recordsToMessages(records), nil
}

func readRecords(dir string) ([]Record, error) {
	f, err := os.Open(filepath.Join(dir, "transcript.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var records []Record
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var r Record
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			slog.Error("load: skip corrupt line", "error", err)
			continue
		}
		records = append(records, r)
	}
	return records, scanner.Err()
}

func recordsToMessages(records []Record) []knot.Message {
	var msgs []knot.Message
	for _, r := range records {
		switch r.Type {
		case RecordUser:
			msgs = append(msgs, knot.Message{
				Role:    knot.MessageRoleUser,
				Content: r.Content,
			})
		case RecordLLM:
			msgs = append(msgs, llmRecordToMessage(r))
		case RecordTool:
			msgs = append(msgs, knot.Message{
				Role:       knot.MessageRoleTool,
				Content:    r.Content,
				ToolCallID: r.ToolCallID,
			})
		}
	}
	return msgs
}

func llmRecordToMessage(r Record) knot.Message {
	var details []knot.ToolCallDetail
	for _, tc := range r.ToolCalls {
		argsBytes, err := json.Marshal(tc.Args)
		if err != nil {
			slog.Error("load: skip corrupt tool_call args", "call_id", tc.ToolCallID, "error", err)
			continue
		}
		details = append(details, knot.ToolCallDetail{
			ID:        tc.ToolCallID,
			Name:      tc.Name,
			Arguments: string(argsBytes),
		})
	}
	return knot.Message{
		Role:      knot.MessageRoleAssistant,
		Content:   r.Content,
		ToolCalls: details,
	}
}

// 编译时检查 FileVault 实现 Vault 接口
var _ Vault = (*FileVault)(nil)
