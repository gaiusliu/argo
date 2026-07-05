// Package brig 实现工具调用的权限审批门控。
// Engine 在 helm.RunLoop 执行工具前判定 Allow/Ask/Deny。
package brig

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"argo/src/knot"
)

// Verdict 工具调用的三态裁决结果
type Verdict int

const (
	Allow Verdict = iota // 免审批，直接执行
	Ask                  // 需用户确认
	Deny                 // 直接拒绝
)

// RuleSource 规则来源
type RuleSource int

const (
	Static  RuleSource = iota // 代码内置，NewEngine 时加载，不可变
	Dynamic                   // 用户确认后追加，可持久化
)

// Rule 规则即记忆 — 既是求值依据，也是已审批记录
type Rule struct {
	Tool    string     // 工具名（bash / write / edit / web_fetch / read / grep）
	Pattern string     // 匹配模式（命令、目录、域名、文件路径）
	Action  Verdict    // Allow / Ask / Deny
	Source  RuleSource // Static / Dynamic
}

// Engine 规则引擎，每个 session 持有一个实例。
// 静态规则只读共享，动态规则由 mu 保护并发追加。
type Engine struct {
	rules []Rule
	mu    sync.Mutex
}

// NewEngine 创建引擎并加载静态规则副本。
func NewEngine() *Engine {
	return &Engine{
		rules: loadStaticRules(),
	}
}

// Evaluate 对工具调用做门控判定，返回裁决、原因描述和触发的 pattern。
// 流程：原始参数 → deny 检查 → Ask/Allow 决议。deny 用 raw 零损失匹配，
// Ask/Allow 也用 raw 直接匹配（matchPattern 六条 case 覆盖所有方向）。
func (e *Engine) Evaluate(tu knot.ToolUse) (Verdict, string, string) {
	raw := getRawParam(tu)

	// deny 检查：原始参数直接匹配 deny 规则，不经过任何提炼
	if verdict, matched := e.checkDeny(tu.Name, raw); verdict == Deny {
		return Deny, fmt.Sprintf("%s %q 被策略拒绝", tu.Name, matched), matched
	}

	// raw 为空表示未知工具或无需门控的工具，默认 Ask
	if raw == "" {
		return Ask, fmt.Sprintf("%s 需要您确认", tu.Name), ""
	}

	// Ask/Allow 决议：单条 raw 遍历规则，最后命中者胜
	verdict, matched := resolveVerdict(e.rules, tu.Name, raw)

	switch verdict {
	case Ask:
		if matched != "" {
			return Ask, fmt.Sprintf("%s %q 需要您确认", tu.Name, matched), matched
		}
		return Ask, fmt.Sprintf("%s 首次使用，需要您确认", tu.Name), raw
	default:
		return Allow, "", matched
	}
}

// Append 追加一条动态规则（用户确认后调用）。
func (e *Engine) Append(tool, pattern string, action Verdict, source RuleSource) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, Rule{
		Tool:    tool,
		Pattern: pattern,
		Action:  action,
		Source:  source,
	})
}

// Snapshot 返回所有动态规则的快照，供持久化使用。
func (e *Engine) Snapshot() []Rule {
	e.mu.Lock()
	defer e.mu.Unlock()

	var dyn []Rule
	for _, r := range e.rules {
		if r.Source == Dynamic {
			dyn = append(dyn, r)
		}
	}
	return dyn
}

// Restore 从快照恢复动态规则到当前引擎。
func (e *Engine) Restore(rules []Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range rules {
		r.Source = Dynamic
		e.rules = append(e.rules, r)
	}
}

// getRawParam 从工具参数中取原始输入字符串——无截断、无拆分、无信息损失。
// 对齐 sail adapter 的格式：Parameters["args"] 是完整 JSON 字符串，先解析再取值。
func getRawParam(tu knot.ToolUse) string {
	// adapter.go 发出 {"args": "{\"cmd\":\"ls\"}"} 格式，先从 "args" 键解析
	if raw, ok := tu.Parameters[knot.ParamArgs].(string); ok && raw != "" {
		var parsed map[string]any
		if json.Unmarshal([]byte(raw), &parsed) == nil {
			return fieldFromArgs(tu.Name, parsed)
		}
		// JSON 解析失败（不应发生），退回直接使用 raw
		return raw
	}
	return fieldFromArgs(tu.Name, tu.Parameters)
}

// fieldFromArgs 从已解析的参数 map 中按工具名取对应的字段值
func fieldFromArgs(name string, params map[string]any) string {
	switch name {
	case "bash":
		cmd, _ := params[knot.ParamCmd].(string)
		return cmd
	case "write", "edit", "read", "grep":
		path, _ := params[knot.ParamPath].(string)
		return path
	case "web_fetch":
		url, _ := params[knot.ParamURL].(string)
		return url
	default:
		return ""
	}
}

// checkDeny 只遍历 Action==Deny 的规则，用原始参数（raw）做匹配。
// 与 resolveVerdict 不同——Deny 走 raw 零损失，Ask/Allow 走提炼后的 pattern。
func (e *Engine) checkDeny(tool, raw string) (Verdict, string) {
	if raw == "" {
		return Allow, ""
	}
	for _, r := range e.rules {
		if r.Action != Deny || r.Tool != tool {
			continue
		}
		if matchPattern(raw, r.Pattern) {
			return Deny, r.Pattern
		}
	}
	return Allow, ""
}

// GeneralizePattern 将用户确认时的原始参数概括为可复用的 pattern。
// 匹配时用 raw 零损失，保存时概括才有意义——同目录、同命令前缀、同域名自动通过。
func GeneralizePattern(tool, raw string) string {
	if raw == "" {
		return ""
	}
	switch tool {
	case "bash":
		// "rm -rf /tmp/test" → 取前 2 词 → "rm -rf*"
		words := strings.Fields(raw)
		n := 2
		if len(words) < n {
			n = len(words)
		}
		return strings.Join(words[:n], " ") + "*"
	case "write", "edit":
		// "/src/internal/config.go" → "/src/internal/*"
		dir := filepath.Dir(raw)
		if dir == "." {
			return "*"
		}
		return dir + "/*"
	case "web_fetch":
		// "https://api.github.com/repos/..." → "api.github.com*"
		domain := extractDomain(raw)
		if domain == "" {
			return raw
		}
		return domain + "*"
	default:
		return raw
	}
}

// resolveVerdict 遍历规则列表，用单条 raw 匹配，取最高优先级，无命中则Ask
func resolveVerdict(rules []Rule, tool, raw string) (Verdict, string) {
	verd := Ask
	pat := ""
	for i := range rules {
		r := &rules[i]
		if r.Tool != tool {
			continue
		}
		if matchPattern(raw, r.Pattern) {
			if verd < r.Action {
				verd = r.Action
				pat = r.Pattern
			}
		}
	}

	return verd, pat
}

// matchPattern 判断目标字符串是否匹配规则模式，支持六种匹配方式。
//
// 匹配按优先级依次尝试，命中即返回（不继续后续 case）：
//
//  1. 精确匹配 — target == pattern
//     防止：规则中写死固定字符串的场景
//     示例：deny "rm -rf /" 拦截精确命令，deny "127.0.0.1" 拦截精确域名
//
//  2. 中间通配 *word* — target 任意位置包含 word
//     防止：敏感文件名隐藏在深层路径中
//     示例：sensitiveFiles "*credentials*" 匹配 "/home/.aws/credentials"、"app/config/credentials.json"
//
//  3. 前导通配 *.ext — target 以 .ext 结尾
//     防止：读取/搜索特定后缀的凭证/密钥文件
//     示例：sensitiveFiles "*.pem" 匹配 "/app/key.pem"、"~/.ssh/id_rsa.pem"
//
//  4. 目录通配 dir/* — target 在目录 dir/ 下
//     防止：用户确认后对同目录下所有子路径自动 Allow
//     示例：动态规则 "src/*" 匹配 "src/internal"、"src/brig/file.go"
//     保留尾部 "/" 避免 "src" 误匹配 "srcnotdir"
//
//  5. 后缀通配 prefix* — target 以 prefix 开头，或 target 的文件名以 prefix 开头
//     防止：对同前缀命令/文件批量生效
//     示例：safeCommands "go version*" 匹配 "go version -m"
//     sensitiveFiles ".env*" 匹配 "/app/.env.local"（通过 basename 匹配）
//
//  6. 反向前缀 — target 以 pattern（纯文本，无通配符）开头
//     防止：危险命令带任意参数时绕过单语规则
//     示例：dangerCommands "rm" 匹配 "rm -rf /"、"rm file.txt"
//     safeCommands  "ls" 匹配 "ls -la"
func matchPattern(target, pattern string) bool {
	// 1. 精确匹配
	if target == pattern {
		return true
	}

	// 2. 中间通配 *word*：防止敏感文件名出现在路径任意位置
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") && len(pattern) > 1 {
		word := pattern[1 : len(pattern)-1]
		return word != "" && strings.Contains(target, word)
	}

	// 3. 前导通配 *.ext：防止读取特定后缀的凭证/密钥文件
	if strings.HasPrefix(pattern, "*") && len(pattern) > 1 {
		return strings.HasSuffix(target, pattern[1:])
	}

	// 4. 目录通配 dir/*：防止对同目录下文件批量 Allow，保留 "/" 避免前缀误匹配
	if strings.HasSuffix(pattern, "/*") && len(pattern) > 1 {
		return strings.HasPrefix(target, pattern[:len(pattern)-1])
	}

	// 5. 后缀通配 prefix*：防止对同前缀命令批量 Allow，同时通过 basename 覆盖绝对路径场景
	if strings.HasSuffix(pattern, "*") && len(pattern) > 1 {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(target, prefix) || strings.HasPrefix(filepath.Base(target), prefix)
	}

	// 6. 反向前缀：防止危险命令带参数绕过单语规则（如 "rm" 匹配 "rm -rf"）
	return strings.HasPrefix(target, pattern) || strings.HasPrefix(filepath.Base(target), pattern)
}
