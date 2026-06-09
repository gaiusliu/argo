# vault 对话记录（transcript）

> vault 负责对话记录的持久化存储。helm 每轮结束后调用 vault 写入本轮产生的所有 record。CLI 模式下内存 messages 可做缓存，Gateway 模式下通过 vault 从 transcript 重建 messages。

---

## 文件位置

```
~/.argo/sessions/{session_id}/transcript.jsonl
```

## 格式

JSONL，每一行一条 record。每条 record 含 `type` 字段，共六种类型。所有 record 均有 `seq`（session 级自增整数，vault 分配）和 `timestamp`（ISO 8601）：

| type | 记录内容 |
|------|------|
| `user` | 用户输入 |
| `llm` | LLM 返回原文、模型、token 用量、工具调用请求 |
| `tool` | 工具执行结果、成功/失败状态 |
| `system` | system prompt 的初始组装及后续变更（含 Skill 激活/切换） |
| `compact` | 上下文压缩触发、裁剪前后 token 数 |
| `profile` | profile.md 的增删改 |

---

## 字段设计

### user

```jsonl
{"type":"user","seq":1,"content":"帮我重构 main.go","timestamp":"2026-06-09T15:30:00Z"}
{"type":"user","seq":5,"action":"interrupt","content":"","timestamp":"2026-06-09T15:30:05Z"}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | `"user"` |
| `seq` | int | session 级自增整数，vault 分配 |
| `content` | string | 用户输入原文；中断时可为空 |
| `action` | string or null | 特殊行为：`"interrupt"`（用户中断）；普通输入时省略 |
| `timestamp` | string | ISO 8601（RFC 3339） |

### llm

```jsonl
{"type":"llm","seq":2,"model":"claude-sonnet","content":null,"tool_calls":[{"call_id":"tc1","name":"read","args":{"filePath":"main.go"}}],"usage_in":30000,"usage_out":150,"timestamp":"2026-06-09T15:30:03Z"}
{"type":"llm","seq":6,"model":"claude-sonnet","content":"重构完成","tool_calls":[],"usage_in":31100,"usage_out":80,"timestamp":"2026-06-09T15:30:09Z"}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | `"llm"` |
| `seq` | int | session 级自增整数，vault 分配 |
| `model` | string | 使用的模型名称 |
| `content` | string or null | LLM 文本输出；仅有工具调用时为 null |
| `tool_calls` | array | `[{call_id, name, args}]`，无工具调用时为空数组或省略 |
| `usage_in` | int | 本轮输入 token |
| `usage_out` | int | 本轮输出 token |
| `timestamp` | string | ISO 8601（RFC 3339） |

### tool

```jsonl
{"type":"tool","seq":3,"call_id":"tc1","name":"read","content":"package main\n\nimport \"fmt\"\n...","status":"success","dur_ms":120,"timestamp":"2026-06-09T15:30:04Z"}
{"type":"tool","seq":4,"call_id":"tc2","name":"bash","content":"bash: xxx: command not found","status":"error","dur_ms":45,"timestamp":"2026-06-09T15:30:07Z"}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | `"tool"` |
| `seq` | int | session 级自增整数，vault 分配 |
| `call_id` | string | 关联 llm.tool_calls 中的 call_id |
| `name` | string | 工具名 |
| `content` | string | 工具执行输出 |
| `status` | string | `"success"` / `"error"` / `"timeout"` |
| `dur_ms` | int | 执行耗时（毫秒） |
| `timestamp` | string | ISO 8601（RFC 3339） |

关联方式：`tool.call_id == llm.tool_calls[].call_id`

### system

```jsonl
{"type":"system","seq":1,"action":"build","skill":null,"tools":["bash","read","write","edit","grep","glob"],"content":"<system prompt 全文>","timestamp":"2026-06-09T15:30:00Z"}
{"type":"system","seq":50,"action":"update","skill":"learn","tools":["bash","read","write","edit","grep","glob"],"content":"<system prompt 全文>","timestamp":"2026-06-09T15:31:00Z"}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | `"system"` |
| `seq` | int | session 级自增整数，vault 分配 |
| `action` | string | `"build"`（初始组装）/ `"update"`（Skill 切换等变更） |
| `skill` | string or null | 当前激活的 Skill 名，未激活时为 null |
| `tools` | string[] | 当前可用工具名列表 |
| `content` | string | system prompt 全文 |
| `timestamp` | string | ISO 8601（RFC 3339） |

### compact

只在实际触发压缩时写入，未触发则不产生记录。

```jsonl
{"type":"compact","seq":45,"trigger":"auto","before":45000,"after":30000,"content":"<被压缩消息的摘要>","timestamp":"2026-06-09T15:30:02Z"}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | `"compact"` |
| `seq` | int | session 级自增整数，vault 分配 |
| `trigger` | string | `"auto"` / `"manual"` |
| `before` | int | 压缩前消息 token 数 |
| `after` | int | 压缩后消息 token 数 |
| `content` | string | 被压缩消息的摘要 |
| `timestamp` | string | ISO 8601（RFC 3339） |

### profile

```jsonl
{"type":"profile","seq":100,"action":"update","old":"用户偏好 Go 方案","new":"用户偏好 Rust 方案","reason":"用户说以后所有新项目都用 Rust，不再用 Go","timestamp":"2026-06-09T15:30:10Z"}
{"type":"profile","seq":101,"action":"insert","new":"每周五下午不接新需求","reason":"用户上周明确提过两次","timestamp":"2026-06-09T16:00:00Z"}
{"type":"profile","seq":102,"action":"delete","old":"喜欢先看测试再写代码","reason":"用户说这个习惯已经改了","timestamp":"2026-06-09T17:00:00Z"}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | `"profile"` |
| `seq` | int | session 级自增整数，vault 分配 |
| `action` | string | `"insert"` / `"update"` / `"delete"` |
| `old` | string | 变更前内容；insert 时省略 |
| `new` | string | 变更后内容；delete 时省略 |
| `reason` | string | 变更原因。说明是用户明确要求还是从上下文推断，尽量引用对话原话 |
| `timestamp` | string | ISO 8601（RFC 3339） |
