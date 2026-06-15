package deck

import (
	"fmt"
	"math"
	"sort"
	"strconv"
)

// paramsToArgs 将 map[string]any 转换为 []string 命令行参数（"--key=val"），按 key 字母序排序。
// 支持 bool / int / float64 / string 类型。nil value 或不支持的类型返回 error。
func paramsToArgs(params map[string]any) ([]string, error) {
	// nil 或空 map 返回空切片
	if len(params) == 0 {
		return []string{}, nil
	}

	// 提取并按 key 字母序排序
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	args := make([]string, 0, len(params))
	for _, k := range keys {
		v := params[k]

		// 处理 nil value
		if v == nil {
			return nil, fmt.Errorf("paramsToArgs: nil value for key %q", k)
		}

		var valStr string
		switch val := v.(type) {
		case bool:
			valStr = strconv.FormatBool(val)
		case int:
			valStr = strconv.FormatInt(int64(val), 10)
		case float64:
			// 如果是整数浮点数，用 %v 会显示为 "10"，但 strconv.FormatFloat 总带小数点。
			// 判断是否为整数：val == math.Trunc(val)
			if val == math.Trunc(val) {
				valStr = strconv.FormatInt(int64(val), 10)
			} else {
				valStr = strconv.FormatFloat(val, 'f', -1, 64)
			}
		case string:
			valStr = val
		default:
			return nil, fmt.Errorf("paramsToArgs: unsupported type %T for key %q", v, k)
		}

		args = append(args, fmt.Sprintf("--%s=%s", k, valStr))
	}

	return args, nil
}
