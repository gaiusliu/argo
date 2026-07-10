package main

import (
	"bufio"
	"flag"
	"fmt"
	"log/slog"
	"os"
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
	sessionID, err := pickOrCreateSession(client, stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 会话初始化失败: %v\n", err)
		os.Exit(1)
	}

	seenTools := make(map[string]bool)

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
		defer cancel()

		// 消费 SSE 事件流
	eventLoop:
		for {
			ev, ok := <-events
			if !ok {
				break
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
				for _, tu := range ev.AskingToolUses {
					approve := askCLI(stdin, tu)
					if approve {
						tu.VerdictResult = brig.UserApprove
					} else {
						tu.VerdictResult = brig.UserDeny
					}

					tus = append(tus, tu)
				}

				clear(seenTools)

				// Ask 后 server 端 goroutine 已退出、连接已关闭，
				// 此处 cancel 确保客户端旧 body 资源释放，
				// 然后通过 sendAskReply 发起新连接继续消费。
				cancel()
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
		}
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
