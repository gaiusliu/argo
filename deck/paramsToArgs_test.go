package deck

import (
	"reflect"
	"testing"
)

// TestParamsToArgs_EmptyMap 测试空 map 输入。
// 目的：验证 paramsToArgs(nil) 和 paramsToArgs(map[string]any{}) 均返回 (nil, nil)。
// 方法：分别传入 nil 和空 map，断言返回空切片、nil error。
// 预期结果：两种输入均返回 (nil, nil)。
func TestParamsToArgs_EmptyMap(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
	}{
		{"nil map", nil},
		{"empty map", map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := paramsToArgs(tt.params)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Errorf("want empty slice, got nil")
			}
			if len(got) != 0 {
				t.Errorf("want empty, got %v", got)
			}
		})
	}
}

// TestParamsToArgs_SinglePair 测试单键值对。
// 目的：验证 {"name": "test"} → ["--name=test"]。
func TestParamsToArgs_SinglePair(t *testing.T) {
	got, err := paramsToArgs(map[string]any{"name": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--name=test"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestParamsToArgs_MultiplePairs 测试多键值对按 key 字母序排序。
// 目的：验证输出按 key 字母序排列，不在测试代码中 sort。
func TestParamsToArgs_MultiplePairs(t *testing.T) {
	got, err := paramsToArgs(map[string]any{
		"gamma": "third",
		"alpha": "first",
		"beta":  "second",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--alpha=first", "--beta=second", "--gamma=third"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestParamsToArgs_SortedByKey 验证排序稳定性——多次调用结果一致且首位为最小 key。
func TestParamsToArgs_SortedByKey(t *testing.T) {
	params := map[string]any{
		"zulu": "last", "alpha": "first", "delta": "fourth",
		"bravo": "second", "charlie": "third",
	}
	runs := make([][]string, 3)
	for i := 0; i < 3; i++ {
		var err error
		runs[i], err = paramsToArgs(params)
		if err != nil {
			t.Fatalf("run %d: unexpected error: %v", i, err)
		}
	}
	for i := 1; i < 3; i++ {
		if !reflect.DeepEqual(runs[0], runs[i]) {
			t.Errorf("run 0 and run %d differ: %v vs %v", i, runs[0], runs[i])
		}
	}
	// 验证字母序：首项应为最小 key
	if runs[0][0] != "--alpha=first" {
		t.Errorf("first arg should be --alpha=first, got %s", runs[0][0])
	}
}

// TestParamsToArgs_BoolValue 测试 bool 类型。
func TestParamsToArgs_BoolValue(t *testing.T) {
	tests := []struct {
		name  string
		value bool
		want  string
	}{
		{"true", true, "--enabled=true"},
		{"false", false, "--enabled=false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := paramsToArgs(map[string]any{"enabled": tt.value})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got[0] != tt.want {
				t.Errorf("got %q, want %q", got[0], tt.want)
			}
		})
	}
}

// TestParamsToArgs_NumericValue 测试整数和浮点数。int 和 float64 均接受。
// 注意：LLM 通过 JSON 传入的数字在 Go 反序列化为 map[string]any 时是 float64。
func TestParamsToArgs_NumericValue(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"int", 42, "--count=42"},
		{"int zero", 0, "--count=0"},
		{"int negative", -5, "--count=-5"},
		{"float64 from JSON", float64(10), "--count=10"},
		{"float64 decimal", 3.14, "--count=3.14"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := paramsToArgs(map[string]any{"count": tt.value})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got[0] != tt.want {
				t.Errorf("got %q, want %q", got[0], tt.want)
			}
		})
	}
}

// TestParamsToArgs_StringValue 测试 string 类型。
func TestParamsToArgs_StringValue(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"simple", "hello", "--msg=hello"},
		{"with spaces", "hello world", "--msg=hello world"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := paramsToArgs(map[string]any{"msg": tt.value})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got[0] != tt.want {
				t.Errorf("got %q, want %q", got[0], tt.want)
			}
		})
	}
}

// TestParamsToArgs_MixedTypes 测试混合类型 + 排序。count 用 float64 模拟 LLM 传参。
func TestParamsToArgs_MixedTypes(t *testing.T) {
	got, err := paramsToArgs(map[string]any{
		"verbose": true,
		"name":    "test-app",
		"ratio":   0.95,
		"count":   float64(10), // LLM 传入的实际类型
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--count=10", "--name=test-app", "--ratio=0.95", "--verbose=true"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestParamsToArgs_NilValue 测试 nil value 返回 error。
func TestParamsToArgs_NilValue(t *testing.T) {
	_, err := paramsToArgs(map[string]any{"key": nil})
	if err == nil {
		t.Fatal("nil value should return error")
	}
}

// TestParamsToArgs_UnsupportedType 测试不支持的类型返回 error（不 panic）。
func TestParamsToArgs_UnsupportedType(t *testing.T) {
	_, err := paramsToArgs(map[string]any{"key": []string{"a"}})
	if err == nil {
		t.Fatal("unsupported type should return error")
	}
}

// TestParamsToArgs_UnsupportedSlice 测试 []int 返回 error。
func TestParamsToArgs_UnsupportedSlice(t *testing.T) {
	_, err := paramsToArgs(map[string]any{"ids": []int{1, 2, 3}})
	if err == nil {
		t.Fatal("unsupported type should return error")
	}
}
