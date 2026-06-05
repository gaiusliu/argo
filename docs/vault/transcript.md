# vault 对话记录（transcript）

> vault 负责对话记录的持久化存储。helm 每轮结束后调用 vault 写入新增消息，reef 和 LLM 通过 Read 工具按需回溯完整对话原文。

---

## 文件位置

```
~/.argo/sessions/{session_id}/transcript.jsonl
```

## 格式

JSONL，每行一条消息。最简 4 字段：

```jsonl
{"role":"user","content":"查 main.go 和 utils.go 里有没有用到 context 包","timestamp":"2026-06-05T14:30:00Z"}
{"role":"assistant","content":null,"tool_calls":[{"id":"call_001","name":"grep","args":{"pattern":"context","path":"main.go"}},{"id":"call_002","name":"grep","args":{"pattern":"context","path":"utils.go"}}],"timestamp":"2026-06-05T14:30:03Z"}
{"role":"tool","tool_call_id":"call_001","content":"main.go:15: import \"context\"","timestamp":"2026-06-05T14:30:04Z"}
{"role":"tool","tool_call_id":"call_002","content":"(无匹配)","timestamp":"2026-06-05T14:30:04Z"}
{"role":"assistant","content":"main.go 第 15 行引用了 context 包，utils.go 中没有。","timestamp":"2026-06-05T14:30:06Z"}
```

## 字段说明

| 字段 | 说明 | 哪些 role 使用 |
|------|------|--------------|
| `role` | user / assistant / tool | 全部 |
| `content` | 文本内容；assistant 有 tool_calls 时为 null；tool 时为执行结果 | 全部 |
| `tool_calls` | LLM 发起的工具调用列表，每项含 `id`（API 返回的调用 ID）+ `name`（工具名）+ `args`（完整参数） | 仅 assistant |
| `tool_call_id` | 关联到对应 tool_call 的 id | 仅 tool |
| `timestamp` | ISO 8601 时间戳 | 全部 |

## 写入时机

每轮 LLM 调用结束、工具执行完毕、消息列表更新后，helm 将本轮新增的消息（assistant 消息 + tool 结果消息）同步追加写入。

## 写入方式

同步追加，`f.WriteString(line + "\n")`，O(1) 磁盘操作，不阻塞主循环。不引入异步 buffer/flush 复杂度。

**参考**：pi 的 JSONL 追加模式。
