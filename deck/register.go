package deck

import "reflect"

// Register 将 Tool 实例注册到进程唯一的 tools map 中。
// 同名冲突时返回 error（builtin > 外部，CLI 和 MCP 同名冲突报错）。
func Register(t Tool) error {
	// 处理 nil 和 typed nil
	if t == nil {
		return &ToolError{Err: "tool is nil"}
	}
	// typed nil: interface 非 nil 但底层值为 nil，调用 Name() 会 panic
	if reflect.ValueOf(t).IsNil() {
		return &ToolError{Err: "tool is nil"}
	}

	// 获取 name
	name := t.Name()
	if name == "" {
		return &ToolError{Err: "tool name is empty"}
	}

	// 检查冲突
	if existing, ok := tools[name]; ok {
		// 已有 builtin → 新工具非 builtin 则拒绝
		if existing.Source() == ToolSourceBuiltin && t.Source() != ToolSourceBuiltin {
			return &ToolError{Err: "cannot overwrite builtin tool: " + name}
		}
		// 同名非 builtin（CLI/MCP）拒绝
		if existing.Source() != ToolSourceBuiltin {
			return &ToolError{Err: "duplicate tool name: " + name}
		}
	}

	// 新 key 时追加注册顺序
	if _, exists := tools[name]; !exists {
		toolsOrder = append(toolsOrder, name)
	}
	tools[name] = t
	return nil
}

// ToolError 工具注册/查找时的错误。
type ToolError struct {
	Err string
}

func (e *ToolError) Error() string {
	return e.Err
}
