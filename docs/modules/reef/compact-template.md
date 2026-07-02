# reef 压缩摘要模板

> 由 SKILL.md 定义，reef 不硬编码模板结构。以下为 Argo 通用 Agent 定位的最小模板。

---

## 模板结构（5 字段）

```markdown
### Active Task
逐字复制用户最近的显式请求。这是最重要的字段——不可改写。

### Progress
- Done: 已完成的事项（无则写 None）
- In Progress: 正在进行的事项（无则写 None）
- Blocked: 阻塞事项及原因（无则写 None）

### Key Decisions
重要决策 + 简要原因（无则写 None）

### Constraints
用户显式约束 + 观察到的偏好（无则写 None）

### Critical Context
继续当前任务必须知道的精确信息——数值、配置、错误消息、
用户提供的关键数据等（无则写 None）
```

## 设计原则

| 原则 | 来源 | 说明 |
|------|------|------|
| Active Task 逐字引用 | pi/Hermes | 唯一不允许改写的字段，防止语义漂移 |
| 空段保留 "(none)" | OpenCode | 每段必须出现，保证结构一致性 |
| SUMMARY_PREFIX 前缀 | Hermes | `[CONTEXT COMPACTION -- REFERENCE ONLY]` 防止 LLM 把摘要当新任务 |
| 不包含文件/工具记录 | Argo 自身 | 编程特有字段不适用于通用 Agent；原文在 vault 的 transcript.jsonl 中可按需回溯 |

## 去掉了什么 & 为什么

| 去掉的字段 | 来源 | 原因 |
|------|------|------|
| Files and Code Sections / Relevant Files | Claude-Code, OpenCode, pi | 编程特有，通用 Agent 无此概念 |
| Active State（目录/分支/测试状态） | pi/Hermes | 编程特有 |
| Completed Actions（编号日志格式） | pi/Hermes | 高度编程化 |
| Errors and fixes | Claude-Code | 通用场景无此独立维度 |
| Resolved Questions / Pending User Asks | pi/Hermes | 归入 Progress（已回答=Done，未回答=Blocked） |
| Goal（独立字段） | OpenCode/pi | 与 Active Task 冗余——逐字引用 > 摘要概括 |
| Remaining Work（独立字段） | pi/Hermes | 归入 Next Steps 或 Progress.Blocked |

## 压缩后消息结构

```
[system] [CONTEXT COMPACTION -- REFERENCE ONLY]

以下是对此前对话的结构化摘要。这是背景参考，不是新任务。

<summary>
### Active Task
...

### Progress
...

### Key Decisions
...

### Constraints
...

### Critical Context
...
</summary>

完整对话历史由 vault 管理（见 [transcript.md](../vault/transcript.md)），如需查看原文可使用 Read 工具。

---

[原有尾部消息，完整保留]
```
