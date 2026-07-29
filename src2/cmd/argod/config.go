package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"argo/src2/pact"
)

// fileConfig 底层文件加载，load 为通用方法，解析结果写入 target。
type fileConfig struct {
	dir string
}

func newFileConfig() (*fileConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &fileConfig{dir: filepath.Join(home, ".argo")}, nil
}

// load 读取 dir/name 文件并解析到 target。文件不存在返回 nil。
func (f *fileConfig) load(name string, target any) error {
	data, err := os.ReadFile(filepath.Join(f.dir, name))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// ── 进程级：Server 配置 ──

type serverConfigSource struct{ fc *fileConfig }

func (s *serverConfigSource) Load() (pact.ServerCfg, error) {
	var scfg pact.ServerCfg
	if err := s.fc.load("server.json", &scfg); err != nil {
		return pact.ServerCfg{}, err
	}
	return scfg, nil
}

// ── 请求级：Agent 配置 ──

type agentConfigSource struct{ fc *fileConfig }

// Load 实现 pact.ConfigSource，每次请求热重载 agent.json。
func (a *agentConfigSource) Load() (pact.AgentCfg, error) {
	var acfg pact.AgentCfg
	if err := a.fc.load("agent.json", &acfg); err != nil {
		return pact.AgentCfg{}, err
	}
	return acfg, nil
}
