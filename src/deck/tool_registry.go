package deck

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type ToolsConfig struct {
	CLI map[string]CLIToolEntry `json:"cli"`
}

type CLIToolEntry struct {
	Description string `json:"description"`
}

// ConfigPath 返回全局配置文件路径：$ARGO_HOME/config.json 或 ~/.argo/config.json
func ConfigPath() (string, error) {
	if home := os.Getenv("ARGO_HOME"); home != "" {
		return filepath.Join(home, "config.json"), nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config path: %w", err)
	}
	return filepath.Join(dir, ".argo", "config.json"), nil
}

// LoadConfig 读取并解析全局配置文件
func LoadConfig(path string) (ToolsConfig, error) {
	var cfg ToolsConfig

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("load config failed: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config failed: %w", err)
	}

	return cfg, nil
}

// validateCLIEntry 校验单个 CLI 工具条目，name 即 executable
func validateCLIEntry(name string, e CLIToolEntry) []error {
	var errs []error

	if name == "" {
		errs = append(errs, fmt.Errorf("missing name"))
	}

	if _, err := exec.LookPath(name); err != nil {
		errs = append(errs, fmt.Errorf("binary not found"))
	}

	if e.Description == "" {
		errs = append(errs, fmt.Errorf("description empty"))
	}

	return errs
}

// ValidateConfig 校验全部 CLI 工具条目，收集所有错误后一并返回
func ValidateConfig(cfg ToolsConfig) []error {
	var errs []error

	for name, e := range cfg.CLI {
		for _, err := range validateCLIEntry(name, e) {
			errs = append(errs, fmt.Errorf("cli.%s: %w", name, err))
		}
	}

	return errs
}
