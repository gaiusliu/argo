package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"argo/src/brig"
	"argo/src/deck"
	"argo/src/helm"
	"argo/src/knot"
	"argo/src/wake"
)

func main() {
	// 1. 加载全局配置（slog 尚未初始化，错误直接输出到 stderr）
	cfg, err := knot.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	// 2. 初始化日志文件 → 配置 slog（日志仅写文件，不占用 stdout/stderr）
	level := parseLevel(cfg.Log.Level)
	programLevel := new(slog.LevelVar)
	programLevel.Set(level)

	w, err := wake.NewLogWriter(cfg.Log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "日志初始化失败: %v\n", err)
		os.Exit(1)
	}
	defer w.Close()

	handler := slog.NewTextHandler(w, &slog.HandlerOptions{
		Level:     programLevel,
		AddSource: true,
	})
	slog.SetDefault(slog.New(handler))

	// 3. 注册所有工具（内置 + CLI）
	if err := deck.RegisterTools(); err != nil {
		slog.Error("注册工具失败", "error", err)
		os.Exit(1)
	}
	slog.Info("启动完成", "tools", len(deck.List()), "workingDir", cfg.WorkingDir)

	// 4. 创建 brig 规则引擎（加载静态规则）
	eng := brig.NewEngine()

	// 5. 构造会话状态（Messages 随每轮对话累积）
	modelName := cfg.Sail.Model
	if modelName == "" {
		modelName = knot.DefaultModel()
	}
	state := &helm.TurnState{
		ModelName: modelName,
	}

	// 6. 统一 stdin 读取器，供 REPL 输入和 EventAsk 共享
	stdin := bufio.NewReader(os.Stdin)

	// 7. REPL 主循环：等待输入 → RunLoop → 消费事件 → 回到等待
	seenTools := make(map[string]bool)

	for {
		fmt.Fprintf(os.Stderr, "argo> ")
		raw, err := stdin.ReadString('\n')
		if err != nil {
			break
		}
		line := strings.TrimRight(raw, " \t\r\n")
		if line == "" {
			continue
		}
		if line == "/exit" {
			break
		}

		// 多行输入：行尾 \ 表示续行，\\ 表示字面反斜杠
		var buf strings.Builder
		for {
			if strings.HasSuffix(line, "\\\\") {
				// 行尾 \\ → 去掉一个 \，作为字面反斜杠结束
				buf.WriteString(line[:len(line)-1])
				break
			}
			if strings.HasSuffix(line, "\\") {
				// 行尾单 \ → 续行：去掉 \ 并换行
				buf.WriteString(line[:len(line)-1])
				buf.WriteByte('\n')
				fmt.Fprintf(os.Stderr, "  → ")
				var err error
				raw, err = stdin.ReadString('\n')
				if err != nil {
					break
				}
				line = strings.TrimRight(raw, " \t\r\n")
				continue
			}
			buf.WriteString(line)
			break
		}

		prompt := buf.String()
		if prompt == "" {
			continue
		}

		// 追加用户消息到会话历史
		state.Messages = append(state.Messages, knot.Message{
			Role: knot.MessageRoleUser, Content: prompt,
		})
		slog.Debug("用户输入", "content", prompt)

		// 启动 RunLoop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		events, err := helm.RunLoop(ctx, state, eng)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ RunLoop 失败: %v\n", err)
			cancel()
			continue
		}

		// 消费事件流
		for ev := range events {
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

			case knot.EventToolUseDelta:
				if ev.ToolUse.ID != "" && !seenTools[ev.ToolUse.ID] {
					seenTools[ev.ToolUse.ID] = true
					// slog.Debug(" --- using tool --- ")
					// slog.Debug(fmt.Sprintf("tool id:%s, tool name:%s, seen tools:%+v", ev.ToolUse.ID, ev.ToolUse.Name, seenTools))
					fmt.Fprintf(os.Stderr, "\n🔧 正在使用工具: %s - %s\n", ev.ToolUse.Name, knot.GetRawParam(ev.ToolUse))
				}

			case knot.EventToolUseDone:
				delete(seenTools, ev.ToolUse.ID)
				if ev.ToolUse.Result.Status == knot.StatusError {
					fmt.Fprintf(os.Stderr, "❌ %s 失败: %s\n", ev.ToolUse.Name, ev.ToolUse.Result.Output)
				} else {
					fmt.Fprintf(os.Stderr, "✅ %s 完成\n", ev.ToolUse.Name)
				}

			case knot.EventAsk:
				result := askCLI(stdin, ev.ToolUse, ev.AskReason)
				ev.AskReply <- result

			case knot.EventError:
				clear(seenTools)
				fmt.Fprintf(os.Stderr, "\n❌ 错误: %v\n", ev.Err)

			case knot.EventDone:
				clear(seenTools)
			}
		}
		cancel()
	}
}

// askCLI 通过 stdin 询问用户是否允许工具执行，返回 AskResult。
func askCLI(stdin *bufio.Reader, tu knot.ToolUse, reason string) knot.AskResult {
	fmt.Fprintf(os.Stderr, "\n⚠️  %s\n", reason)
	fmt.Fprintf(os.Stderr, "   工具: %s, 参数: %s\n", tu.Name, knot.GetRawParam(tu))
	fmt.Fprintf(os.Stderr, "   执行? [y/N]: ")
	answer, _ := stdin.ReadString('\n')
	if strings.EqualFold(strings.TrimSpace(answer), "y") {
		return knot.AskAllowed
	}
	return knot.AskDenied
}

// parseLevel 将配置字符串转为 slog.Level，非法值默认 error
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
