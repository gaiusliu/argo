// Package brig 实现工具调用的权限审批门控。
// Warden 在 helm.Run 执行工具前做分层裁决。
package brig

import (
	"argo/src/knot"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
)

const (
	Default     int = iota
	Approve         // 免审批，直接执行
	Ask             // 需用户确认
	Deny            // 直接拒绝
	UserApprove     // 用户审批通过
	UserDeny        // 用户审批拒绝
)

// RuleType 规则类型——零值无意义，必须显式指定。
const (
	RuleIron = iota + 1 // 红线，铁律不可绕过
	RuleCord            // 普通可配置规则，绳令可按需调整
)

// Gate 工具调用门禁接口（替代 Guard）。
type Gate interface {
	Evaluate(tu knot.ToolUse) (int, string)
	Approve(tu knot.ToolUse)
	Snapshot() []ApprovalEntry // 导出用户审批记录
	Restore([]ApprovalEntry)   // 从快照恢复
}

// RuleLoader 规则来源接口。Load 在构造 Warden 时调用一次，Evaluate 不再调用。
type RuleLoader interface {
	Load() []Rule
}

// Rule 规则即记忆 — 既是求值依据，也是已审批记录
type Rule struct {
	Tool    string // 工具名（bash / write / edit / web_fetch / read / grep）
	Pattern string // 匹配模式（命令、目录、域名、文件路径）
	Action  int    // Approve / Ask / Deny
	Type    int    // RuleIron / RuleCord（零值无意义，必须显式指定）
}

// ApprovalEntry 用户审批记录的持久化条目。
type ApprovalEntry struct {
	Tool    string `json:"tool"`
	Pattern string `json:"pattern"`
}

type ToolCall struct {
	ToolName   string
	RawParam   string
	WorkingDir string
}

// Warden 分层裁决引擎（替代 FileGuard）。构造时从 RuleLoader 一次性加载规则，
// 按 Type 分为 iron（红线）和 cord（普通）两桶，Evaluate 时只读遍历。
type Warden struct {
	iron          []Rule
	cord          []Rule
	userApprovals map[ToolCall]struct{}
	workingDir    string
	mu            sync.RWMutex
}

// NewWarden 创建 Warden，从 loaders 加载规则并按 Type 分桶。
func NewWarden(workingDir string, loaders ...RuleLoader) *Warden {
	w := &Warden{
		userApprovals: make(map[ToolCall]struct{}),
		workingDir:    workingDir,
	}
	// 一次性加载所有规则，按类型分桶
	for _, ld := range loaders {
		for _, r := range ld.Load() {
			switch r.Type {
			case RuleIron:
				w.iron = append(w.iron, r)
			case RuleCord:
				w.cord = append(w.cord, r)
			}
		}
	}
	return w
}

// formatReason 生成统一格式的门控原因描述
func formatReason(tc ToolCall, suffix string) string {
	return "tool name: " + tc.ToolName + " - " + suffix
}

// scopeKey 将原始参数转为审批粒度键。web_fetch 按域名匹配，其余工具按精确参数匹配。
func scopeKey(toolName, raw string) string {
	if toolName == "web_fetch" && raw != "" {
		if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
			return strings.ToLower(u.Hostname())
		}
	}
	return raw
}

// evaluate 对一组规则做匹配，首个命中即返回。
func evaluate(rules []Rule, tc ToolCall) (int, string) {
	for _, r := range rules {
		if r.Tool != tc.ToolName {
			continue
		}
		if matchPattern(tc.RawParam, r.Pattern) {
			if r.Action == Deny {
				return Deny, formatReason(tc, "denied")
			}
			if r.Action == Approve {
				return Approve, formatReason(tc, "allowed")
			}
			if r.Action == Ask {
				return Ask, formatReason(tc, "permission required")
			}
		}
	}
	return Ask, formatReason(tc, "permission required")
}

// Evaluate 分层裁决：Iron（红线）→ Cord（可配置）→ 用户审批 → Ask。
func (w *Warden) Evaluate(tu knot.ToolUse) (int, string) {
	tc := ToolCall{ToolName: tu.Name, RawParam: scopeKey(tu.Name, knot.GetRawParam(tu)), WorkingDir: w.workingDir}

	// 1. Iron 红线优先
	if verdict, reason := evaluate(w.iron, tc); verdict != Ask {
		slog.Info("门控判定", "tool", tu.Name, "verdict", verdict, "layer", "iron", "reason", reason)
		return verdict, reason
	}

	// 2. Cord 可配置规则
	if verdict, reason := evaluate(w.cord, tc); verdict != Ask {
		// 规则命中 Approve → 检查是否已有用户审批记录
		if verdict == Approve {
			slog.Info("门控判定", "tool", tu.Name, "verdict", verdict, "layer", "cord", "reason", reason)
			return Approve, reason
		}
		slog.Info("门控判定", "tool", tu.Name, "verdict", verdict, "layer", "cord", "reason", reason)
		return verdict, reason
	}

	// 3. 动态审批记录
	if w.approved(tc) {
		return Approve, formatReason(tc, "user approved")
	}

	// 4. 兜底 Ask
	return Ask, formatReason(tc, "permission required")
}

func (w *Warden) approved(call ToolCall) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	_, ok := w.userApprovals[call]
	return ok
}

// Approve 追加一条动态规则（用户确认后调用）。
func (w *Warden) Approve(tu knot.ToolUse) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.userApprovals[ToolCall{ToolName: tu.Name, RawParam: scopeKey(tu.Name, knot.GetRawParam(tu)), WorkingDir: w.workingDir}] = struct{}{}
}

func (w *Warden) Snapshot() []ApprovalEntry {
	w.mu.RLock()
	defer w.mu.RUnlock()
	result := make([]ApprovalEntry, 0, len(w.userApprovals))
	for tc := range w.userApprovals {
		result = append(result, ApprovalEntry{Tool: tc.ToolName, Pattern: tc.RawParam})
	}
	return result
}

// Restore 从快照恢复用户审批记录。
func (w *Warden) Restore(entries []ApprovalEntry) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, entry := range entries {
		w.userApprovals[ToolCall{ToolName: entry.Tool, RawParam: entry.Pattern, WorkingDir: w.workingDir}] = struct{}{}
	}
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
