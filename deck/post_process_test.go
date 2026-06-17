package deck

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"argo/deck/truncation"
)

// =============================================================================
// Mock Writer — 实现 truncation.Writer 接口，避免写 ~/.argo/truncation/ 真实目录
// =============================================================================

// mockWriter 实现 truncation.Writer 接口，用于测试注入。
// writeFn 为 nil 时返回默认有效路径；设为自定义函数可模拟 Write 成功/失败。
type mockWriter struct {
	dir     string
	writeFn func(content string) (string, error)
}

func (m *mockWriter) Dir() string { return m.dir }

func (m *mockWriter) Write(content string) (string, error) {
	if m.writeFn != nil {
		return m.writeFn(content)
	}
	// 默认正常行为：返回一个固定的有效路径
	return m.dir + "/tool-1234567890-abcdef1234567890.log", nil
}

// newMockWriter 创建一个默认成功的 mock Writer。
func newMockWriter() *mockWriter {
	return &mockWriter{dir: "/fake/truncation"}
}

// setTruncWriter 注入 mock Writer 到全局 truncWriter，并在测试后通过 t.Cleanup 恢复原值。
func setTruncWriter(t *testing.T, w truncation.Writer) {
	t.Helper()
	original := truncWriter
	t.Cleanup(func() { truncWriter = original })
	truncWriter = w
}

// =============================================================================
// 辅助函数
// =============================================================================

// makeStr 生成 n 个 'x' 字符的字符串。
func makeStr(n int) string {
	return strings.Repeat("x", n)
}

// makeNumberedStr 生成可辨识的编号字符串，每行一个编号，用于验证前缀/后缀匹配。
// 生成的字符串长度恰好为 n 字节。
func makeNumberedStr(n int) string {
	var b strings.Builder
	b.Grow(n)
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "%08d\n", i)
	}
	return b.String()[:n]
}

// makeChineseStr 生成 n 个中文字符（每个 3 字节），用于 UTF-8 多字节测试。
func makeChineseStr(n int) string {
	return strings.Repeat("中文", n/6+1)[:n] // "中文" = 6 字节，多取一点再截断
}

// assertMetadataHasKey 断言 metadata 中存在指定 key。
func assertMetadataHasKey(t *testing.T, m map[string]any, key string) {
	t.Helper()
	if _, ok := m[key]; !ok {
		t.Errorf("metadata missing key %q", key)
	}
}

// assertMetadataNoKey 断言 metadata 中不存在指定 key。
func assertMetadataNoKey(t *testing.T, m map[string]any, key string) {
	t.Helper()
	if _, ok := m[key]; ok {
		t.Errorf("metadata should not have key %q", key)
	}
}

// assertMetadataEquals 断言 metadata 中指定 key 的值与期望值相等。
func assertMetadataEquals(t *testing.T, m map[string]any, key string, want any) {
	t.Helper()
	got, ok := m[key]
	if !ok {
		t.Errorf("metadata missing key %q", key)
		return
	}
	if got != want {
		t.Errorf("metadata[%q] = %v, want %v", key, got, want)
	}
}

// =============================================================================
// 组 1：TruncationStrategy.String() 测试
// =============================================================================

// TestTruncationStrategy_String 验证三种策略的 String() 返回值：
// head → "head", tail → "tail", HeadAndTail → "HeadAndTail"
func TestTruncationStrategy_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		strategy TruncationStrategy
		want     string
	}{
		{name: "head", strategy: TruncationHead, want: "head"},
		{name: "tail", strategy: TruncationTail, want: "tail"},
		{name: "HeadAndTail", strategy: TruncationHeadAndTail, want: "HeadAndTail"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.strategy.String()
			if got != tt.want {
				t.Errorf("TruncationStrategy(%d).String() = %q, want %q", int(tt.strategy), got, tt.want)
			}
		})
	}
}

// =============================================================================
// 组 2：TruncationConfig.Validate() 测试
// =============================================================================

// TestTruncationConfig_Validate 验证 TruncationConfig 的合法性校验：
// ratio 合法值 1~80、非法值 0/-10/81；HeadAndTail 策略下 tail_ratio 同样校验。
func TestTruncationConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     TruncationConfig
		wantErr bool
	}{
		// 合法 ratio 边界
		{name: "ratio=20 valid head", cfg: TruncationConfig{Strategy: TruncationHead, Ratio: 20}, wantErr: false},
		{name: "ratio=80 valid head", cfg: TruncationConfig{Strategy: TruncationHead, Ratio: 80}, wantErr: false},
		{name: "ratio=1 valid head", cfg: TruncationConfig{Strategy: TruncationHead, Ratio: 1}, wantErr: false},
		{name: "ratio=20 valid tail", cfg: TruncationConfig{Strategy: TruncationTail, Ratio: 20}, wantErr: false},

		// 非法 ratio：0
		{name: "ratio=0 invalid", cfg: TruncationConfig{Strategy: TruncationHead, Ratio: 0}, wantErr: true},
		// 非法 ratio：负数
		{name: "ratio=-10 invalid", cfg: TruncationConfig{Strategy: TruncationHead, Ratio: -10}, wantErr: true},
		// 非法 ratio：> 80
		{name: "ratio=81 invalid", cfg: TruncationConfig{Strategy: TruncationHead, Ratio: 81}, wantErr: true},

		// HeadAndTail 合法 tail_ratio
		{name: "HeadAndTail ratio=20 tail_ratio=20 valid", cfg: TruncationConfig{Strategy: TruncationHeadAndTail, Ratio: 20, TailRatio: 20}, wantErr: false},
		{name: "HeadAndTail ratio=20 tail_ratio=80 valid", cfg: TruncationConfig{Strategy: TruncationHeadAndTail, Ratio: 20, TailRatio: 80}, wantErr: false},

		// HeadAndTail 非法 tail_ratio
		{name: "HeadAndTail tail_ratio=81 invalid", cfg: TruncationConfig{Strategy: TruncationHeadAndTail, Ratio: 20, TailRatio: 81}, wantErr: true},
		{name: "HeadAndTail tail_ratio=0 invalid", cfg: TruncationConfig{Strategy: TruncationHeadAndTail, Ratio: 20, TailRatio: 0}, wantErr: true},

		// HeadAndTail 且 ratio=0 非法（ratio 检查先于 tail_ratio）
		{name: "HeadAndTail ratio=0 invalid", cfg: TruncationConfig{Strategy: TruncationHeadAndTail, Ratio: 0, TailRatio: 20}, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// =============================================================================
// 组 3：postProcess 不截断场景
// =============================================================================

// TestPostProcess_NoTruncation_BelowThreshold 验证 outputLen=100 < MaxOutputChars=50000 时不截断。
// 方法：构造短字符串输入，预期 Output 不变，metadata.truncated=false。
func TestPostProcess_NoTruncation_BelowThreshold(t *testing.T) {
	// 注入 mock writer
	setTruncWriter(t, newMockWriter())

	input := ToolResult{
		Output:   makeStr(100),
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHead, Ratio: 20}

	got := postProcess(input, cfg)

	// Output 不变
	if got.Output != input.Output {
		t.Errorf("Output changed: len=%d, want len=%d", len(got.Output), len(input.Output))
	}
	// metadata.truncated = false
	assertMetadataEquals(t, got.Metadata, "truncated", false)
	// 无截断专用字段
	assertMetadataNoKey(t, got.Metadata, "original_length")
	assertMetadataNoKey(t, got.Metadata, "kept_length")
	assertMetadataNoKey(t, got.Metadata, "truncation_strategy")
	assertMetadataNoKey(t, got.Metadata, "full_output_path")
}

// TestPostProcess_NoTruncation_AtThreshold 验证 outputLen=50000 == MaxOutputChars 时不截断（> 比较，不含等号）。
// 方法：构造恰好等于上限的字符串，预期不截断。
func TestPostProcess_NoTruncation_AtThreshold(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	input := ToolResult{
		Output:   makeStr(MaxOutputChars),
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHead, Ratio: 20}

	got := postProcess(input, cfg)

	// Output 不变（等号不触发截断）
	if got.Output != input.Output {
		t.Errorf("Output should not be truncated when len==MaxOutputChars, got len=%d", len(got.Output))
	}
	// metadata.truncated = false
	assertMetadataEquals(t, got.Metadata, "truncated", false)
}

// =============================================================================
// 组 4：postProcess 三种策略截断
// =============================================================================

// TestPostProcess_HeadTruncation 验证 Head 策略：60000 字符 > MaxOutputChars，ratio=20，
// 保留前 12000 字符（向下取整到 rune 边界）。
func TestPostProcess_HeadTruncation(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	original := makeNumberedStr(60000)
	input := ToolResult{
		Output:   original,
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHead, Ratio: 20}

	got := postProcess(input, cfg)

	// 截断后 Output 长度应小于原始长度
	if len(got.Output) >= 60000 {
		t.Error("Output should be truncated")
	}
	// 保留前缀：前 12000 字节（rune 对齐）应与原始前缀匹配
	expectedKeep := 60000 * 20 / 100 // 12000
	// 取前缀部分（不含标记），找到截断标记位置
	markerIdx := strings.Index(got.Output, "\n[...truncated:")
	if markerIdx < 0 {
		t.Fatal("truncation marker not found")
	}
	kept := got.Output[:markerIdx]
	// 保留长度应接近 expectedKeep（rune 对齐可能略短）
	if len(kept) < expectedKeep-10 || len(kept) > expectedKeep+10 {
		t.Errorf("head kept length = %d, want ~%d (rune-aligned)", len(kept), expectedKeep)
	}
	// 前缀匹配
	if !strings.HasPrefix(original, kept) {
		t.Error("kept content should match original prefix")
	}
}

// TestPostProcess_TailTruncation 验证 Tail 策略：60000 字符 > MaxOutputChars，ratio=20，
// 保留后 12000 字符（向下取整到 rune 边界）。
func TestPostProcess_TailTruncation(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	original := makeNumberedStr(60000)
	input := ToolResult{
		Output:   original,
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationTail, Ratio: 20}

	got := postProcess(input, cfg)

	if len(got.Output) >= 60000 {
		t.Error("Output should be truncated")
	}
	// 保留后缀：后 12000 字节应与原始后缀匹配
	expectedKeep := 60000 * 20 / 100 // 12000
	// 找到截断标记之后的内容
	markerEnd := strings.Index(got.Output, "...]\n")
	if markerEnd < 0 {
		t.Fatal("truncation marker end not found")
	}
	kept := got.Output[markerEnd+5:] // 跳过 "...]\n"
	if len(kept) < expectedKeep-10 || len(kept) > expectedKeep+10 {
		t.Errorf("tail kept length = %d, want ~%d (rune-aligned)", len(kept), expectedKeep)
	}
	// 后缀匹配
	if !strings.HasSuffix(original, kept) {
		t.Error("kept content should match original suffix")
	}
}

// TestPostProcess_TailTruncation_LargeOutput 验证 Tail 策略：200000 字符，ratio=20，
// 保留后 40000 字符。
func TestPostProcess_TailTruncation_LargeOutput(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	original := makeNumberedStr(200000)
	input := ToolResult{
		Output:   original,
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationTail, Ratio: 20}

	got := postProcess(input, cfg)

	if len(got.Output) >= 200000 {
		t.Error("Output should be truncated")
	}
	// 保留后缀
	markerEnd := strings.Index(got.Output, "...]\n")
	if markerEnd < 0 {
		t.Fatal("truncation marker end not found")
	}
	kept := got.Output[markerEnd+5:]
	if !strings.HasSuffix(original, kept) {
		t.Error("kept content should match original suffix")
	}
	// 验证截断标记包含正确的原始长度
	if !strings.Contains(got.Output, "original 200000") {
		t.Error("marker should contain original length 200000")
	}
}

// TestPostProcess_HeadAndTailTruncation 验证 HeadAndTail 策略：60000 字符，
// ratio=20 tail_ratio=20，保留头 12000 + 尾 12000 共约 24000 字符。
func TestPostProcess_HeadAndTailTruncation(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	original := makeNumberedStr(60000)
	input := ToolResult{
		Output:   original,
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHeadAndTail, Ratio: 20, TailRatio: 20}

	got := postProcess(input, cfg)

	if len(got.Output) >= 60000 {
		t.Error("Output should be truncated")
	}
	// 找到标记：Output = <head>\n[...marker...]\n<tail>
	markerStart := strings.Index(got.Output, "\n[...truncated:")
	if markerStart < 0 {
		t.Fatal("truncation marker not found")
	}
	markerEnd := strings.Index(got.Output[markerStart:], "...]\n")
	if markerEnd < 0 {
		t.Fatal("truncation marker end not found")
	}
	markerEnd += markerStart // 转为绝对位置

	headPart := got.Output[:markerStart]
	tailPart := got.Output[markerEnd+5:] // 跳过 "...]\n"

	// head 部分匹配原始前缀
	if !strings.HasPrefix(original, headPart) {
		t.Error("head part should match original prefix")
	}
	// tail 部分匹配原始后缀
	if !strings.HasSuffix(original, tailPart) {
		t.Error("tail part should match original suffix")
	}
}

// TestPostProcess_HeadAndTailTruncation_LargeOutput 验证 HeadAndTail 策略：200000 字符大输出。
func TestPostProcess_HeadAndTailTruncation_LargeOutput(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	original := makeNumberedStr(200000)
	input := ToolResult{
		Output:   original,
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHeadAndTail, Ratio: 20, TailRatio: 20}

	got := postProcess(input, cfg)

	if len(got.Output) >= 200000 {
		t.Error("Output should be truncated")
	}
	// 验证截断标记包含正确的原始长度
	if !strings.Contains(got.Output, "original 200000") {
		t.Error("marker should contain original length 200000")
	}
	// 验证包含 HeadAndTail 策略名
	if !strings.Contains(got.Output, "HeadAndTail") {
		t.Error("marker should contain HeadAndTail strategy name")
	}
}

// TestPostProcess_HeadAndTail_NoOverlap 验证 HeadAndTail 策略头尾不重叠：
// 100000 字符，ratio=20 tail_ratio=20，head+tail < original，kept_length < original_length。
func TestPostProcess_HeadAndTail_NoOverlap(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	original := makeNumberedStr(100000)
	input := ToolResult{
		Output:   original,
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHeadAndTail, Ratio: 20, TailRatio: 20}

	got := postProcess(input, cfg)

	// 总的保留长度（含标记）应小于原始长度
	if len(got.Output) >= 100000 {
		t.Error("kept_length (with marker) should be less than original_length")
	}
	// metadata.kept_length 不含标记，应 < original_length
	keptLen, _ := got.Metadata["kept_length"].(int)
	origLen, _ := got.Metadata["original_length"].(int)
	if keptLen >= origLen {
		t.Errorf("kept_length=%d should be < original_length=%d", keptLen, origLen)
	}
}

// =============================================================================
// 组 5：postProcess metadata 字段
// =============================================================================

// TestPostProcess_Metadata_Truncated 验证截断时 metadata 包含：
// truncated=true, original_length, kept_length, truncation_strategy, full_output_path。
func TestPostProcess_Metadata_Truncated(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	input := ToolResult{
		Output:   makeNumberedStr(60000),
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHead, Ratio: 20}

	got := postProcess(input, cfg)

	// 五个关键字段均存在且值合理
	assertMetadataEquals(t, got.Metadata, "truncated", true)
	assertMetadataEquals(t, got.Metadata, "original_length", 60000)
	assertMetadataEquals(t, got.Metadata, "truncation_strategy", "head")

	// kept_length > 0 且 < original_length
	keptLen, ok := got.Metadata["kept_length"].(int)
	if !ok || keptLen <= 0 || keptLen >= 60000 {
		t.Errorf("kept_length should be >0 and <60000, got %v", got.Metadata["kept_length"])
	}

	// full_output_path 非空
	fullPath, ok := got.Metadata["full_output_path"].(string)
	if !ok || fullPath == "" {
		t.Errorf("full_output_path should be non-empty, got %q", fullPath)
	}
}

// TestPostProcess_Metadata_NotTruncated 验证不截断时 metadata.truncated=false，
// 原有字段透传，无截断专用字段。
func TestPostProcess_Metadata_NotTruncated(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	input := ToolResult{
		Output:   makeStr(100),
		Status:   "error",
		Metadata: map[string]any{"custom_key": "custom_value"},
	}
	cfg := TruncationConfig{Strategy: TruncationHead, Ratio: 20}

	got := postProcess(input, cfg)

	// truncated=false
	assertMetadataEquals(t, got.Metadata, "truncated", false)
	// 原有字段透传
	assertMetadataEquals(t, got.Metadata, "custom_key", "custom_value")
	// 无截断专用字段
	assertMetadataNoKey(t, got.Metadata, "original_length")
	assertMetadataNoKey(t, got.Metadata, "kept_length")
	assertMetadataNoKey(t, got.Metadata, "truncation_strategy")
	assertMetadataNoKey(t, got.Metadata, "full_output_path")
}

// TestPostProcess_Metadata_Preserved 验证截断时原有 metadata 保留，并追加截断字段。
func TestPostProcess_Metadata_Preserved(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	input := ToolResult{
		Output: makeNumberedStr(60000),
		Status: "success",
		Metadata: map[string]any{
			"custom_field": "preserved",
			"exit_code":    0,
		},
	}
	cfg := TruncationConfig{Strategy: TruncationTail, Ratio: 20}

	got := postProcess(input, cfg)

	// 原有字段保留
	assertMetadataEquals(t, got.Metadata, "custom_field", "preserved")
	assertMetadataEquals(t, got.Metadata, "exit_code", 0)

	// 截断字段追加
	assertMetadataEquals(t, got.Metadata, "truncated", true)
	assertMetadataEquals(t, got.Metadata, "original_length", 60000)
	assertMetadataHasKey(t, got.Metadata, "kept_length")
	assertMetadataHasKey(t, got.Metadata, "full_output_path")
}

// =============================================================================
// 组 6：postProcess 边界与鲁棒性
// =============================================================================

// TestPostProcess_EmptyOutput 验证空字符串输入：不截断、不 panic。
func TestPostProcess_EmptyOutput(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	input := ToolResult{
		Output:   "",
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHead, Ratio: 20}

	got := postProcess(input, cfg)

	// 不 panic 即为通过；再验证 Output 仍为空
	if got.Output != "" {
		t.Errorf("empty output should remain empty, got %q", got.Output)
	}
	// 不截断
	assertMetadataEquals(t, got.Metadata, "truncated", false)
}

// TestPostProcess_UTF8MultiByte 验证中文等多字节 UTF-8 场景：不 panic、输出合法 UTF-8。
func TestPostProcess_UTF8MultiByte(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	// 构造超过阈值的 UTF-8 中文内容（每个中文字符 3 字节）
	original := makeChineseStr(60000)
	input := ToolResult{
		Output:   original,
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHead, Ratio: 20}

	got := postProcess(input, cfg)

	// 不 panic 即为通过
	// 验证输出是合法 UTF-8（可以遍历 rune）
	for range got.Output {
		// 遍历 rune 不会 panic 说明 UTF-8 合法
	}
	// 截断后应短于原始
	if len(got.Output) >= len(original) {
		t.Error("Output should be truncated")
	}
}

// TestPostProcess_OneMBOutput 验证 1MB 输出不 panic，Tail 策略下保留后 10%（约 10%）。
func TestPostProcess_OneMBOutput(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	// 1MB = 1048576 字节
	const oneMB = 1048576
	original := makeNumberedStr(oneMB)
	input := ToolResult{
		Output:   original,
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationTail, Ratio: 10}

	got := postProcess(input, cfg)

	// 不 panic 即为通过
	// 验证截断发生
	if len(got.Output) >= oneMB {
		t.Error("Output should be truncated")
	}
	// 验证截断标记中原始长度为 1048576
	if !strings.Contains(got.Output, "original 1048576") {
		t.Errorf("marker should contain original length 1048576, got: %s", got.Output[len(got.Output)-200:])
	}
}

// =============================================================================
// 组 7：postProcess 自定义 Ratio
// =============================================================================

// TestPostProcess_Head_CustomRatio 验证 Head 策略 ratio=30，保留前 30%。
func TestPostProcess_Head_CustomRatio(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	original := makeNumberedStr(60000)
	input := ToolResult{
		Output:   original,
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHead, Ratio: 30}

	got := postProcess(input, cfg)

	if len(got.Output) >= 60000 {
		t.Error("Output should be truncated")
	}
	// 保留前缀约 30% = 18000 字节
	markerIdx := strings.Index(got.Output, "\n[...truncated:")
	if markerIdx < 0 {
		t.Fatal("truncation marker not found")
	}
	kept := got.Output[:markerIdx]
	expected := 60000 * 30 / 100 // 18000
	if len(kept) < expected-20 || len(kept) > expected+20 {
		t.Errorf("kept length = %d, want ~%d (rune-aligned)", len(kept), expected)
	}
}

// TestPostProcess_Tail_CustomRatio 验证 Tail 策略 ratio=10，保留后 10%。
func TestPostProcess_Tail_CustomRatio(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	original := makeNumberedStr(60000)
	input := ToolResult{
		Output:   original,
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationTail, Ratio: 10}

	got := postProcess(input, cfg)

	if len(got.Output) >= 60000 {
		t.Error("Output should be truncated")
	}
	// 保留后缀约 10%
	markerEnd := strings.Index(got.Output, "...]\n")
	if markerEnd < 0 {
		t.Fatal("truncation marker end not found")
	}
	kept := got.Output[markerEnd+5:]
	expected := 60000 * 10 / 100 // 6000
	if len(kept) < expected-10 || len(kept) > expected+10 {
		t.Errorf("kept length = %d, want ~%d (rune-aligned)", len(kept), expected)
	}
}

// TestPostProcess_HeadAndTail_CustomRatio 验证 HeadAndTail 策略 ratio=15 + tail_ratio=25，
// 保留头 15% + 尾 25%。
func TestPostProcess_HeadAndTail_CustomRatio(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	original := makeNumberedStr(60000)
	input := ToolResult{
		Output:   original,
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHeadAndTail, Ratio: 15, TailRatio: 25}

	got := postProcess(input, cfg)

	if len(got.Output) >= 60000 {
		t.Error("Output should be truncated")
	}
	// 验证截断标记包含 HeadAndTail
	if !strings.Contains(got.Output, "HeadAndTail") {
		t.Error("marker should contain HeadAndTail strategy name")
	}

	// 验证头尾部分
	markerStart := strings.Index(got.Output, "\n[...truncated:")
	if markerStart < 0 {
		t.Fatal("truncation marker not found")
	}
	markerEnd := strings.Index(got.Output[markerStart:], "...]\n")
	if markerEnd < 0 {
		t.Fatal("truncation marker end not found")
	}
	markerEnd += markerStart

	headPart := got.Output[:markerStart]
	tailPart := got.Output[markerEnd+5:]

	headExpected := 60000 * 15 / 100 // 9000
	if len(headPart) < headExpected-10 || len(headPart) > headExpected+10 {
		t.Errorf("head kept length = %d, want ~%d", len(headPart), headExpected)
	}

	tailExpected := 60000 * 25 / 100 // 15000
	if len(tailPart) < tailExpected-15 || len(tailPart) > tailExpected+15 {
		t.Errorf("tail kept length = %d, want ~%d", len(tailPart), tailExpected)
	}
}

// =============================================================================
// 组 8：postProcess 截断标记（DEC-011）
// =============================================================================

// TestPostProcess_Marker_Head 验证 Head 策略截断标记格式：
// \n[...truncated: head, original <N> chars; full output: <path>...]\n
func TestPostProcess_Marker_Head(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	input := ToolResult{
		Output:   makeNumberedStr(60000),
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHead, Ratio: 20}

	got := postProcess(input, cfg)

	// 验证标记包含完整格式
	if !strings.Contains(got.Output, "[...truncated: head, original 60000 chars; full output:") {
		t.Errorf("marker should contain [...truncated: head, original 60000 chars; full output:, got: %s", got.Output[len(got.Output)-200:])
	}
	if !strings.HasSuffix(got.Output, "...]\n") {
		t.Error("marker should end with ...]\\n")
	}
}

// TestPostProcess_Marker_Tail 验证 Tail 策略 marker 包含 "tail"。
func TestPostProcess_Marker_Tail(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	input := ToolResult{
		Output:   makeNumberedStr(60000),
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationTail, Ratio: 20}

	got := postProcess(input, cfg)

	if !strings.Contains(got.Output, "[...truncated: tail, original 60000 chars; full output:") {
		t.Errorf("marker should contain 'tail' strategy, got: %s", got.Output[:200])
	}
}

// TestPostProcess_Marker_HeadAndTail 验证 HeadAndTail 策略 marker 包含 "HeadAndTail"。
func TestPostProcess_Marker_HeadAndTail(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	input := ToolResult{
		Output:   makeNumberedStr(60000),
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHeadAndTail, Ratio: 20, TailRatio: 20}

	got := postProcess(input, cfg)

	if !strings.Contains(got.Output, "[...truncated: HeadAndTail, original 60000 chars; full output:") {
		t.Errorf("marker should contain 'HeadAndTail' strategy, got: %s", got.Output[:200])
	}
}

// TestPostProcess_Marker_NotTriggered 验证未截断时 Output 不含截断标记。
func TestPostProcess_Marker_NotTriggered(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	input := ToolResult{
		Output:   makeStr(100),
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHead, Ratio: 20}

	got := postProcess(input, cfg)

	if strings.Contains(got.Output, "[...truncated:") {
		t.Error("output should not contain truncation marker when not truncated")
	}
}

// TestPostProcess_Marker_TruncationPath 验证 marker 中的路径与 metadata.full_output_path 一致。
func TestPostProcess_Marker_TruncationPath(t *testing.T) {
	setTruncWriter(t, newMockWriter())

	input := ToolResult{
		Output:   makeNumberedStr(60000),
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHead, Ratio: 20}

	got := postProcess(input, cfg)

	fullPath, ok := got.Metadata["full_output_path"].(string)
	if !ok || fullPath == "" {
		t.Fatal("full_output_path should be non-empty")
	}

	// marker 中应包含 "; full output: <path>...]"
	expectedSeg := "; full output: " + fullPath + "...]"
	if !strings.Contains(got.Output, expectedSeg) {
		t.Errorf("marker should contain path %q, got marker segment: %s", fullPath, got.Output[len(got.Output)-200:])
	}
}

// TestPostProcess_Marker_TruncationPath_Degraded 验证 truncation.Writer.Write 失败时
// marker 不含 "; full output:" 段，且 metadata.full_output_path 为空字符串（DEC-012 降级策略）。
func TestPostProcess_Marker_TruncationPath_Degraded(t *testing.T) {
	// 注入会失败的 mock Writer
	degradedWriter := &mockWriter{
		dir: "/fake/truncation",
		writeFn: func(content string) (string, error) {
			return "", errors.New("disk full")
		},
	}
	setTruncWriter(t, degradedWriter)

	input := ToolResult{
		Output:   makeNumberedStr(60000),
		Status:   "success",
		Metadata: map[string]any{},
	}
	cfg := TruncationConfig{Strategy: TruncationHead, Ratio: 20}

	got := postProcess(input, cfg)

	// marker 不应包含 "; full output:" 段
	if strings.Contains(got.Output, "; full output:") {
		t.Error("marker should NOT contain '; full output:' when Write fails (degraded mode)")
	}

	// 但 marker 仍应包含策略和原始长度（降级不中断截断）
	if !strings.Contains(got.Output, "[...truncated: head, original 60000 chars") {
		t.Error("marker should still contain strategy and original length in degraded mode")
	}

	// metadata.full_output_path 应为空字符串
	fullPath, ok := got.Metadata["full_output_path"].(string)
	if ok && fullPath != "" {
		t.Errorf("full_output_path should be empty in degraded mode, got %q", fullPath)
	}
}
