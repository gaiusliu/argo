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

	diffmatchpatch "github.com/sergi/go-diff/diffmatchpatch"
)

// ANSI：输入区域低饱和度深灰底，与 LLM 输出区域视觉区分
const (
	inputBg = "\033[48;5;60m" // 256 色：灰蓝 (#5f5f87)，与黑底有明显区分
	resetBg = "\033[49m"
	clrEOL  = "\033[K" // 清除光标到行尾（以当前背景色填充）
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
		// 蓝底全宽行（输入区域顶部边界）→ 回行首 → 提示符
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
				// 续行提示符（蓝底全宽）
				fmt.Fprintf(os.Stderr, "%s%s\r%s  → ", inputBg, clrEOL, inputBg)
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
			fmt.Fprintf(os.Stderr, "%s", resetBg)
			continue
		}

		// 关闭背景色（输入区域结束）
		fmt.Fprintf(os.Stderr, "%s\n", resetBg)

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
// 对 write/edit 工具额外展示彩色 diff 变更预览。
func askCLI(stdin *bufio.Reader, tu knot.ToolUse, reason string) knot.AskResult {
	// write/edit 工具：展示编辑前后对比
	switch tu.Name {
	case "write", "edit":
		showDiff(tu)
	}

	fmt.Fprintf(os.Stderr, "\n⚠️  %s\n", reason)
	fmt.Fprintf(os.Stderr, "   工具: %s, 参数: %s\n", tu.Name, knot.GetRawParam(tu))
	fmt.Fprintf(os.Stderr, "   执行? [y/N]: ")
	answer, _ := stdin.ReadString('\n')
	if strings.EqualFold(strings.TrimSpace(answer), "y") {
		return knot.AskAllowed
	}
	return knot.AskDenied
}

// ANSI 颜色码，用于 diff 输出
const (
	red   = "\033[31m"
	green = "\033[32m"
	reset = "\033[0m"
)

// showDiff 读取目标文件当前内容，与工具参数中的新内容做 diff，彩色输出到 stderr。
func showDiff(tu knot.ToolUse) {
	// 获取文件路径
	path, _ := tu.Parameters[knot.ParamPath].(string)
	if path == "" {
		return
	}

	// 读取当前文件内容：不存在视为空，其他错误放弃预览并告警
	var oldText string
	if data, err := os.ReadFile(path); err == nil {
		oldText = string(data)
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "\n⚠️  无法读取原文件 %s: %v\n", path, err)
		return
	}

	// 根据工具类型确定新内容
	var newText string
	switch tu.Name {
	case "write":
		newText, _ = tu.Parameters[knot.ParamContent].(string)
	case "edit":
		oldStr, _ := tu.Parameters[knot.ParamOldStr].(string)
		newStr, _ := tu.Parameters[knot.ParamNewStr].(string)
		if replaceAll, _ := tu.Parameters[knot.ParamReplaceAll].(bool); replaceAll {
			newText = strings.ReplaceAll(oldText, oldStr, newStr)
		} else {
			newText = strings.Replace(oldText, oldStr, newStr, 1)
		}
	}

	// 计算行级 diff
	dmp := diffmatchpatch.New()
	a, b, lines := dmp.DiffLinesToChars(oldText, newText)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, lines)

	// 输出 diff，删除行红色、新增行绿色
	fmt.Fprintf(os.Stderr, "\n── 📄 %s ──", path)
	for _, d := range diffs {
		for _, line := range strings.Split(strings.TrimSuffix(d.Text, "\n"), "\n") {
			switch d.Type {
			case diffmatchpatch.DiffInsert:
				fmt.Fprintf(os.Stderr, "\n%s+ %s%s", green, line, reset)
			case diffmatchpatch.DiffDelete:
				fmt.Fprintf(os.Stderr, "\n%s- %s%s", red, line, reset)
			case diffmatchpatch.DiffEqual:
				fmt.Fprintf(os.Stderr, "\n  %s", line)
			}
		}
	}
	fmt.Fprintf(os.Stderr, "\n──%s──\n", strings.Repeat("─", 40))
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
