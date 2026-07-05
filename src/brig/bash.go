package brig

import "strings"

// splitTokens 定义 bash 命令的拆分分隔符
var splitTokens = []string{"&&", "||", ";", "|"}

// extractSubCommands 将复合 shell 命令拆分为独立子命令，
// 对每个子命令提取前 2 个单词作为规则匹配的 pattern。
// 如 "git push && ls -la" → ["git push", "ls"]。
func extractSubCommands(cmd string) []string {
	// 用分隔符拆成独立子命令段
	segments := []string{cmd}
	for _, sep := range splitTokens {
		var expanded []string
		for _, seg := range segments {
			parts := strings.Split(seg, sep)
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					expanded = append(expanded, p)
				}
			}
		}
		segments = expanded
	}

	// 对每个子命令取前 2 个单词作为 base command
	var patterns []string
	for _, seg := range segments {
		words := strings.Fields(seg)
		if len(words) == 0 {
			continue
		}
		// 取前 2 词，使 "git push origin" → "git push"
		n := 2
		if len(words) < n {
			n = len(words)
		}
		base := strings.Join(words[:n], " ")
		patterns = append(patterns, base)
	}

	return patterns
}
