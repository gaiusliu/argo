// Package voyage 会话运行时，管理 voyage 生命周期。
package voyage

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// VoyageInfo 一次航程的持久化元数据。
type VoyageInfo struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"createdAt"`
	LastActiveAt time.Time `json:"lastActiveAt"`
	Name         string    `json:"name,omitempty"`
}

// ── 存储路径 ──

func voyagesDir() string { return filepath.Join(argoHome(), "voyages") }

func voyageDir(id string) string { return filepath.Join(voyagesDir(), id) }

func voyageInfoPath(id string) string { return filepath.Join(voyageDir(id), "voyage.json") }

// ── 生命周期 ──

// NewVoyageID 生成唯一 voyage ID。
func NewVoyageID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("v_%d_%x", time.Now().UTC().UnixMilli(), b)
}

// InitVoyageInfo 创建 voyage 目录并写入初始 voyage.json。
func InitVoyageInfo(id string) error {
	if err := os.MkdirAll(voyageDir(id), 0700); err != nil {
		return fmt.Errorf("voyage init: mkdir: %w", err)
	}
	now := time.Now()
	return saveInfo(VoyageInfo{ID: id, CreatedAt: now, LastActiveAt: now})
}

// ListVoyages 返回所有 voyage，按 LastActiveAt 倒序。
func ListVoyages() ([]VoyageInfo, error) {
	entries, err := os.ReadDir(voyagesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("voyage list: %w", err)
	}
	var infos []VoyageInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := loadInfo(e.Name())
		if err != nil {
			continue
		}
		infos = append(infos, info)
	}
	slices.SortFunc(infos, func(a, b VoyageInfo) int {
		return b.LastActiveAt.Compare(a.LastActiveAt)
	})
	return infos, nil
}

// DeleteVoyage 物理删除 voyage 目录。
func DeleteVoyage(id string) error {
	return os.RemoveAll(voyageDir(id))
}

// RenameVoyage 修改 voyage 名称。
func RenameVoyage(id, name string) error {
	info, err := loadInfo(id)
	if err != nil {
		return err
	}
	info.Name = name
	return saveInfo(info)
}

// ── 内部辅助 ──

func saveInfo(info VoyageInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("save voyage info: %w", err)
	}
	return os.WriteFile(voyageInfoPath(info.ID), data, 0644)
}

func loadInfo(id string) (VoyageInfo, error) {
	data, err := os.ReadFile(voyageInfoPath(id))
	if err != nil {
		return VoyageInfo{}, fmt.Errorf("load voyage info: %w", err)
	}
	var info VoyageInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return VoyageInfo{}, fmt.Errorf("load voyage info: %w", err)
	}
	return info, nil
}
