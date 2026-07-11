# vault 对话记录（transcript）

> vault 负责对话记录的持久化存储。helm 每个 Phase 结束后通过 checkpoint 增量写入本轮产生的所有 record。

## 文件位置

```
~/.argo/sessions/{session_id}/transcript.jsonl
```

## 格式

JSONL，每一行一条 record。每条 record 含 `type` 字段，当前共三种类型。所有 record 均有 `timestamp`（ISO 8601，FileVault.Append 自动填充）：

| type | 记录内容 |
|------|------|
| `user` | 用户输入 |
| `model` | 模型返回原文、模型名、工具调用请求 |
| `tool` | 工具执行结果、审批状态、成功/失败状态 |

## 字段设计

### 公共字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | `"user"` / `"model"` / `"tool"` |
| `seq` | int | session 级自增整数（当前由 json.Encoder 自行管理，未显式分配） |
| `timestamp` | string | ISO 8601（RFC 3339），FileVault.Append 自动填充 `time.Now().UTC()` |

### user

```jsonl
{"type":"user","content":"帮我重构 main.go","timestamp":"2026-06-09T15:30:00Z"}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `content` | string | 用户输入原文 |

### model

```jsonl
{"type":"model","model":"claude-sonnet","content":null,"tool_calls":[{"tool_call_id":"tc1","name":"read","args":{"filePath":"main.go"}}],"timestamp":"2026-06-09T15:30:03Z"}
{"type":"model","model":"claude-sonnet","content":"重构完成","timestamp":"2026-06-09T15:30:09Z"}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `model` | string | 使用的模型名称 |
| `content` | string or null | 模型文本输出；仅有工具调用时为 null |
| `tool_calls` | array | `[{tool_call_id, name, args}]`，无工具调用时 omitempty 省略 |

### tool

```jsonl
{"type":"tool","verdict":1,"tool_call_id":"tc1","name":"read","content":"package main\n\nimport \"fmt\"\n...","status":"success","timestamp":"2026-06-09T15:30:04Z"}
{"type":"tool","verdict":5,"tool_call_id":"tc2","name":"bash","content":"tool bash failed: permission denied","status":"error","timestamp":"2026-06-09T15:30:07Z"}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `verdict` | int | 审批结果，见下表 |
| `tool_call_id` | string | 关联 model.tool_calls 中的 tool_call_id |
| `name` | string | 工具名 |
| `content` | string | 工具执行输出或错误信息 |
| `status` | string | `"success"` / `"error"` |

关联方式：`tool.tool_call_id == model.tool_calls[].tool_call_id`

### ToolVerdict 枚举（knot 包定义）

| 常量 | 值 | 含义 |
|------|-----|------|
| `Default` | 0 | 未审批 |
| `Approve` | 1 | 系统自动放行（brig 规则命中 Allow） |
| `Ask` | 2 | 需用户确认 |
| `Deny` | 3 | 系统自动拒绝（brig 规则命中 Deny） |
| `UserApprove` | 4 | 用户同意 |
| `UserDeny` | 5 | 用户拒绝 |
