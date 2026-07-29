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

// fileConfigSource 从 ~/.argo/config.json 加载配置。
type fileConfigSource struct {
	path string
}

func newFileConfigSource() (*fileConfigSource, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &fileConfigSource{
		path: filepath.Join(home, ".argo", "config.json"),
	}, nil
}

// Load 实现 pact.ConfigSource，返回 AgentCfg。
func (f *fileConfigSource) Load() (pact.AgentCfg, error) {
	cf, err := f.load()
	if err != nil {
		return pact.AgentCfg{}, err
	}
	return cf.Agent, nil
}

// LoadServer 返回 ServerCfg。
func (f *fileConfigSource) LoadServer() (pact.ServerCfg, error) {
	cf, err := f.load()
	if err != nil {
		return pact.ServerCfg{}, err
	}
	return cf.Server, nil
}

// load 读取并解析配置文件，不存在时返回默认值。
func (f *fileConfigSource) load() (*configFile, error) {
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
