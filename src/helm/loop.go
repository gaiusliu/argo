package helm

import (
	"context"
	"strings"

	"argo/src/brig"
	"argo/src/deck"
	"argo/src/knot"
	"argo/src/reef"
	"argo/src/sail"
)

// RunLoop 执行一次 "用户输入 → 最终回复" 的 ReAct 循环。
// state 由调用方初始化并传入，循环结束后 state.Messages 包含完整历史。
// confirm 为工具调用 Ask 时的用户确认回调，eng 为 session 级规则引擎。
func RunLoop(ctx context.Context, state *TurnState, confirm knot.ConfirmFunc, eng *brig.Engine) (<-chan knot.Event, error) {
	out := newEventChannel()

	go func() {
		defer close(out)

		// 会话级缓存：工具列表和 system prompt 只构建一次
		tools := deck.List()
		sp := buildSystemPrompt(tools)
		sysMsg := knot.Message{Role: knot.MessageRoleSystem, Content: sp}

		for {
			state.TurnCount++

			// 1. 裁剪上下文，失败降级使用原始消息
			if msgs, err := reef.Compact(ctx, state.Messages, sail.Window(state.ModelName)); err == nil {
				state.Messages = msgs
			}

			// 2. LLM 调用，system 消息永远在最前
			req := knot.ChatRequest{
				Messages: append([]knot.Message{sysMsg}, state.Messages...),
				Tools:    tools,
			}

			sailEvents, err := sail.Chat(ctx, state.ModelName, req)
			if err != nil {
				out <- knot.Event{Type: knot.EventError, Err: err}
				return
			}

			// 3. 流式消费 sail 事件，累计文本 + 收集 tool_use
			var toolUses []knot.ToolUse
			var textBuf strings.Builder

			for se := range sailEvents {
				if se.Type == knot.EventToolUseDelta && se.ToolUse != nil {
					toolUses = append(toolUses, *se.ToolUse)
				}
				if se.Type == knot.EventTextDelta {
					textBuf.WriteString(se.Delta)
				}
				out <- se
			}

			// assistant message 注入（LLM 本轮的文本回复）
			state.Messages = append(state.Messages, knot.Message{
				Role:    knot.MessageRoleAssistant,
				Content: textBuf.String(),
			})

			// 无工具调用 → 退出循环
			if len(toolUses) == 0 {
				return
			}

			// 4. 逐条 Lookup → brig 门控 → Execute，填充 Result
			for i := range toolUses {
				tu := &toolUses[i]

				tool, found := deck.Lookup(tu.Name)
				if !found {
					tu.Result = &knot.ToolResult{
						Status: knot.StatusError,
						Output: "tool not found: " + tu.Name,
					}
				} else {
					// brig 门控：判定 Allow / Ask / Deny
					exec := false
					switch verdict, reason, pat := eng.Evaluate(*tu); verdict {
					case brig.Deny:
						tu.Result = &knot.ToolResult{Status: knot.StatusError, Output: reason}
					case brig.Ask:
						ok, err := confirm(*tu, reason)
						if err != nil {
							tu.Result = &knot.ToolResult{Status: knot.StatusError, Output: "confirm failed: " + err.Error()}
							break
						}
						if !ok {
							tu.Result = &knot.ToolResult{Status: knot.StatusError, Output: "user rejected: " + reason}
							break
						}
						// 用户确认后概括 raw 为通用 pattern，追加动态 Allow 规则
						eng.Append(tu.Name, brig.GeneralizePattern(tu.Name, pat), brig.Allow, brig.Dynamic)
						exec = true
					case brig.Allow:
						exec = true
					}

					if exec {
						result, err := tool.Execute(ctx, tu.Parameters)
						if err != nil {
							tu.Result = &knot.ToolResult{
								Status: knot.StatusError,
								Output: tu.Name + " tool use failed: " + err.Error(),
							}
						} else {
							tu.Result = &result
						}
					}
				}

				// 通知调用方执行完成
				out <- knot.Event{Type: knot.EventToolUseDone, ToolUse: tu}
			}

			// 5. 结果注入 state.Messages，继续循环
			for _, tu := range toolUses {
				state.Messages = append(state.Messages, knot.Message{
					Role:    knot.MessageRoleTool,
					Content: tu.Result.Output,
				})
			}
		}
	}()

	return out, nil
}
