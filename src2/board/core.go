package board

import (
	"fmt"
	"strings"

	"argo/src2/pact"
)

// match 在 rules 中匹配 omen，返回匹配的动作和理由。无匹配返回 0, ""。
func match(rules []Rule, omen pact.Omen) (int, string) {
	for _, r := range rules {
		if r.Hand == omen.Name && matchPattern(omen.Arguments, r.Pattern) {
			return r.Action, "matched " + r.Pattern
		}
	}
	return 0, ""
}

// hasAye 检查 omen 是否已有船长许可。
func (b *Board) hasAye(omen pact.Omen) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.aye[scopeKey(omen)]
	return ok
}

func scopeKey(omen pact.Omen) string { return omen.Name + ":" + omen.Arguments }

// splitScopeKey 将 scopeKey 格式 "Name:Arguments" 拆分为工具名和参数。
func splitScopeKey(key string) (name, args string, err error) {
	var ok bool
	name, args, ok = strings.Cut(key, ":")
	if !ok {
		return "", "", fmt.Errorf("invalid scope key: %q", key)
	}
	return name, args, nil
}

// matchPattern 规则模式匹配。
// TODO：当前仅支持精确匹配和 * 通配，后续需实现完整模式匹配
// （精确/中间/前缀/后缀/目录/边界），参考 src/board/board.go:197-244。
func matchPattern(target, pattern string) bool {
	return target == pattern || pattern == "*"
}
