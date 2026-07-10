package vault

import (
	"argo/src/knot"
	"encoding/json"
)

// ToolRecordMeta 工具执行的附加元数据（RunLoop 收集，Message 中不包含）。
type ToolRecordMeta struct {
	Name    string           // 工具名
	Verdict knot.ToolVerdict // 审批结果 5 态
	Status  string           // "success" / "error"
}

// MessagesToRecords 将本轮新增消息转为 Record 列表。
// toolMeta 的 key 为 tool_call_id，由 RunLoop 在执行阶段收集。
func MessagesToRecords(msgs []knot.Message, toolMeta map[string]ToolRecordMeta, modelName string) []Record {
	records := make([]Record, 0, len(msgs))
	for _, msg := range msgs {
		switch msg.Role {
		case knot.MessageRoleUser:
			records = append(records, Record{Type: RecordUser, Content: msg.Content})
		case knot.MessageRoleAssistant:
			records = append(records, llmRecord(msg, modelName))
		case knot.MessageRoleTool:
			records = append(records, toolRecord(msg, knot.ToolUse{}))
		}
	}
	return records
}

func MessagesToRecordsNew(msgs []knot.Message, toolUsesByIDs map[string]knot.ToolUse, modelName string) []Record {
	records := make([]Record, 0, len(msgs))
	for _, msg := range msgs {
		switch msg.Role {
		case knot.MessageRoleUser:
			records = append(records, Record{Type: RecordUser, Content: msg.Content})
		case knot.MessageRoleAssistant:
			records = append(records, llmRecord(msg, modelName))
		case knot.MessageRoleTool:
			records = append(records, toolRecord(msg, toolUsesByIDs[msg.ToolCallID]))
		}
	}
	return records
}

func llmRecord(msg knot.Message, modelName string) Record {
	r := Record{Type: RecordLLM, Content: msg.Content, Model: modelName}
	for _, tc := range msg.ToolCalls {
		r.ToolCalls = append(r.ToolCalls, ToolCallRecord{
			ToolCallID: tc.ID,
			Name:       tc.Name,
			Args:       parseArgs(tc.Arguments),
		})
	}
	return r
}

func toolRecord(msg knot.Message, toolUse knot.ToolUse) Record {
	status := ""
	if toolUse.Result.Status == knot.StatusSuccess {
		status = "success"
	} else if toolUse.Result.Status == knot.StatusError {
		status = "error"
	}
	return Record{Type: RecordTool, Content: msg.Content, ToolCallID: msg.ToolCallID,
		Name: toolUse.Name, Verdict: toolUse.VerdictResult, Status: status}
}

// parseArgs 将 JSON 参数字符串反序列化为 map[string]any。
func parseArgs(raw string) map[string]any {
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil
	}
	return args
}
