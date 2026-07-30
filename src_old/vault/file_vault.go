package vault

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"argo/src_old/knot"
)

// FileVault Vault 接口的文件系统实现。
// 构造时绑定到 session 目录（~/.argo/sessions/{sessionID}/），所有操作限定在此目录内：
//
//	{sessionDir}/
//	    └── transcript.jsonl          — 对话记录
type FileVault struct {
	sessionDir string // session 目录，如 ~/.argo/sessions/sess_xxx/
}

// NewFileVault 创建 FileVault 实例。dir 为 session 目录路径。
func NewFileVault(dir string) *FileVault {
	return &FileVault{sessionDir: dir}
}

// Append 追加 records 到 transcript.jsonl。
func (v *FileVault) Append(records []Record) error {
	return writeRecords(v.sessionDir, records)
}

// writeRecords 将 records 逐行追加写入 transcript.jsonl。
func writeRecords(dir string, records []Record) error {
	f, err := os.OpenFile(filepath.Join(dir, "transcript.jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("append: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	now := time.Now().UTC()
	for i := range records {
		records[i].Timestamp = now
		if err := enc.Encode(records[i]); err != nil {
			return fmt.Errorf("append: %w", err)
		}
	}
	return nil
}

// Load 读取 transcript.jsonl，映射为 knot.Message。新会话文件不存在时返回空列表。
func (v *FileVault) Load() ([]knot.Message, error) {
	records, err := readRecords(v.sessionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("load: %w", err)
	}
	return recordsToMessages(records), nil
}

func readRecords(dir string) ([]Record, error) {
	f, err := os.Open(filepath.Join(dir, "transcript.jsonl"))
	if err != nil {
		slog.Error("read records failed", "error", err.Error(), "dir", dir)
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
		case RecordModel:
			msgs = append(msgs, modelRecordToMessage(r))
		case RecordTool:
			msgs = append(msgs, knot.Message{
				Role:       knot.MessageRoleTool,
				Content:    r.Content,
				ToolCallID: r.ToolCallID,
			})
		case RecordCompact:
			msgs = append(msgs, knot.Message{
				Role:        knot.MessageRoleCompact,
				Content:     r.CompactSummary,
				CompactFrom: r.CompactFrom,
				CompactTo:   r.CompactTo,
			})
		}
	}
	return msgs
}

func modelRecordToMessage(r Record) knot.Message {
	var details []knot.ToolCall
	for _, tc := range r.ToolCalls {
		argsBytes, err := json.Marshal(tc.Args)
		if err != nil {
			slog.Error("load: skip corrupt tool_call args", "call_id", tc.ToolCallID, "error", err)
			continue
		}
		details = append(details, knot.ToolCall{
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
