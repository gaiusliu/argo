// Package brig 实现工具调用的权限审批门控。
// Engine 在 helm.RunLoop 执行工具前判定 Allow/Ask/Deny。
package brig

import (
	"log/slog"
	"net/url"
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

// Rule 规则即记忆 — 既是求值依据，也是已审批记录
type Rule struct {
	Tool    string  // 工具名（bash / write / edit / web_fetch / read / grep）
	Pattern string  // 匹配模式（命令、目录、域名、文件路径）
	Action  Verdict // Allow / Ask / Deny
}

type ToolCall struct {
	ToolName   string
	RawParam   string
	WorkingDir string
}

// Engine 规则引擎，每个 session 持有一个实例。
// 静态规则只读共享，动态规则由 mu 保护并发追加。
type Engine struct {
	staticRules   []Rule
	userApprovals map[ToolCall]struct{}
	mu            sync.Mutex
}

// NewEngine 创建引擎并加载静态规则副本。
func NewEngine() *Engine {
	return &Engine{
		staticRules:   loadStaticRules(),
		userApprovals: make(map[ToolCall]struct{}),
	}
}

// resolveStaticRules 遍历静态规则列表，首个命中即返回（不再取最高优先级）。
// 正确性依赖 loadStaticRules 中规则组的排列顺序——Deny 组必须在 Allow/Ask 组之前。
// 如需调整规则顺序，参见 deny.go 中 loadStaticRules 的注释。
func resolveStaticRules(staticRules []Rule, tc ToolCall) (Verdict, string) {
	for _, rule := range staticRules {
		if rule.Tool != tc.ToolName {
			continue
		}
		if matchPattern(tc.RawParam, rule.Pattern) {
			if rule.Action == Deny {
				return Deny, formatReason(tc, "denied")
			}

			if rule.Action == Allow {
				return Allow, formatReason(tc, "allowed")
			}

			if rule.Action == Ask {
				return Ask, formatReason(tc, "permission required")
			}
		}

	}
	return Ask, formatReason(tc, "permission required")
}

func (e *Engine) approved(call ToolCall) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.userApprovals[call]
	return ok
}

// scopeKey 将原始参数转为审批粒度键。web_fetch 按域名匹配，其余工具按精确参数匹配。
func scopeKey(toolName, raw string) string {
	if toolName == "web_fetch" && raw != "" {
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			return strings.ToLower(u.Host)
		}
	}
	return raw
}

// formatReason 生成统一格式的门控原因描述
func formatReason(tc ToolCall, suffix string) string {
	return "tool name: " + tc.ToolName + " - " + suffix
}

// Evaluate 对工具调用做门控判定，返回裁决和原因描述。
// 流程：静态规则匹配（Deny > Allow > Ask）→ 已审批检查。
func (e *Engine) Evaluate(tu knot.ToolUse) (Verdict, string) {
	cfg, _ := knot.GetConfig()
	tc := ToolCall{ToolName: tu.Name, RawParam: scopeKey(tu.Name, knot.GetRawParam(tu)), WorkingDir: cfg.WorkingDir}
	verdict, reason := resolveStaticRules(e.staticRules, tc)
	slog.Debug("门控判定", "tool", tu.Name, "result", verdict, "reason", reason)
	switch verdict {
	case Deny:
		return Deny, reason

	case Allow:
		return Allow, reason

	case Ask:
		if e.approved(tc) {
			return Allow, formatReason(tc, "user approved")
		}

		return Ask, formatReason(tc, "permission required")
	}

	// Go 编译器无法证明 switch 已穷尽 Verdict 的 iota 值，保留此兜底 return
	return Ask, formatReason(tc, "permission required")
}

// Approve 追加一条动态规则（用户确认后调用）。
func (e *Engine) Approve(tu knot.ToolUse) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cfg, _ := knot.GetConfig()
	e.userApprovals[ToolCall{ToolName: tu.Name, RawParam: scopeKey(tu.Name, knot.GetRawParam(tu)), WorkingDir: cfg.WorkingDir}] = struct{}{}
}

// Snapshot 返回所有动态规则的快照，供持久化使用。
func (e *Engine) Snapshot() []Rule {
	return nil
}

// Restore 从快照恢复动态规则到当前引擎。
func (e *Engine) Restore(rules []Rule) {

}

// isBoundary 判断字符串是否为空或首字符为合法边界符（空格、路径分隔符）。
// case 6 使用此函数防止前缀误匹配——pattern 匹配到的位置之后必须是边界。
func isBoundary(rest string) bool {
	return rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '/' || rest[0] == '\\'
}

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
	if strings.HasSuffix(pattern, "/*") && len(pattern) > 2 {
		return strings.HasPrefix(target, pattern[:len(pattern)-1])
	}

	// 5. 后缀通配 prefix*：防止对同前缀命令批量 Allow，同时通过 basename 覆盖绝对路径场景
	if strings.HasSuffix(pattern, "*") && len(pattern) > 1 {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(target, prefix) || strings.HasPrefix(filepath.Base(target), prefix)
	}

	// 6. 反向前缀：防止危险命令带参数绕过单语规则。
	// 检查边界——pattern 后必须为空、空格或路径分隔符（/ 或 \），防止：
	//   - "rm" 误匹配 "rmdir"（命令名前缀）
	//   - "/etc/shadow" 误匹配 "/etc/shadow.bak"（文件名前缀）
	// 同时保留 "C:\Windows\System32" 匹配子路径的能力。
	if strings.HasPrefix(target, pattern) {
		rest := target[len(pattern):]
		if isBoundary(rest) {
			return true
		}
	}
	base := filepath.Base(target)
	if strings.HasPrefix(base, pattern) {
		rest := base[len(pattern):]
		if isBoundary(rest) {
			return true
		}
	}
	return false
}
