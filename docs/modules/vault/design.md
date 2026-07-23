# vault

## 概述

vault 是 Argo 的纯存储层。负责对话 transcript 的读写（Append/Load）和 Message ↔ Record 的单向转换。Session CRUD 已迁至 voyage 模块（DEC-037）。MVP 阶段使用文件系统后端（`FileVault`），`Vault` 接口预留后端切换能力。

vault 只依赖 knot（`Load` 返回 `[]knot.Message`），不依赖 helm/deck/sail。每个 `FileVault` 实例绑定一个 session 目录，不知道 session ID 的存在。

Record 字段详细设计见 [transcript.md](transcript.md)。

## 核心概念

| 概念 | 说明 |
|------|------|
| Vault | 纯存储接口，只有 2 个方法：`Append(records []Record) error` + `Load() ([]knot.Message, error)` |
| FileVault | Vault 的文件系统实现，绑定到 session 目录，所有操作限定在此目录内 |
| Record | transcript 中单条记录，user/model/tool 共用同一扁平 struct，JSON tag + omitempty 控制序列化 |
| RecordType | 四种 record 类型：`RecordUser`（用户输入）、`RecordModel`（模型回复）、`RecordTool`（工具结果）、`RecordCompact`（压缩标记持久化） |
| transcript.jsonl | 对话记录文件，每行一条 Record JSON，FileVault.Append 逐行追加 |
| MessagesToRecords | 将 `[]knot.Message` 单向转为 `[]Record`，需要 `map[string]knot.ToolUse` 补充工具元数据（verdict/name/status） |

## Vault 接口

```go
type Vault interface {
    Append(records []Record) error
    Load() ([]knot.Message, error)
}
```

| 方法 | 说明 |
|------|------|
| Append | 逐行追加 Record JSON 到 transcript.jsonl，自动填充 Timestamp |
| Load | 读取 transcript.jsonl，逐行反序列化，跳过损坏行，映射为 `[]knot.Message`。文件不存在返回空切片 |

## 行为合约

### Transcript 读写

- **Append**：打开 transcript.jsonl（追加模式，不存在则创建），每条 Record 自动打上 `time.Now().UTC()` 时间戳，`json.Encoder` 逐行写入。不涉及 session.json（元数据更新由上层 voyage 负责）
- **Load**：逐行读取 transcript.jsonl，跳过反序列化失败的行（slog.Error），按 type 过滤映射：
  - `RecordUser` → `Message{Role: user, Content: ...}`
  - `RecordModel` → `Message{Role: assistant, Content: ..., ToolCalls: [...]}`
  - `RecordTool` → `Message{Role: tool, Content: ..., ToolCallID: ...}`
  - 其余 type 忽略

### Message → Record 转换（MessagesToRecords）

- user msg → `Record{Type: RecordUser, Content: ...}`
- assistant msg → `Record{Type: RecordModel, Content: ..., Model: modelName, ToolCalls: [...]}`
- tool msg → `Record{Type: RecordTool, Content: ..., ToolCallID: ..., Name: toolUse.Name, Verdict: toolUse.VerdictResult, Status: "success"/"error"}`

转换需要 `toolUsesByIDs map[string]knot.ToolUse` 作为辅助输入——tool role message 本身不携带工具名和 verdict，需从 HelmState.ToolUses 中按 ToolCallID 查找补充。`Compact` 类型的消息直接写入 `RecordCompact`，记录被压缩的消息区间和 LLM 摘要正文。

### 并发安全

- 每个 session 天然隔离（独立目录），无共享状态
- 并发控制由上层 voyage 的调用时序保证

## 关键文件

| 文件 | 职责 |
|------|------|
| `vault.go` | Vault 接口定义 |
| `file_vault.go` | FileVault 实现：Append（writeRecords）+ Load（readRecords → recordsToMessages） |
| `record.go` | Record 结构体 + RecordType 常量 + ToolCallRecord |
| `convert.go` | MessagesToRecords：Message → Record 单向转换 + ToolRecordMeta |
