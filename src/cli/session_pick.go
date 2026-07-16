package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// pickOrCreateSession 展示最近会话列表，让用户选择或新建。
// 返回 sessionID、是否为已有会话（而非新建）。
func pickOrCreateSession(c *argoClient, stdin *bufio.Reader) (string, bool, error) {
	sessions, err := c.listSessions()
	if err != nil {
		return "", false, err
	}

	if len(sessions) > 0 {
		show := sessions
		if len(show) > 5 {
			show = show[:5]
		}
		fmt.Fprintf(os.Stderr, "── 最近会话 ──\n")
		for i, s := range show {
			name := s.Name
			if name == "" {
				name = s.ID
			}
			fmt.Fprintf(os.Stderr, "  [%d] %s  (%s)\n",
				i+1, name, s.LastActiveAt.Format("01-02 15:04"))
		}
		fmt.Fprintf(os.Stderr, "──────────────\n")

		fmt.Fprintf(os.Stderr, "输入序号恢复 / n 新建: ")
		input, _ := stdin.ReadString('\n')
		input = strings.TrimSpace(input)

		if input != "" && strings.ToLower(input) != "n" {
			if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(show) {
				return show[idx-1].ID, true, nil // 已有会话
			}
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", false, err
	}

	id, err := c.createSession(cwd)
	if err != nil {
		return "", false, err
	}
	fmt.Fprintf(os.Stderr, "会话已创建: %s\n", id)
	return id, false, nil // 新会话
}
