package helm

import (
	"context"
	"strings"

	"argo/src/deck"
	"argo/src/knot"
	"argo/src/reef"
	"argo/src/sail"
)

// RunLoop 执行一次 "用户输入 → 最终回复" 的 ReAct 循环。
// state 由调用方初始化并传入，循环结束后 state.Messages 包含完整历史。
func RunLoop(ctx context.Context, state *TurnState) (<-chan knot.Event, error) {
	out := newEventChannel()

	go func() {
		defer close(out)

		// 会话级缓存：工具列表和 system prompt 只构建一次
		tools := deck.List()
		sp := buildSystemPrompt(tools)

		for {
			state.TurnCount++

			// 1. 裁剪上下文，失败降级使用原始消息
			if msgs, err := reef.Compact(ctx, state.Messages, sail.Window(state.ModelName)); err == nil {
				state.Messages = msgs
			}

			// 2. LLM 调用
			req := knot.ChatRequest{
				SystemPrompt: sp,
				Messages:     state.Messages,
				Tools:        tools,
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
				if se.Type == knot.EventToolUseStart && se.ToolUse != nil {
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

			// 4. 逐条 Lookup + Execute，填充 Result
			for i := range toolUses {
				tu := &toolUses[i]

				tool, found := deck.Lookup(tu.Name)
				if !found {
					tu.Result = &deck.ToolResult{
						Status: deck.StatusError,
						Output: "tool not found: " + tu.Name,
					}
				} else {
					result, err := tool.Execute(ctx, tu.Parameters)
					if err != nil {
						tu.Result = &deck.ToolResult{
							Status: deck.StatusError,
							Output: tu.Name + " tool use failed: " + err.Error(),
						}
					} else {
						tu.Result = &result
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
