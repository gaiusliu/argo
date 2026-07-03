package deck

import (
	"context"
	"fmt"
	"sync"
)

// ToolRegistry 管理工具注册表，支持并发安全读写
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewToolRegistry 创建空的工具注册表——测试等场景可使用独立实例
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]Tool)}
}

// 包级默认注册表实例
var defaultRegistry = NewToolRegistry()

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

// List 返回注册表中所有工具的切片
func List() []Tool {
	return defaultRegistry.List()
}

// Lookup 按名称查找工具
func Lookup(name string) (Tool, bool) {
	return defaultRegistry.Lookup(name)
}

// --- ToolRegistry 方法 ---

// List 返回注册表中所有工具的切片
func (r *ToolRegistry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// Lookup 按名称精确匹配工具，O(1)
func (r *ToolRegistry) Lookup(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// RegisterTool 注册工具到默认注册表
func RegisterTool(t Tool) error {
	return defaultRegistry.RegisterTool(t)
}

// resolveConflict 处理工具注册冲突：builtin 覆盖 CLI（返回 nil），其他情况返回错误
func resolveConflict(existing, incoming Tool) error {
	if incoming.Source() == SourceBuiltin && existing.Source() == SourceCLI {
		return nil
	}
	if incoming.Source() == SourceBuiltin {
		return fmt.Errorf("builtin tool %q already registered", incoming.Name())
	}
	return fmt.Errorf("CLI tool %q conflicts with builtin tool", incoming.Name())
}

// RegisterTool 注册工具。builtin 优先：与内置工具同名的 CLI 工具会被拒绝
func (r *ToolRegistry) RegisterTool(t Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.tools[t.Name()]; ok {
		if err := resolveConflict(existing, t); err != nil {
			return err
		}
	}
	r.tools[t.Name()] = t
	return nil
}

// RegisterTools 默认注册流程：先注册内置工具，再加载外部 CLI 工具
func RegisterTools() error {
	return defaultRegistry.RegisterTools()
}

// RegisterTools 先注册内置工具（遇错即停），再注册 CLI 工具（校验/冲突不打断，收尾汇总）
func (r *ToolRegistry) RegisterTools() error {
	// 内置工具：单个失败即代码 bug，应立即暴露
	for _, bt := range builtinTools {
		if err := r.RegisterTool(bt); err != nil {
			return fmt.Errorf("register tools failed: %w", err)
		}
	}

	// 读配置——找不到或解析不了则直接报错（基础设施问题）
	cfgPath, err := ConfigPath()
	if err != nil {
		return fmt.Errorf("register tools failed: %w", err)
	}
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("register tools failed: %w", err)
	}

	// 逐条校验 + 注册，跳过有问题的工具，错误收尾汇总
	var skipped []error
	for name, e := range cfg.CLI {
		verrs := validateCLIEntry(name, e)
		if len(verrs) > 0 {
			for _, verr := range verrs {
				skipped = append(skipped, fmt.Errorf("validate %s: %w", name, verr))
			}
			continue
		}
		if err := r.RegisterTool(&cliTool{
			name:        name,
			description: e.Description,
		}); err != nil {
			skipped = append(skipped, err)
		}
	}

	if len(skipped) > 0 {
		return fmt.Errorf("register tools failed: %v", skipped)
	}

	return nil
}
