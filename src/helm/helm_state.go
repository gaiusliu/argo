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
	// 当前的模型
	Model string
	// 记录待执行的 tool call（含 verdict/result）
	ToolUses []knot.ToolUse
	// Saved 是已持久化到 transcript.jsonl 的消息条数，也即 Messages 的游标：
	//   Messages[:Saved]  — 已在 transcript 中
	//   Messages[Saved:]  — 本轮新增、尚未持久化
	// PhaseDone 时以该游标做增量写入，写入成功后更新 Saved。
	Saved int `json:"saved"`
	// 完整对话历史，不持久化到 state.json（transcript.jsonl 是唯一持久化来源）
	Messages []knot.Message `json:"-"`
}

func NewHelmState(model string, messages []knot.Message) *HelmState {
	return &HelmState{
		Phase:    HelmPhaseModel,
		Model:    model,
		Messages: messages,
	}
}

func (st *HelmState) Save(sessionID string) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(knot.ArgoDir(), sessionID), data, 0644)
}

func (st *HelmState) Load(sessionID string) error {
	data, err := os.ReadFile(statePath(knot.ArgoDir(), sessionID))
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
