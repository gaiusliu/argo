package board

import (
	"encoding/json"
	"fmt"

	"argo/src2/pact"
)

// Entries 导出所有船长许可记录，供 Seal 持久化使用。
func (b *Board) Entries() []AyeEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]AyeEntry, 0, len(b.aye))
	for key := range b.aye {
		hand, pattern, err := splitScopeKey(key)
		if err != nil {
			// TODO：记录日志
			continue
		}
		result = append(result, AyeEntry{
			Hand:    hand,
			Pattern: pattern,
		})
	}
	return result
}

// Seal 将当前审批记录持久化到快照。
func (b *Board) Seal() error {
	data, err := json.Marshal(b.Entries())
	if err != nil {
		return fmt.Errorf("seal approvals: %w", err)
	}
	return b.cp.Save(string(data))
}

// Recall 从快照数据恢复审批记录。
func (b *Board) Recall(data string) error {
	if data == "" {
		return nil
	}
	var entries []AyeEntry
	if err := json.Unmarshal([]byte(data), &entries); err != nil {
		return fmt.Errorf("recall approvals: %w", err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range entries {
		key := scopeKey(pact.Omen{Name: e.Hand, Arguments: e.Pattern})
		b.aye[key] = struct{}{}
	}
	return nil
}
