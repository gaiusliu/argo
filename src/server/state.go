package server

import (
	"encoding/json"
	"os"
	"path/filepath"

	"argo/src/helm"
)

// statePath 返回 session 目录下状态文件的路径。
func statePath(baseDir, sessionID string) string {
	return filepath.Join(baseDir, "sessions", sessionID, "state.json")
}

// saveLoopState 将 LoopState 序列化为 JSON 写入 session 目录。
func saveLoopState(baseDir, sessionID string, st *helm.LoopState) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(baseDir, sessionID), data, 0644)
}

// loadLoopState 从 session 目录读取并反序列化 LoopState。
func loadLoopState(baseDir, sessionID string) (*helm.LoopState, error) {
	data, err := os.ReadFile(statePath(baseDir, sessionID))
	if err != nil {
		return nil, err
	}
	var st helm.LoopState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}
