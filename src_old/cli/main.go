package main

import (
	"bufio"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"argo/src_old/knot"
	"argo/src_old/wake"

	tea "charm.land/bubbletea/v2"
)

func main() {
	addr := flag.String("addr", "localhost:4090", "argo-server 地址")
	flag.Parse()

	// 初始化日志文件
	cfg, err := knot.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	level := parseLevel(cfg.Log.Level)
	programLevel := new(slog.LevelVar)
	programLevel.Set(level)
	w, err := wake.NewLogWriter(cfg.Log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "日志初始化失败: %v\n", err)
		os.Exit(1)
	}
	defer w.Close()
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: programLevel, AddSource: true,
	})))
	slog.Info("CLI 启动", "addr", *addr)

	// 启动或连接 argo-server
	cleanup, err := startServer(*addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ server 启动失败: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	client := newArgoClient(*addr)

	// 会话选择（在 TUI 之前，线性流程）
	sessionID, isExisting, err := pickOrCreateSession(client, bufio.NewReader(os.Stdin))
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 会话初始化失败: %v\n", err)
		os.Exit(1)
	}

	// 恢复已有会话时加载历史消息，传入 TUI 统一渲染
	var historyMsgs []knot.Message
	if isExisting {
		resp, err := client.resumeSession(sessionID)
		if err == nil {
			historyMsgs = resp.Messages
		}
	}

	// 启动 Bubble Tea TUI（历史消息由 initialModel 预填充到 viewport）
	p := tea.NewProgram(initialModel(client, sessionID, historyMsgs))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}

// parseLevel 将配置字符串转为 slog.Level。
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}
