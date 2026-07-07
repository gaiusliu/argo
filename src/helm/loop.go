package helm

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"argo/src/brig"
	"argo/src/deck"
	"argo/src/knot"
	"argo/src/reef"
	"argo/src/sail"
)

// RunLoop 执行一次 "用户输入 → 最终回复" 的 ReAct 循环。
// state 由调用方初始化并传入，循环结束后 state.Messages 包含完整历史。
// eng 为 session 级规则引擎。Ask 确认通过 EventAsk 事件与调用方交互。
func RunLoop(ctx context.Context, state *TurnState, eng *brig.Engine) (<-chan knot.Event, error) {
	out := newEventChannel()

	go func() {
		defer close(out)
		slog.Debug("RunLoop 开始")

		// 会话级缓存：工具列表和 system prompt 只构建一次
		tools := deck.List()
		sp := buildSystemPrompt(tools)
		sysMsg := knot.Message{Role: knot.MessageRoleSystem, Content: sp}

		out <- knot.Event{Type: knot.EventStart}

		for {
			state.TurnCount++

			// 1. 裁剪上下文，失败降级使用原始消息
			if msgs, err := reef.Compact(ctx, state.Messages, sail.Window(state.ModelName)); err == nil {
				state.Messages = msgs
			}

			// 2. LLM 调用，system 消息永远在最前，独立超时
			llmCtx, llmCancel := context.WithTimeout(ctx, 5*time.Minute)
			req := knot.ChatRequest{
				Messages: append([]knot.Message{sysMsg}, state.Messages...),
				Tools:    tools,
			}

			sailEvents, err := sail.Chat(llmCtx, state.ModelName, req)
			llmCancel()
			if err != nil {
				out <- knot.Event{Type: knot.EventError, Err: err}
				slog.Debug("RunLoop 结束", "reason", "sail.Chat 失败")
				return
			}

			// 3. 流式消费 sail 事件，累计文本 + 收集 tool_use
			var toolUses []knot.ToolUse
			var textBuf strings.Builder

			for se := range sailEvents {
				if se.Type == knot.EventToolUseDelta && se.ToolUse.ID != "" {
					toolUses = append(toolUses, se.ToolUse)
					continue // 延后到工具获批后再发出
				}
				if se.Type == knot.EventTextDelta {
					textBuf.WriteString(se.Delta)
				}
				out <- se
			}

			// 无工具调用 → 注入纯文本 assistant message，退出循环
			if len(toolUses) == 0 {
				state.Messages = append(state.Messages, knot.Message{
					Role:    knot.MessageRoleAssistant,
					Content: textBuf.String(),
				})
				slog.Debug("RunLoop 结束")
				out <- knot.Event{Type: knot.EventDone}
				return
			}

			// 有工具调用 → assistant message 需携带 tool_calls 元数据
			toolCalls := make([]knot.ToolCallDetail, len(toolUses))
			for i, tu := range toolUses {
				argsBytes, _ := json.Marshal(tu.Parameters)
				toolCalls[i] = knot.ToolCallDetail{
					ID:        tu.ID,
					Name:      tu.Name,
					Arguments: string(argsBytes),
				}
			}
			state.Messages = append(state.Messages, knot.Message{
				Role:      knot.MessageRoleAssistant,
				Content:   textBuf.String(),
				ToolCalls: toolCalls,
			})

			// 4. 逐条 Lookup → brig 门控 → Execute，填充 Result
			for i := range toolUses {
				tu := &toolUses[i]

				tool, found := deck.Lookup(tu.Name)
				if !found {
					tu.Result = knot.ToolResult{
						Status: knot.StatusError,
						Output: "tool not found: " + tu.Name,
					}
				} else {
					// brig 门控：判定 Allow / Ask / Deny
					exec := false
					switch verdict, reason := eng.Evaluate(*tu); verdict {
					case brig.Deny:
						tu.Result = knot.ToolResult{Status: knot.StatusError, Output: reason}
					case brig.Ask:
						// 通过 EventAsk 让调用方处理用户确认
						reply := make(chan knot.AskResult, 1)
						out <- knot.Event{Type: knot.EventAsk, ToolUse: *tu, AskReason: reason, AskReply: reply}
						if <-reply == knot.AskDenied {
							tu.Result = knot.ToolResult{Status: knot.StatusError, Output: "user rejected: " + reason}
							break
						}
						eng.Approve(*tu)
						exec = true
					case brig.Allow:
						exec = true
					}

					if exec {
						// 工具获批 → 此时才通知调用方展示工具信息
						out <- knot.Event{Type: knot.EventToolUseDelta, ToolUse: *tu}
						slog.Debug("工具开始执行", "tool", tu.Name, "params", tu.Parameters)
						toolCtx, toolCancel := context.WithTimeout(ctx, 5*time.Minute)
						result, err := tool.Execute(toolCtx, tu.Parameters)
						toolCancel()
						if err != nil {
							slog.Error("工具执行失败", "tool", tu.Name, "error", err)
							tu.Result = knot.ToolResult{
								Status: knot.StatusError,
								Output: tu.Name + " tool use failed: " + err.Error(),
							}
						} else {
							slog.Debug("工具执行成功", "tool", tu.Name, "output", result.Output)
							tu.Result = result
						}
					}
				}

				// 通知调用方执行完成
				out <- knot.Event{Type: knot.EventToolUseDone, ToolUse: *tu}
			}

			// 5. 结果注入 state.Messages，继续循环
			for _, tu := range toolUses {
				state.Messages = append(state.Messages, knot.Message{
					Role:       knot.MessageRoleTool,
					Content:    tu.Result.Output,
					ToolCallID: tu.ID,
				})
			}
		}
	}()

	return out, nil
}
