# 009：Vault 唯一数据源 — 代码对齐 DEC-037

**日期**：2026-07-11 ~
**状态**：✅ 完成
**涉及模块**：helm、vault、server、cli

## 目标

将代码行为对齐 DEC-037 和 helm design.md 中已确立的设计：transcript.jsonl 为对话历史唯一真相源，state.json 只保留控制流快照，消除双写导致的重复记录、协议不一致和无效 IO。

## 背景

排查两轮对话中"思考内容多且偏"的问题时，发现四个关联 bug：

1. **writeRecords 写全量而非增量**：每轮 PhaseDone 时把 `st.Messages` 全部追加到 transcript，Vault 不检查重复，导致对话记录指数级膨胀。LLM 收到重复消息后思考分裂。

2. **CLI/Server 协议不匹配**：Server 写的 `EventJSON` 曾缺少 `AskingToolUses`/`DeniedToolUses` 字段，CLI 反序列化时 EventAsk 的审批工具列表被静默丢弃，CLI 不弹审批提示。（已修复：协议统一为 `EventJSON`，补全字段）

3. **state.json 和 transcript.jsonl 双写 Messages**：两个文件各自维护一份 Messages，写入时机和完整性不同。Ask 回复路径轮空时 `state.json` 缺失 Messages，崩溃恢复不可靠。

4. **handlePrompt 无条件读 transcript**：Ask 回复路径曾无条件 `session.Load()`，但紧接着 `st.Load` 覆盖 `st.Messages`，造成无效磁盘 IO。（已修复：Ask 回复路径跳过 transcript 读取）

## 范围

### 包含

- writeRecords 从全量写入改为增量写入
- handlePrompt 按分支决定是否读 transcript（Ask 回复路径跳过）
- state.json 去掉 Messages 字段，仅保留 Phase / Model / ToolUses / Saved 控制流
- vault 通过 Saved 游标实现增量写入，天然去重

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
| checkpoint 统一移入 PhaseRunnerAsking / PhaseRunnerDone，ExecTools 不再调用 | `phase_exec_tools.go` |
| Run 中 HelmPhaseAsking 独立分支，调用 PhaseRunnerAsking 后 return | `run.go` |
| CLI SSE 协议对齐：补全 AskingToolUses / DeniedToolUses 反序列化 | `cli/convert.go`、`cli/sse.go` |
| Server ToolUseJSON 新增 VerdictReason 字段 | `server/event_json.go` |

## 整体开发状态

| 功能 | 状态 | 涉及文件 |
|------|------|---------|
| 状态机四阶段拆分 + checkpoint 收敛 | ✅ 完成 | `phase_model.go`、`phase_asking.go`、`phase_done.go`、`phase_exec_tools.go`、`run.go` |
| CLI/Server EventJSON 协议对齐 | ✅ 完成 | `cli/convert.go`、`cli/sse.go`、`server/event_json.go` |
| checkpoint 全量 → 增量写入（Saved 游标） | ✅ 完成 | `phase_model.go` |
| handlePrompt Ask 回复路径跳过 transcript 读取 | ✅ 完成 | `server/prompt.go` |
| state.json 去掉 Messages 字段（`json:"-"`） | ✅ 完成 | `helm/helm_state.go` |
| Vault 去重/幂等保护（Saved 游标天然去重） | ✅ 完成 | `helm/helm_state.go` |

## 关键决策

| 日期 | 决策 | 原因 |
|------|------|------|
| 2026-07-09 | DEC-037：Vault 为对话历史唯一数据源，LoopState 去 Messages | 消除双写不一致，state.json 体积恒定 |
| 2026-07-11 | 本迭代：代码对齐 DEC-037 | 排查思考污染时发现四处代码偏差 |

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-07-12 | 全部 6 项完成：增量写入 + transcript 跳过 + Messages json:"-" + Saved 游标去重；符号去 New 后缀转正 |
| 2026-07-11 | 迭代开始；四 bug 排查完成；状态机拆分 + 协议对齐已提交 |
