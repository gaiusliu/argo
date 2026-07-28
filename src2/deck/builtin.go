package deck

// BuiltinTools 返回编译期内置工具实例，hauler 函数指针由上层注入。
func BuiltinTools() []Hand {
	return []Hand{
		builtin("bash", "Execute shell commands", cmdSchema()),
		builtin("read", "Read file contents with line numbers", pathSchema()),
		builtin("write", "Create or overwrite a file", pathContentSchema()),
		builtin("grep", "Search files with regex", patternPathSchema()),
		builtin("web_fetch", "Fetch URL content", urlSchema()),
	}
}

type builtinTool struct {
	name   string
	desc   string
	schema map[string]any
}

func builtin(name, desc string, schema map[string]any) *builtinTool {
	return &builtinTool{name: name, desc: desc, schema: schema}
}

func (b *builtinTool) Name() string                     { return b.name }
func (b *builtinTool) Description() string              { return b.desc }
func (b *builtinTool) ParametersSchema() map[string]any { return b.schema }
func (b *builtinTool) Source() string                   { return "builtin" }

// Haul 为桩实现，后续从 src/deck/ 搬运真实 handler 逻辑。
// TODO：从 src/deck/ 搬运真实工具执行器。
func (b *builtinTool) Haul(_ map[string]any) (Catch, error) {
	return Catch{Output: "stub: " + b.name + " executed"}, nil
}

// ── JSON Schema 辅助 ──

func cmdSchema() map[string]any {
	return toolSchema([]string{"cmd"}, map[string]string{"cmd": "string"})
}
func pathSchema() map[string]any {
	return toolSchema([]string{"path"}, map[string]string{"path": "string"})
}
func pathContentSchema() map[string]any {
	return toolSchema([]string{"path", "content"},
		map[string]string{"path": "string", "content": "string"})
}
func patternPathSchema() map[string]any {
	return toolSchema([]string{"pattern", "path"},
		map[string]string{"pattern": "string", "path": "string"})
}
func urlSchema() map[string]any {
	return toolSchema([]string{"url"}, map[string]string{"url": "string"})
}

func toolSchema(required []string, props map[string]string) map[string]any {
	pp := make(map[string]any, len(props))
	for k, v := range props {
		pp[k] = map[string]string{"type": v}
	}
	s := map[string]any{"type": "object", "properties": pp}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}
