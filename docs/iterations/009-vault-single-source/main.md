# 009：Vault 唯一数据源 — 代码对齐 DEC-037

**日期**：2026-07-11 ~
**状态**：🚧 进行中
**涉及模块**：helm、vault、server、cli

## 目标

将代码行为对齐 DEC-037 和 helm design.md 中已确立的设计：transcript.jsonl 为对话历史唯一真相源，state.json 只保留控制流快照，消除双写导致的重复记录、协议不一致和无效 IO。

## 背景

排查两轮对话中"思考内容多且偏"的问题时，发现四个关联 bug：

1. **writeRecordsNew 写全量而非增量**：每轮 PhaseDone 时把 `st.Messages` 全部追加到 transcript，Vault 不检查重复，导致对话记录指数级膨胀。LLM 收到重复消息后思考分裂。

2. **CLI/Server 协议不匹配**：Server 写 `EventJSONNew`（含 `AskingToolUses`/`DeniedToolUses`），CLI 读 `EventJSON`（不含这些字段），EventAsk 的审批工具列表被静默丢弃，CLI 不弹审批提示。

3. **state.json 和 transcript.jsonl 双写 Messages**：两个文件各自维护一份 Messages，写入时机和完整性不同。Ask 回复路径轮空时 `state.json` 缺失 Messages，崩溃恢复不可靠。

4. **handlePromptNew 无条件读 transcript**：Ask 回复路径 `st.Load` 紧接着覆盖 `st.Messages`，`session.Load()` 是无效磁盘 IO。

## 范围

### 包含

- writeRecordsNew 从全量写入改为增量写入
- handlePromptNew 按分支决定是否读 transcript（Ask 回复路径跳过）
- state.json 去掉 Messages 字段，仅保留 Phase / Model / ToolUses 控制流
- vault.FileVault 或 RecordsToMessages 增加去重/幂等保护

### 不含

- vault 存储后端切换（仍用 FileVault）
- transcript 压缩/归档
- 跨 session 的 transcript 迁移

## 公共设计约束

1. **transcript.jsonl 是 Messages 的唯一持久化来源**——任何恢复路径都从 Vault.Load 获取对话历史
2. **state.json 大小恒定**——不随对话轮次增长
3. **增量写入**——每次 Append 只写本轮新增的 record，不重复写已有内容
4. **幂等恢复**——同一 transcript 多次 Load 得到相同 Messages

## 已修复（commit 75e5a96）

| 修复项 | 涉及文件 |
|--------|---------|
| 状态机四阶段拆分：PhaseRunnerModel / ExecTools / Asking / Done 独立文件 | `phase_model.go`（rename）、`phase_asking.go`（new）、`phase_done.go`（new） |
| writeRecordsNew 统一移入 PhaseRunnerDone，ExecTools/Asking 不再调用 | `phase_exec_tools.go` |
| RunNew 中 HelmPhaseAsking 独立分支，调用 PhaseRunnerAsking 后 return | `run_new.go` |
| CLI SSE 协议对齐：EventJSON → EventJSONNew，补全 AskingToolUses / DeniedToolUses 反序列化 | `cli/convert.go`、`cli/sse.go` |
| Server ToolUseJSONNew 新增 VerdictReason 字段 | `server/event_json.go` |

## 整体开发状态

| 功能 | 状态 | 涉及文件 |
|------|------|---------|
| 状态机四阶段拆分 + writeRecordsNew 收敛 | ✅ 完成 | `phase_model.go`、`phase_asking.go`、`phase_done.go`、`phase_exec_tools.go`、`run_new.go` |
| CLI/Server EventJSON 协议对齐 | ✅ 完成 | `cli/convert.go`、`cli/sse.go`、`server/event_json.go` |
| writeRecordsNew 全量 → 增量写入 | ⏳ 待开始 | `phase_model.go`、`vault/file_vault.go` |
| handlePromptNew Ask 回复路径跳过 transcript 读取 | ⏳ 待开始 | `server/prompt_new.go` |
| state.json 去掉 Messages 字段 | ⏳ 待开始 | `helm/helm_state.go` |
| Vault 去重/幂等保护 | ⏳ 待开始 | `vault/file_vault.go` 或 `vault/convert.go` |

## 关键决策

| 日期 | 决策 | 原因 |
|------|------|------|
| 2026-07-09 | DEC-037：Vault 为对话历史唯一数据源，LoopState 去 Messages | 消除双写不一致，state.json 体积恒定 |
| 2026-07-11 | 本迭代：代码对齐 DEC-037 | 排查思考污染时发现四处代码偏差 |

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-07-11 | 迭代开始；四 bug 排查完成；状态机拆分 + 协议对齐已提交 |
