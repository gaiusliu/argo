# vault

## 概述

vault 是 Argo 的纯存储层。负责对话 transcript 的持久化（Append/Load）和 Message ↔ Record 的单向转换。Session CRUD（创建/恢复/列表/删除）已迁至 voyage 模块（DEC-037）。MVP 阶段使用文件系统后端（`FileVault`），`Vault` 接口预留后端切换能力。

vault 依赖 knot（`Load` 返回 `[]knot.Message`），不依赖 helm/deck/sail。AGENTS.md 和 PROFILE.md 不经过 vault——前者由 helm 直接读取，后者留待后续的 deck 工具实现。

Record 字段详细设计见 [transcript.md](transcript.md)。

## 核心概念

| 概念 | 说明 |
|------|------|
| Vault | 统一存储接口，定义 Session CRUD + Transcript Append/Load，6 个方法 |
| FileVault | Vault 的文件系统实现，数据存储在 `~/.argo/sessions/{sessionID}/` 下 |
| SessionInfo | 会话元数据缓存（`session.json`）：ID、工作目录、时间、消息数、标题、token 用量 |
| Record | transcript 中单条记录，user/llm/tool 共用同一扁平 struct，JSON tag + omitempty 控制序列化 |
| RecordType | 三种 record 类型：`user`（用户输入）、`llm`（LLM 回复）、`tool`（工具结果） |
| ToolVerdict | 工具审批结果追踪，5 态：auto_allow / auto_deny / cached_allow / ask_allow / ask_deny |
| transcript.jsonl | 对话记录文件，每行一条 Record JSON，按 seq 自增顺序写入 |
| session.json | SessionInfo 的 JSON 缓存，CreateSession 写入，Append 时更新动态字段 |

## 行为合约

### Transcript 读写

- **Append**：读取 `session.json` 获取当前 `MessageCount`，以此为 base 分配 seq，逐行追加 Record JSON 到 `transcript.jsonl`。写入后更新 `session.json` 的动态字段（`LastActiveAt`、`MessageCount`、`Tokens`）
- **Load**：逐行读取 `transcript.jsonl`，跳过反序列化失败的行（记日志），过滤保留 user/llm/tool 三种 record，映射为 `[]knot.Message` 返回。文件不存在返回空切片，不报错

### Record → Message 转换

- user record → `Message{Role: user, Content: ...}`
- llm record → `Message{Role: assistant, Content: ..., ToolCalls: [...]}`。tool_call args 反序列化失败时记日志并跳过该条
- tool record → `Message{Role: tool, Content: ..., ToolCallID: ...}`
- system/compact/profile 类型 record 在 Load 时过滤掉

### 并发安全

- 每个 session 天然隔离（独立目录），无共享状态
- 文件锁 `LockDir` / `UnlockDir` 使用 `os.Mkdir` 原子目录锁（对标 opencode Flock 方案），含指数退避重试（DEC-034）

### 惰性创建

- CLI 启动时不立即创建 session，首条用户消息发出时才 `CreateSession()`
- `CreateSession` 仅需 `workingDir` 参数，模型名等元信息由后续 Append 间接记录
