package helm

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"argo/src/brig"
	"argo/src/deck"
	"argo/src/knot"
	"argo/src/reef"
	"argo/src/sail"
	"argo/src/vault"
	"argo/src/voyage"
)

// RunLoop 执行一次 "用户输入 → 最终回复" 的 ReAct 循环。
// state 由调用方初始化并传入，循环结束后 state.Messages 包含完整历史。
// session 为会话级服务聚合（Vault + Engine + ID）。
// onPause 在遇到 brig.Ask 时回调，传入检查点供外部持久化；goroutine 随即退出。
func RunLoop(ctx context.Context, state *TurnState, session *voyage.Session, onPause func(*Checkpoint)) (<-chan knot.Event, error) {
	out := newEventChannel()

	go func() {
		defer close(out)
		slog.Debug("RunLoop 开始")

		// 会话级缓存：工具列表和 system prompt 只构建一次
		tools := deck.List()
		sp := buildSystemPrompt(tools)
		sysMsg := knot.Message{Role: knot.MessageRoleSystem, Content: sp}

		out <- knot.Event{Type: knot.EventStart}

		// 记录本轮开始前的消息数，用于每轮结束后提取新增消息
		msgCount := len(state.Messages)

		for {
			state.TurnCount++

			// 1. 裁剪上下文，失败降级使用原始消息
			if msgs, err := reef.Compact(ctx, state.Messages, sail.Window(state.ModelName)); err == nil {
				state.Messages = msgs
			}

			// 2. LLM 调用，system 消息永远在最前，独立超时
			req := knot.ChatRequest{
				Messages: append([]knot.Message{sysMsg}, state.Messages...),
				Tools:    tools,
			}

			sailEvents, err := sail.Chat(ctx, state.ModelName, req)
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

			// 无工具调用 → 写入 assistant message，退出循环
			if len(toolUses) == 0 {
				state.Messages = append(state.Messages, knot.Message{
					Role:    knot.MessageRoleAssistant,
					Content: textBuf.String(),
				})
				appendRecords(state, session, msgCount, nil)
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
			toolMeta := make(map[string]vault.ToolRecordMeta)
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
					switch verdict, reason := session.Guard.Evaluate(*tu); verdict {
					case brig.Deny:
						tu.Result = knot.ToolResult{Status: knot.StatusError, Output: reason}
						toolMeta[tu.ID] = vault.ToolRecordMeta{Name: tu.Name, Verdict: vault.VerdictAutoDeny, Status: "error"}
					case brig.Ask:
						// 构建检查点交由外部处理，goroutine 退出不阻塞
						onPause(&Checkpoint{
							Messages:     state.Messages,
							TurnCount:    state.TurnCount,
							ModelName:    state.ModelName,
							MsgCount:     msgCount,
							ToolUses:     toolUses,
							ToolUseIndex: i,
							AskReason:    reason,
							ToolMeta:     toolMeta,
						})

						out <- knot.Event{
							Type:      knot.EventAsk,
							ToolUse:   *tu,
							AskReason: reason,
							AskID:     knot.NewAskID(),
						}
						return
					case brig.Allow:
						exec = true
						if session.Guard.IsCachedApproval(*tu) {
							toolMeta[tu.ID] = vault.ToolRecordMeta{Name: tu.Name, Verdict: vault.VerdictCachedAllow}
						} else {
							toolMeta[tu.ID] = vault.ToolRecordMeta{Name: tu.Name, Verdict: vault.VerdictAutoAllow}
						}
					}

					if exec {
						// 工具获批 → 此时才通知调用方展示工具信息
						out <- knot.Event{Type: knot.EventToolUseStart, ToolUse: *tu}
						slog.Debug("工具开始执行", "tool", tu.Name, "params", tu.Parameters)

						result, err := tool.Execute(ctx, tu.Parameters)
						if err != nil {
							slog.Error("工具执行失败", "tool", tu.Name, "error", err)
							tu.Result = knot.ToolResult{
								Status: knot.StatusError,
								Output: tu.Name + " tool use failed: " + err.Error(),
							}
							meta := toolMeta[tu.ID]
							meta.Status = "error"
							toolMeta[tu.ID] = meta
						} else {
							slog.Debug("工具执行成功", "tool", tu.Name, "output", result.Output)
							tu.Result = result
							meta := toolMeta[tu.ID]
							meta.Status = "success"
							toolMeta[tu.ID] = meta
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

			// 6. 本轮新增消息写入 transcript
			appendRecords(state, session, msgCount, toolMeta)
			msgCount = len(state.Messages)
		}
	}()

	return out, nil
}

// appendRecords 将本轮新增消息转换为 Record 并写入 vault。
// toolMeta 仅在包含工具调用时传入，无工具调用时传 nil。
// 写入失败只记日志，不阻塞 RunLoop。
func appendRecords(state *TurnState, session *voyage.Session, since int, toolMeta map[string]vault.ToolRecordMeta) {
	newMsgs := state.Messages[since:]
	if len(newMsgs) == 0 {
		return
	}
	records := vault.MessagesToRecords(newMsgs, toolMeta, state.ModelName)
	if err := session.Append(records); err != nil {
		slog.Error("vault 写入失败", "error", err)
	}
}
