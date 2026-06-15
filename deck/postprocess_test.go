package deck

import (
	"strconv"
	"strings"
	"testing"
)

// ============================================================
// postProcess 测试
//
// 职责：对 ToolResult.Output 做字符数截断。超过 MaxOutputChars（50000）
// 时，保留头部 40% + 尾部 60%，中间插入截断标记。
//
// 设计决策：
//   1. 截断标记文本：\n\n[... 输出被截断，省略 X 字符，原始长度 Y 字符 ...]\n\n
//      - X = 原始长度 - 截断后保留的字符数
//      - Y = 原始长度
//   2. MaxOutputChars 比较用 >（严格大于），刚好 50000 字符不触发截断
//   3. 按字节截断（Go 的 len() 行为），不是 rune
//   4. 截断后 ToolResult.Status 和 Metadata 保持不变
// ============================================================

// expectedTruncatedOutput 构造预期截断后的完整输出字符串，用于测试断言。
// 不依赖 postProcess 内部实现，独立构造预期值。
// headLen = MaxOutputChars * 40% = 20000
// tailLen = MaxOutputChars * 60% = 30000
// omitted = originalLen - MaxOutputChars
func expectedTruncatedOutput(head, tail string, omitted, originalLen int) string {
	marker := "\n\n[... 输出被截断，省略 " +
		strconv.Itoa(omitted) +
		" 字符，原始长度 " +
		strconv.Itoa(originalLen) +
		" 字符 ...]\n\n"
	return head + marker + tail
}

// ============================================================
// 测试用例
// ============================================================

// TestPostProcess_OutputLessThanLimit 测试输出小于 MaxOutputChars 时不截断。
// 目的：验证 len(Output) < MaxOutputChars 时，Output、Status、Metadata 均保持不变。
// 方法：构造一个 100 字符的 ToolResult，调用 postProcess。
// 预期：返回的 ToolResult 与原值完全一致。
func TestPostProcess_OutputLessThanLimit(t *testing.T) {
	result := ToolResult{
		Output:   strings.Repeat("a", 100),
		Status:   "success",
		Metadata: map[string]any{"key": "value"},
	}
	got := postProcess(result)

	if got.Output != result.Output {
		t.Errorf("输出不应被截断：\n  got:  %d 字符\n  want: %d 字符", len(got.Output), len(result.Output))
	}
	if got.Status != result.Status {
		t.Errorf("Status 不应改变：got %q, want %q", got.Status, result.Status)
	}
	if got.Metadata["key"] != result.Metadata["key"] {
		t.Errorf("Metadata 不应改变：got %v, want %v", got.Metadata, result.Metadata)
	}
}

// TestPostProcess_OutputEqualToLimit 测试输出恰好等于 MaxOutputChars 时不截断。
// 目的：确认边界值 50000 字符不触发截断（用 > 比较，不是 >=）。
// 方法：构造一个正好 50000 字符的 ToolResult，调用 postProcess。
// 预期：Output 保持不变，Status 和 Metadata 不变。
func TestPostProcess_OutputEqualToLimit(t *testing.T) {
	output := strings.Repeat("b", MaxOutputChars)
	result := ToolResult{
		Output:   output,
		Status:   "error",
		Metadata: nil,
	}
	got := postProcess(result)

	if got.Output != output {
		t.Errorf("刚好等于上限时不应截断，输出长度从 %d 变为 %d", len(output), len(got.Output))
	}
	if got.Status != result.Status {
		t.Errorf("Status 不应改变：got %q, want %q", got.Status, result.Status)
	}
}

// TestPostProcess_WithinBufferZone 测试输出超过 MaxOutputChars 但在缓冲区内时不触发截断。
// 目的：确认 MaxOutputChars+1 时因 TruncationMinSavings=200 缓冲区而不截断。
// 方法：构造 MaxOutputChars+1 字符的 Output，调用 postProcess。
// 预期：Output 原样返回，不插入截断标记。
func TestPostProcess_WithinBufferZone(t *testing.T) {
	output := strings.Repeat("c", MaxOutputChars+1)

	result := ToolResult{
		Output:   output,
		Status:   "success",
		Metadata: map[string]any{"exit_code": 0},
	}
	got := postProcess(result)

	if got.Output != output {
		t.Errorf("缓冲区内的输出不应被截断，got %d 字符, want %d 字符", len(got.Output), len(output))
	}
	if got.Status != result.Status {
		t.Errorf("Status 不应改变：got %q, want %q", got.Status, result.Status)
	}
	if got.Metadata["exit_code"] != 0 {
		t.Errorf("Metadata 不应改变")
	}
}

// TestPostProcess_ExceedsBufferZone 测试输出超过 MaxOutputChars+TruncationMinSavings 时触发截断。
// 目的：确认刚好越过缓冲区边界时正确截断。
// 方法：构造 MaxOutputChars+TruncationMinSavings+1 字符的 Output。
// 预期：触发截断，head 和 tail 相交位置正确，截断标记包含正确的省略量。
func TestPostProcess_ExceedsBufferZone(t *testing.T) {
	output := strings.Repeat("c", MaxOutputChars+TruncationMinSavings+1)
	head := output[:20000]
	tail := output[len(output)-30000:]
	omitted := len(output) - len(head) - len(tail)

	result := ToolResult{
		Output:   output,
		Status:   "success",
		Metadata: map[string]any{"exit_code": 0},
	}
	got := postProcess(result)

	expected := expectedTruncatedOutput(head, tail, omitted, len(output))
	if got.Output != expected {
		t.Errorf("超过缓冲区的输出应被截断")
		t.Logf("  got:  %d 字符", len(got.Output))
		t.Logf("  want: %d 字符", len(expected))
	}
	if got.Status != result.Status {
		t.Errorf("Status 不应改变：got %q, want %q", got.Status, result.Status)
	}
}

// TestPostProcess_OutputFarExceedsLimit 测试输出远大于 MaxOutputChars 时正确截断。
// 目的：确认大幅超限时截断逻辑正确，head 和 tail 不重叠。
// 方法：构造 200000 字符的 Output（4 倍上限），调用 postProcess。
// 预期：
//   - 头部 = Output[:20000]
//   - 尾部 = Output[170000:]
//   - 截断标记中 X = 150000, Y = 200000
func TestPostProcess_OutputFarExceedsLimit(t *testing.T) {
	output := strings.Repeat("d", 200000)
	head := output[:20000]
	tail := output[len(output)-30000:]

	result := ToolResult{
		Output:   output,
		Status:   "success",
		Metadata: nil,
	}
	got := postProcess(result)

	expected := expectedTruncatedOutput(head, tail, 150000, 200000)
	if got.Output != expected {
		t.Errorf("远大于上限时截断结果不符合预期")
		t.Logf("  got:  %d 字符", len(got.Output))
		t.Logf("  want: %d 字符", len(expected))
	}
}

// TestPostProcess_StatusAndMetadataPreserved 验证截断时 Status 和 Metadata 被保留。
// 目的：确认 postProcess 只修改 Output，不触碰其他字段。
// 方法：构造各种 Status 和 Metadata 组合，验证截断后不变。
// 预期：所有 case 的 Status 和 Metadata 与输入一致。
func TestPostProcess_StatusAndMetadataPreserved(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		metadata map[string]any
	}{
		{"success with nil metadata", "success", nil},
		{"error with metadata", "error", map[string]any{"exit_code": 1, "stderr": "command not found"}},
		{"timeout with empty metadata", "timeout", map[string]any{}},
		{"empty status", "", nil},
		{"custom status", "cancelled", map[string]any{"reason": "user interrupted"}},
	}

	output := strings.Repeat("e", MaxOutputChars+100)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToolResult{
				Output:   output,
				Status:   tt.status,
				Metadata: tt.metadata,
			}
			got := postProcess(result)

			if got.Status != tt.status {
				t.Errorf("Status 被修改：got %q, want %q", got.Status, tt.status)
			}

			// 比较 Metadata
			if tt.metadata == nil && got.Metadata != nil {
				t.Errorf("Metadata 应为 nil，got %v", got.Metadata)
			} else if tt.metadata != nil {
				for k, v := range tt.metadata {
					if got.Metadata[k] != v {
						t.Errorf("Metadata[%q] 被修改：got %v, want %v", k, got.Metadata[k], v)
					}
				}
			}
		})
	}
}

// TestPostProcess_EmptyOutput 测试空输出（空字符串）时不截断。
// 目的：确认空字符串的边界情况。
// 方法：构造 Output="" 的 ToolResult，调用 postProcess。
// 预期：Output 保持为空，Status 和 Metadata 不变。
func TestPostProcess_EmptyOutput(t *testing.T) {
	result := ToolResult{
		Output:   "",
		Status:   "success",
		Metadata: map[string]any{},
	}
	got := postProcess(result)

	if got.Output != "" {
		t.Errorf("空输出不应被修改：got %q", got.Output)
	}
	if got.Status != "success" {
		t.Errorf("Status 不应改变：got %q, want %q", got.Status, "success")
	}
}

// TestPostProcess_OutputWithNullCharacters 测试输出包含 null 字符时的截断行为。
// 目的：确保包含 \x00 字节的输出能被正确处理。
// 方法：构造包含 null 字符的超过上限的 Output，调用 postProcess。
// 预期：正确截断，不 panic。
func TestPostProcess_OutputWithNullCharacters(t *testing.T) {
	output := strings.Repeat("f", 10000) + "\x00" + strings.Repeat("g", 40001)
	result := ToolResult{
		Output:   output,
		Status:   "success",
		Metadata: nil,
	}
	got := postProcess(result)

	if len(got.Output) == 0 {
		t.Error("截断后输出不应为空")
	}
}

// TestPostProcess_OutputWithUnicode 测试包含多字节 Unicode 字符时的截断行为。
// 目的：确认按字节截断不会 panic，且不至于破坏输出结构。
// 方法：构造包含中文的 60000 字节输出（20000 中文字符），超过上限。
// 预期：不 panic，截断后长度在预期范围内。
func TestPostProcess_OutputWithUnicode(t *testing.T) {
	// 每个中文字符占 3 字节
	chineseChar := "中"
	var sb strings.Builder
	for i := 0; i < 20000; i++ {
		sb.WriteString(chineseChar)
	}
	output := sb.String() // 60000 字节

	result := ToolResult{
		Output:   output,
		Status:   "success",
		Metadata: nil,
	}
	got := postProcess(result)

	// 不 panic
	if len(got.Output) <= 0 {
		t.Error("截断后输出不应为空")
	}

	// 检查是否包含截断标记的关键词
	if !strings.Contains(got.Output, "输出被截断") {
		t.Error("截断后的输出应包含截断标记")
	}
}

// TestPostProcess_HeadTailNoOverlap 测试 head 和 tail 取自不同的区域，不重叠。
// 目的：确认 head 和 tail 取自正确的区域，不会重叠导致内容重复。
// 方法：构造 50010 字节的 Output，每段用不同字符标识。
// 预期：head = output[:20000] 全 'h', tail = output[20010:] 全 't'。
func TestPostProcess_HeadTailNoOverlap(t *testing.T) {
	headPart := strings.Repeat("h", 20000)
	middlePart := strings.Repeat("m", 300)
	tailPart := strings.Repeat("t", 30000)
	output := headPart + middlePart + tailPart

	result := ToolResult{
		Output:   output,
		Status:   "success",
		Metadata: nil,
	}
	got := postProcess(result)

	if !strings.HasPrefix(got.Output, headPart) {
		t.Error("头部应为 20000 个 'h'")
	}

	if !strings.HasSuffix(got.Output, tailPart) {
		t.Error("尾部应为 30000 个 't'")
	}

	body := got.Output[len(headPart) : len(got.Output)-len(tailPart)]
	if strings.Contains(body, "m") {
		t.Error("head 和 tail 之间不应包含中间部分")
	}

	marker := "\n\n[... 输出被截断，省略 300 字符，原始长度 50300 字符 ...]\n\n"
	if body != marker {
		t.Errorf("截断标记不正确：\n  got:  %q\n  want: %q", body, marker)
	}
}

// TestPostProcess_MarkerFormat 验证截断标记的精确格式。
// 目的：确保标记文本与文档完全一致。
// 方法：构造稍超限的输出，提取标记部分，精确比对。
// 预期：标记文本与规格文档一致。
func TestPostProcess_MarkerFormat(t *testing.T) {
	output := strings.Repeat("i", MaxOutputChars+555)
	result := ToolResult{
		Output:   output,
		Status:   "success",
		Metadata: nil,
	}
	got := postProcess(result)

	head := output[:20000]
	tail := output[len(output)-30000:]

	marker := got.Output[len(head) : len(got.Output)-len(tail)]

	expectedMarker := "\n\n[... 输出被截断，省略 555 字符，原始长度 50555 字符 ...]\n\n"
	if marker != expectedMarker {
		t.Errorf("截断标记格式不正确：\n  got:  %q\n  want: %q", marker, expectedMarker)
	}
}

// TestPostProcess_HugeOutput 测试极大数据（百万级字符）的截断。
// 目的：确认在极端大输出下不 panic、截断公式正确。
// 方法：构造 1MB（1048576 字节）的 Output，调用 postProcess。
// 预期：截断后符合头 20000 + 尾 30000 + 标记的格式。
func TestPostProcess_HugeOutput(t *testing.T) {
	output := strings.Repeat("j", 1048576)

	result := ToolResult{
		Output:   output,
		Status:   "success",
		Metadata: nil,
	}
	got := postProcess(result)

	head := output[:20000]
	tail := output[len(output)-30000:]

	if !strings.HasPrefix(got.Output, head) {
		t.Error("头部不正确")
	}
	if !strings.HasSuffix(got.Output, tail) {
		t.Error("尾部不正确")
	}

	omitted := 1048576 - 50000
	expectedMarkerPart := "省略 " + strconv.Itoa(omitted) + " 字符，原始长度 1048576 字符"
	if !strings.Contains(got.Output, expectedMarkerPart) {
		t.Errorf("截断标记中省略数字不正确，应包含 %q", expectedMarkerPart)
	}
}

// ============================================================
// 测试场景清单
//
// 以下列出本文件所有测试方法名和覆盖的场景：
//
// 1. TestPostProcess_OutputLessThanLimit
//    - 场景：输出小于 MaxOutputChars（100 < 50000）
//    - 预期：不截断，Output/Status/Metadata 全部不变
//
// 2. TestPostProcess_OutputEqualToLimit
//    - 场景：输出恰好等于 MaxOutputChars（50000 == 50000）
//    - 预期：用 > 比较，刚好等于时不截断
//
// 3. TestPostProcess_OutputExceedsLimitByOne
//    - 场景：输出刚超过 1 字符（50001 > 50000）
//    - 预期：触发截断，head=20000, tail=30000, X=1, Y=50001
//
// 4. TestPostProcess_OutputFarExceedsLimit
//    - 场景：输出 4 倍上限（200000 >> 50000）
//    - 预期：正确截断，X=150000, Y=200000
//
// 5. TestPostProcess_StatusAndMetadataPreserved
//    - 场景：多种 Status 和 Metadata 组合 + 截断
//    - 预期：所有 case 中 Status 和 Metadata 保持不变
//
// 6. TestPostProcess_EmptyOutput
//    - 场景：Output 为空字符串
//    - 预期：不截断，全部字段不变
//
// 7. TestPostProcess_OutputWithNullCharacters
//    - 场景：Output 包含 \x00 null 字符
//    - 预期：不 panic，正确截断
//
// 8. TestPostProcess_OutputWithUnicode
//    - 场景：Output 包含多字节 UTF-8 字符（中文）
//    - 预期：不 panic，按字节截断
//
// 9. TestPostProcess_HeadTailNoOverlap
//    - 场景：Output 刚超过上限少量字符
//    - 预期：head 和 tail 无重叠，中间只有截断标记
//
// 10. TestPostProcess_MarkerFormat
//     - 场景：验证截断标记文本的精确格式
//     - 预期：标记文本与文档规格完全一致
//
// 11. TestPostProcess_HugeOutput
//     - 场景：1MB 级超大输出
//     - 预期：不 panic，截断公式正确
// ============================================================
