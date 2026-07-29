package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"argo/src2/pact"
)

// configFile 配置文件顶层结构。
type configFile struct {
	Server pact.ServerCfg `json:"server"`
	Agent  pact.AgentCfg  `json:"agent"`
}

// ── 底层文件加载 ──

// fileConfig 从 ~/.argo/config.json 读取原始配置。
type fileConfig struct {
	path string
}

func newFileConfig() (*fileConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &fileConfig{path: filepath.Join(home, ".argo", "config.json")}, nil
}

// load 读取并解析配置文件，不存在时返回默认值。
func (f *fileConfig) load() (*configFile, error) {
	data, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return &configFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	var cf configFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, err
	}
	return &cf, nil
}

// ── 进程级：Server 配置 ──

// serverConfigSource 加载进程级 ServerCfg。
type serverConfigSource struct {
	fc *fileConfig
}

func (s *serverConfigSource) Load() (pact.ServerCfg, error) {
	cf, err := s.fc.load()
	if err != nil {
		return pact.ServerCfg{}, err
	}
	return cf.Server, nil
}

// ── 请求级：Agent 配置 ──

// agentConfigSource 实现 pact.ConfigSource，每次请求热重载 AgentCfg。
type agentConfigSource struct {
	fc *fileConfig
}

// Load 实现 pact.ConfigSource。
func (a *agentConfigSource) Load() (pact.AgentCfg, error) {
	cf, err := a.fc.load()
	if err != nil {
		return pact.AgentCfg{}, err
	}
	return cf.Agent, nil
}
