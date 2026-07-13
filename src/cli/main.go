package main

import (
	"bufio"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"argo/src/brig"
	"argo/src/knot"
	"argo/src/wake"
)

// ANSI：输入区域低饱和度深灰底，与 LLM 输出区域视觉区分
const (
	inputBg = "\033[48;5;60m"
	resetBg = "\033[49m"
	clrEOL  = "\033[K"
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
	stdin := bufio.NewReader(os.Stdin)

	// 列出最近会话，让用户选择或创建
	sessionID, isExisting, err := pickOrCreateSession(client, stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 会话初始化失败: %v\n", err)
		os.Exit(1)
	}

	// 恢复已有会话时展示对话历史
	if isExisting {
		resp, err := client.resumeSession(sessionID)
		if err == nil && len(resp.Messages) > 0 {
			showMessages(resp.Messages)
		}
	}

	seenTools := make(map[string]bool)

	// Ctrl+C 不再杀进程，改为中断信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	// REPL 主循环
	for {
		fmt.Fprintf(os.Stderr, "\n%s%s\r%sargo> ", inputBg, clrEOL, inputBg)
		raw, err := stdin.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", resetBg)
			break
		}
		line := strings.TrimRight(raw, " \t\r\n")
		if line == "" {
			fmt.Fprintf(os.Stderr, "%s", resetBg)
			continue
		}
		if line == "/exit" {
			fmt.Fprintf(os.Stderr, "%s\n", resetBg)
			break
		}

		// 多行输入：行尾 \ 表示续行
		prompt := readMultiLine(stdin, line)
		if prompt == "" {
			fmt.Fprintf(os.Stderr, "%s", resetBg)
			continue
		}
		fmt.Fprintf(os.Stderr, "%s\n", resetBg)

		// 通过 HTTP SSE 发送消息
		events, cancel, err := client.sendPrompt(sessionID, prompt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ 发送失败: %v\n", err)
			continue
		}

		// SSE 事件消费（同时监听 Ctrl+C 中断）
	eventLoop:
		for {
			select {
			case ev, ok := <-events:
				if !ok {
					cancel()
					break eventLoop
				}
				switch ev.Type {
				case knot.EventTextDelta:
					fmt.Print(ev.Delta)
				case knot.EventTextDone:
					fmt.Println()
				case knot.EventThinkingStart:
					fmt.Println("\n── 💭思考中 ──")
				case knot.EventThinkingDelta:
					fmt.Print(ev.Delta)
				case knot.EventThinkingDone:
					fmt.Println("\n── 💭思考完成 ──")
				case knot.EventToolUseStart:
					if ev.ToolUse.ID != "" && !seenTools[ev.ToolUse.ID] {
						seenTools[ev.ToolUse.ID] = true
						showToolStart(ev)
					}
				case knot.EventToolUseDone:
					delete(seenTools, ev.ToolUse.ID)
					showToolDone(ev)
				case knot.EventAsk:
					var tus []knot.ToolUse
					interrupted := false
					for _, tu := range ev.AskingToolUses {
						verdict := askCLI(stdin, tu)
						if verdict == verdictInterrupt {
							interrupted = true
							break
						}
						if verdict == verdictApprove {
							tu.VerdictResult = brig.UserApprove
						} else {
							tu.VerdictResult = brig.UserDeny
						}
						tus = append(tus, tu)
					}

					clear(seenTools)
					cancel()

					if interrupted {
						client.interruptSession(sessionID)
						fmt.Fprintf(os.Stderr, "\n⏹ 会话已终止\n")
						break eventLoop
					}

					events, cancel, err = client.sendAskReply(sessionID, tus)
					if err != nil {
						fmt.Fprintf(os.Stderr, "❌ 回复失败: %v\n", err)
						break eventLoop
					}

				case knot.EventError:
					clear(seenTools)
					fmt.Fprintf(os.Stderr, "\n❌ 错误: %v\n", ev.Err)
				case knot.EventDone:
					clear(seenTools)
				}

			case <-sigCh:
				cancel()
				fmt.Fprintf(os.Stderr, "\n⏹ 已中断\n")
				break eventLoop
			}
		}
		cancel()
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
