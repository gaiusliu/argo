package server

import (
	"encoding/json"
	"os"
	"path/filepath"

	"argo/src2/pact"
)

// ConfigLoader 底层配置文件加载器。
type ConfigLoader struct {
	dir string
}

// NewConfigLoader 创建配置加载器，默认从 ~/.argo/ 读取。
func NewConfigLoader() (*ConfigLoader, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &ConfigLoader{dir: filepath.Join(home, ".argo")}, nil
}

// Load 读取 dir/name 文件，解析到 target。文件不存在返回 nil。
func (c *ConfigLoader) Load(name string, target any) error {
	data, err := os.ReadFile(filepath.Join(c.dir, name))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// AgentConfigSource 加载 AgentCfg，实现 pact.ConfigSource 接口。
// 每次请求热重载 agent.json。
type AgentConfigSource struct {
	Loader *ConfigLoader
}

// Load 实现 pact.ConfigSource。
func (a *AgentConfigSource) Load() (pact.AgentCfg, error) {
	var acfg pact.AgentCfg
	if err := a.Loader.Load("agent.json", &acfg); err != nil {
		return pact.AgentCfg{}, err
	}
	return acfg, nil
}
