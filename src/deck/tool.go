package deck

import (
	"context"
	"fmt"
	"sync"
)

var (
	regMu    sync.RWMutex
	tools    []Tool
	toolsIdx = make(map[string]int) // name → index in tools
)

type ToolSource int

const (
	SourceBuiltin ToolSource = iota
	SourceCLI
)

type Tool interface {
	Name() string
	Description() string
	Source() ToolSource
	Execute(ctx context.Context, params map[string]any) (ToolResult, error)
}

type ResultStatus int

const (
	StatusSuccess ResultStatus = iota
	StatusError
)

type ToolResult struct {
	Output   string
	Status   ResultStatus
	Metadata map[string]any
}

func List() []Tool {
	regMu.RLock()
	defer regMu.RUnlock()
	return tools
}

func Lookup(name string) (Tool, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	i, ok := toolsIdx[name]
	if !ok {
		return nil, false
	}
	return tools[i], true
}

// RegisterTool 注册工具。builtin 优先：与内置工具同名的 CLI 工具会被拒绝
func RegisterTool(t Tool) error {
	regMu.Lock()
	defer regMu.Unlock()

	if i, ok := toolsIdx[t.Name()]; ok {
		existing := tools[i]
		// builtin 覆盖 CLI：builtin 优先
		if t.Source() == SourceBuiltin && existing.Source() == SourceCLI {
			tools[i] = t
			return nil
		}
		// 其余情况均为冲突
		if t.Source() == SourceBuiltin {
			return fmt.Errorf("builtin tool %q already registered", t.Name())
		}
		return fmt.Errorf("CLI tool %q conflicts with builtin tool", t.Name())
	}

	toolsIdx[t.Name()] = len(tools)
	tools = append(tools, t)
	return nil
}

func RegisterTools() error {
	for _, bt := range builtinTools {
		if err := RegisterTool(bt); err != nil {
			return fmt.Errorf("register tools failed: %w", err)
		}
	}

	// 读配置，校验CLI工具参数
	cfgPath, err := ConfigPath()
	if err != nil {
		return fmt.Errorf("register tools failed: %w", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("register tools failed: %w", err)
	}

	if errs := ValidateConfig(cfg); len(errs) > 0 {
		return fmt.Errorf("register tools failed: %v", errs)
	}

	// 校验通过后逐条注册 CLI 工具
	for name, e := range cfg.CLI {
		if err := RegisterTool(cliTool{
			name:        name,
			description: e.Description,
		}); err != nil {
			return fmt.Errorf("register tools failed: %w", err)
		}
	}
	return nil
}
