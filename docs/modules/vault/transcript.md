# vault 对话记录（transcript）

> vault 负责对话记录的持久化存储。helm 每轮结束后调用 vault 写入本轮产生的所有 record。

## 文件位置

```
~/.argo/sessions/{session_id}/transcript.jsonl
```

## 格式

JSONL，每一行一条 record。每条 record 含 `type` 字段，当前共三种类型。所有 record 均有 `seq`（session 级自增整数，vault 分配）和 `timestamp`（ISO 8601）：

| type | 记录内容 |
|------|------|
| `user` | 用户输入 |
| `llm` | LLM 返回原文、模型、token 用量、工具调用请求 |
| `tool` | 工具执行结果、审批状态、成功/失败状态 |

> system / compact / profile 三种类型推迟到对应模块（crew / reef / 后续）实现时再加入。

## 字段设计

### 公共字段

每条 record 均包含：

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | `"user"` / `"llm"` / `"tool"` |
| `seq` | int | session 级自增整数，vault 分配 |
| `timestamp` | string | ISO 8601（RFC 3339） |

### user

```jsonl
{"type":"user","seq":1,"content":"帮我重构 main.go","timestamp":"2026-06-09T15:30:00Z"}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `content` | string | 用户输入原文 |

### llm

```jsonl
{"type":"llm","seq":2,"model":"claude-sonnet","content":null,"tool_calls":[{"tool_call_id":"tc1","name":"read","args":{"filePath":"main.go"}}],"input_tokens":30000,"output_tokens":150,"timestamp":"2026-06-09T15:30:03Z"}
{"type":"llm","seq":6,"model":"claude-sonnet","content":"重构完成","tool_calls":[],"input_tokens":31100,"output_tokens":80,"timestamp":"2026-06-09T15:30:09Z"}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `model` | string | 使用的模型名称 |
| `content` | string or null | LLM 文本输出；仅有工具调用时为 null |
| `tool_calls` | array | `[{tool_call_id, name, args}]`，无工具调用时为空数组或省略 |
| `input_tokens` | int | 本轮输入 token（暂不填充，预留） |
| `output_tokens` | int | 本轮输出 token（暂不填充，预留） |

### tool

```jsonl
{"type":"tool","seq":3,"verdict":"auto_allow","tool_call_id":"tc1","name":"read","content":"package main\n\nimport \"fmt\"\n...","status":"success","dur_ms":120,"timestamp":"2026-06-09T15:30:04Z"}
{"type":"tool","seq":4,"verdict":"ask_deny","tool_call_id":"tc2","name":"bash","content":"user rejected: tool name: bash - permission required","status":"error","dur_ms":0,"timestamp":"2026-06-09T15:30:07Z"}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `verdict` | string | 审批结果，见下表 |
| `tool_call_id` | string | 关联 llm.tool_calls 中的 tool_call_id |
| `name` | string | 工具名 |
| `content` | string | 工具执行输出或被拒绝的原因 |
| `status` | string | `"success"` / `"error"` |
| `dur_ms` | int | 执行耗时（毫秒），暂不填充，预留 |

关联方式：`tool.tool_call_id == llm.tool_calls[].tool_call_id`

### ToolVerdict 枚举

| 值 | 含义 | 触发条件 |
|------|------|------|
| `auto_allow` | 系统自动放行 | brig 静态规则命中 Allow（如 safeCommands） |
| `auto_deny` | 系统自动拒绝 | brig 静态规则命中 Deny（如 denyRules） |
| `cached_allow` | 记忆放行 | 命中用户之前的审批记录，本轮未问 |
| `ask_allow` | 用户同意 | brig 返回 Ask，用户当场确认 |
| `ask_deny` | 用户拒绝 | brig 返回 Ask，用户当场拒绝 |
