package board

import (
	"fmt"
	"path/filepath"
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

func isBoundary(rest string) bool {
	return rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '/' || rest[0] == '\\'
}

// matchPattern 规则模式匹配（精确/中间/前导/后缀/目录/边界）。
func matchPattern(target, pattern string) bool {
	// 1. 精确匹配
	if target == pattern {
		return true
	}
	// 2. 中间通配 *word*
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") && len(pattern) > 1 {
		word := pattern[1 : len(pattern)-1]
		return word != "" && strings.Contains(target, word)
	}
	// 3. 前导通配 *.ext
	if strings.HasPrefix(pattern, "*") && len(pattern) > 1 {
		return strings.HasSuffix(target, pattern[1:])
	}
	// 4. 目录通配 dir/*
	if strings.HasSuffix(pattern, "/*") && len(pattern) > 2 {
		return strings.HasPrefix(target, pattern[:len(pattern)-1])
	}
	// 5. 后缀通配 prefix*
	if strings.HasSuffix(pattern, "*") && len(pattern) > 1 {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(target, prefix) ||
			strings.HasPrefix(filepath.Base(target), prefix)
	}
	// 6. 边界检查
	if strings.HasPrefix(target, pattern) {
		if isBoundary(target[len(pattern):]) {
			return true
		}
	}
	base := filepath.Base(target)
	if strings.HasPrefix(base, pattern) && isBoundary(base[len(pattern):]) {
		return true
	}
	return false
}
