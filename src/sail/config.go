package sail

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config config.json 顶层结构
type Config struct {
	Model    string                    `json:"model,omitempty"`
	Provider map[string]ProviderConfig `json:"provider"`
}

// ProviderConfig provider 级别配置
type ProviderConfig struct {
	Name    string                 `json:"name,omitempty"`
	Env     []string               `json:"env,omitempty"`
	Options map[string]any         `json:"options,omitempty"`
	Models  map[string]ModelConfig `json:"models"`
}

// ModelConfig 单个模型配置
type ModelConfig struct {
	Name      string         `json:"name,omitempty"`
	Reasoning bool           `json:"reasoning,omitempty"`
	Limit     *ModelLimit    `json:"limit,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
}

// ModelLimit token 限制，nil 使用 API 默认值
type ModelLimit struct {
	Context *int `json:"context,omitempty"`
	Input   *int `json:"input,omitempty"`
	Output  *int `json:"output,omitempty"`
}

// LoadConfig 加载 ~/.argo/config.json，不存在返回零值 Config
func LoadConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(filepath.Join(home, ".argo", "config.json"))
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// DefaultModel 返回配置中的默认模型名，失败回退 "claude"
func DefaultModel() string {
	cfg, err := LoadConfig()
	if err != nil {
		return "claude"
	}
	if cfg.Model == "" {
		return "claude"
	}
	return cfg.Model
}

// LookupModel 按模型名搜索所有 provider，返回匹配的模型配置
func LookupModel(modelName string) (ModelConfig, bool) {
	cfg, err := LoadConfig()
	if err != nil {
		return ModelConfig{}, false
	}
	for _, p := range cfg.Provider {
		if m, ok := p.Models[modelName]; ok {
			return m, true
		}
	}
	return ModelConfig{}, false
}
