package helm

import (
	"argo/src/knot"
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	HelmPhaseDefault int = iota
	HelmPhaseModel
	HelmPhaseExecTools
	HelmPhaseAsking
	HelmPhaseDone
)

type HelmState struct {
	// 当前处于哪个环节
	// HelmPhaseModel | HelmPhaseExecTools | HelmPhaseAsking | HelmPhaseDone
	Phase int
	// 系统消息
	SystemPrompt knot.Message
	// 消息列表
	Messages []knot.Message
	// 当前的模型
	Model string
	// 记录待执行的tool call
	ToolUses []knot.ToolUse
}

func NewHelmState(model string, messages []knot.Message) *HelmState {
	return &HelmState{
		Phase:    HelmPhaseModel,
		Model:    model,
		Messages: messages,
	}
}

func (st *HelmState) Save(sessionID, argoDir string) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(argoDir, sessionID), data, 0644)
}

func (st *HelmState) Load(sessionID, argoDir string) error {
	data, err := os.ReadFile(statePath(argoDir, sessionID))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, st); err != nil {
		return err
	}
	return nil
}

// statePath 返回 session 目录下状态文件的路径。
func statePath(argoDir, sessionID string) string {
	return filepath.Join(argoDir, "sessions", sessionID, "state.json")
}
