package knot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ArgoDir 返回 ~/.argo/ 全局配置目录，home 取不到返回空字符串
func ArgoDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".argo")
}

// LoadConfig 读取 ~/.argo/config.json，不存在则用默认配置生成并写盘
func LoadConfig() (*Config, error) {
	// 1. 默认骨架
	cfg := GenerateDefaultConfig()

	// 2. 读用户配置
	path, err := ConfigPath()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("load config: %w", err)
		}
		// 文件不存在：写骨架到磁盘
		if err := writeConfig(path, cfg); err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
		return cfg, nil
	}

	// 3. 合并：用户配置覆盖默认骨架
	var userCfg Config
	if err := json.Unmarshal(data, &userCfg); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	mergeConfig(cfg, &userCfg)
	return cfg, nil
}

// mergeConfig 将 src 字段级合并到 dst，src 有的字段覆盖 dst
func mergeConfig(dst, src *Config) {
	if src.Sail.Model != "" {
		dst.Sail.Model = src.Sail.Model
	}
	// 逐 provider 合并
	for name, sp := range src.Sail.Provider {
		dp, ok := dst.Sail.Provider[name]
		if !ok {
			dst.Sail.Provider[name] = sp
			continue
		}
		// 字段级合并：Name / Env / Options 用户覆盖
		if sp.Name != "" {
			dp.Name = sp.Name
		}
		if len(sp.Env) > 0 {
			dp.Env = sp.Env
		}
		if sp.Options != nil {
			dp.Options = sp.Options
		}
		// Models：用户定义什么就用什么
		if sp.Models != nil {
			dp.Models = sp.Models
		}
		dst.Sail.Provider[name] = dp
	}
	// Deck 子配置
	if src.Deck.CLI != nil {
		dst.Deck.CLI = src.Deck.CLI
	}
	// Log 子配置：非空/非零字段覆盖默认值
	if src.Log.Dir != "" {
		dst.Log.Dir = src.Log.Dir
	}
	if src.Log.Level != "" {
		dst.Log.Level = src.Log.Level
	}
	if src.Log.MaxSize > 0 {
		dst.Log.MaxSize = src.Log.MaxSize
	}
	if src.Log.MaxAge > 0 {
		dst.Log.MaxAge = src.Log.MaxAge
	}
}

// writeConfig 将配置序列化写入磁盘
func writeConfig(path string, cfg *Config) error {
	// 确保父目录存在
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// 已知 provider 的环境变量映射
var knownProviders = map[string]string{
	"anthropic": "ANTHROPIC_API_KEY",
	"openai":    "OPENAI_API_KEY",
	"deepseek":  "DEEPSEEK_API_KEY",
}

// GenerateDefaultConfig 扫描环境变量，生成默认配置
func GenerateDefaultConfig() *Config {
	cfg := &Config{
		Log: LogConfig{
			Level:   "error",
			MaxSize: 100,
			MaxAge:  7,
		},
		Sail: SailConfig{
			Provider: make(map[string]ProviderConfig),
		},
	}

	for provider, envVar := range knownProviders {
		// 检查环境变量是否存在
		if _, ok := os.LookupEnv(envVar); !ok {
			continue
		}
		cfg.Sail.Provider[provider] = ProviderConfig{
			Name:   provider,
			Env:    []string{envVar},
			Models: make(map[string]ModelConfig),
		}
	}

	return cfg
}

// LookupModel 按模型名搜索所有 provider，返回匹配的模型配置
func LookupModel(modelName string) (ModelConfig, bool) {
	cfg, err := GetConfig()
	if err != nil {
		return ModelConfig{}, false
	}
	for _, p := range cfg.Sail.Provider {
		if m, ok := p.Models[modelName]; ok {
			return m, true
		}
	}
	return ModelConfig{}, false
}

// 全局配置缓存
var (
	globalCfg  *Config
	globalOnce sync.Once
	globalErr  error
)

// GetConfig 返回已加载的全局配置，首次调用从磁盘加载，后续返回缓存
func GetConfig() (*Config, error) {
	globalOnce.Do(func() {
		globalCfg, globalErr = LoadConfig()
	})
	return globalCfg, globalErr
}
