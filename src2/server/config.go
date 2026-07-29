package server

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ConfigLoader 配置加载接口，未来可扩展为 DB/Redis 等实现。
type ConfigLoader interface {
	Load(name string, target any) error
}

// FileConfigLoader 从 ~/.argo/ 目录加载 JSON 配置文件。
type FileConfigLoader struct {
	dir string
}

// NewFileConfigLoader 创建文件配置加载器。
func NewFileConfigLoader() (*FileConfigLoader, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &FileConfigLoader{dir: filepath.Join(home, ".argo")}, nil
}

// Load 实现 ConfigLoader。
func (f *FileConfigLoader) Load(name string, target any) error {
	data, err := os.ReadFile(filepath.Join(f.dir, name))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
