package helm

import (
	"context"
	"log/slog"

	"argo/src/deck"
	"argo/src/knot"
	"argo/src/vault"
	"argo/src/voyage"
)

// Run 驱动状态机从当前 Phase 执行到暂停或结束。
// msgs 指向对话历史，Phase 间共享；reply 为用户对 EventAsk 的回复（新消息时为 AskDefault）。
func Run(ctx context.Context, st *LoopState, session *voyage.Session, msgs *[]knot.Message, reply knot.AskResult) (<-chan knot.Event, error) {
	out := newEventChannel()

	go func() {
		defer close(out)

		emit := func(ev knot.Event) { out <- ev }

		// 根据启动阶段做入口处理
		switch st.Phase {
		case PhaseLLM:
			// 新消息路径：msgs 已包含 user 消息，无需额外处理

		case PhaseWaitAsk:
			// 设置当前用户问题的回答结果
			i := st.ToolUseIndex
			tu := &st.ToolUses[i]
			v := vault.VerdictAskAllow
			if reply == knot.AskAllowed {
				// 1. 用户允许 → 注册审批 + 执行工具
				session.Guard.Approve(*tu)
				emit(knot.Event{Type: knot.EventToolUseStart, ToolUse: *tu})
				slog.Info("执行工具", "tool", tu.Name, "session id", session.ID, "cwd", session.WorkingDir)

				// 1a. 查找工具定义
				tool, found := deck.Lookup(tu.Name)
				if !found {
					tu.Result = knot.ToolResult{Status: knot.StatusError, Output: "tool not found: " + tu.Name}
				} else {
					// 1b. 执行工具并记录结果
					result, err := tool.Execute(ctx, tu.Parameters)
					if err != nil {
						slog.Error("工具执行失败", "tool", tu.Name, "error", err, "cwd", session.WorkingDir, "session id", session.ID)
						tu.Result = knot.ToolResult{Status: knot.StatusError, Output: tu.Name + " failed: " + err.Error()}
					} else {
						tu.Result = result
					}
				}
			} else {
				// 2. 用户拒绝/Default → 记录拒绝结果，跳过执行
				v = vault.VerdictAskDeny
				tu.Result = knot.ToolResult{Status: knot.StatusError, Output: "user rejected: " + st.AskReason}
			}
			emit(knot.Event{Type: knot.EventToolUseDone, ToolUse: *tu})

			// 将 ask 回复结果立刻写入 vault
			appendToolResult(msgs, session, *tu, v, st.ModelName)

			// 3. 推进索引，进入工具执行循环处理剩余工具
			st.ToolUseIndex++
			st.Phase = PhaseExecuteTools
		}

		// 主循环：自动串联 LLM ↔ ExecuteTools，直至挂起或结束
		for {
			switch st.Phase {
			case PhaseLLM:
				runPhaseLLM(ctx, st, session, msgs, emit)

			case PhaseExecuteTools:
				if !runPhaseExecuteTools(ctx, st, session, msgs, emit) {
					// runPhaseExecuteTools 遇 Ask 挂起，退出 goroutine 等外部回复
					return
				}

			case PhaseWaitAsk:
				return

			case PhaseDone:
				emit(knot.Event{Type: knot.EventDone})
				return
			}
		}
	}()

	return out, nil
}
