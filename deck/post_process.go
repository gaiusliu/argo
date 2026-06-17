package deck

import "argo/deck/truncation"

// truncWriter 是全局的 truncation 写入器单例，postProcess 截断时通过它持久化完整输出。
// 调用方可替换（如测试中注入 mock）。
var truncWriter truncation.Writer = truncation.NewDefault()

// postProcess 对 ToolResult.Output 按 TruncationConfig 做截断。
// stub — 待 gen-code 实现，当前仅返回原值以让包可编译。
func postProcess(result ToolResult, cfg TruncationConfig) ToolResult {
	// stub
	return result
}
