package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// pickOrCreateSession 展示最近会话列表，让用户选择或新建。
func pickOrCreateSession(c *argoClient, stdin *bufio.Reader) (string, error) {
	sessions, err := c.listSessions()
	if err != nil {
		return "", err
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

		// 用户输入序号选择已有会话，或 n 新建
		fmt.Fprintf(os.Stderr, "输入序号恢复 / n 新建: ")
		input, _ := stdin.ReadString('\n')
		input = strings.TrimSpace(input)

		if input != "" && strings.ToLower(input) != "n" {
			if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(show) {
				id := show[idx-1].ID
				fmt.Fprintf(os.Stderr, "已恢复会话: %s\n", id)
				return id, nil
			}
		}
	}

	// 获取当前工作目录
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	id, err := c.createSession(cwd)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(os.Stderr, "会话已创建: %s\n", id)
	return id, nil
}
