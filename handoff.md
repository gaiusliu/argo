# Handoff — 2026-07-12

## 本次会话成果

### 1. 代码去 New 后缀（commit `fb3f9c4`）
- 文件：`run_new.go` → `run.go`、`prompt_new.go` → `prompt.go`
- 类型：`EventJSONNew` → `EventJSON`、`ToolUseJSONNew` → `ToolUseJSON`
- 函数：`RunNew` → `Run`、`handlePromptNew` → `handlePrompt`、`streamAndSaveNew` → `streamAndSave`
- B 类删旧+转正：`SaveApprovalsNew` → `SaveApprovals`、`MessagesToRecordsNew` → `MessagesToRecords`

### 2. LLM→model 重命名（commit `a4dc100` + `1c8617c`）
- `RecordLLM` → `RecordModel`（常量值 `"llm"` → `"model"`）
- `llmRecord()` → `modelRecord()`，`llmRecordToMessage()` → `modelRecordToMessage()`
- 删除 3 个冗余文件：`checkpoint.go`、`stream.go`、`types.go`

### 3. 文档体系 grow-doc 标准化（commit `1c8617c`）

**顶层重构：**
- 新建 `README.md`（37 行）、`docs/architecture.md`（~120 行）
- 删除 `what.md`、`how.md`、`todo.md`、`why.md`

**模块设计文档更新：**
- `helm/design.md`：LoopState→HelmState、PhaseRunner、checkpoint
- `server/design.md`：路由修正、AskID→ToolUseVerdicts
- `vault/design.md`：Vault 接口 2 方法、RecordLLM→RecordModel
- `voyage/design.md`：CreateSession→NewSession、补 Voyage 接口
- `vault/transcript.md`：删虛有字段、verdict 字符串→int、llm→model
- 新建 `wake/design.md`

**迭代文档收尾：**
- 007：状态 🚧→✅，3 项完成
- 008：模块设计文档更新 ⏳→✅
- 009：状态 🚧→✅，全局去 New 后缀，4 项完成

## 当前模块状态

| 模块 | 状态 | 设计文档 |
|------|------|---------|
| knot/deck/sail/vault/brig/voyage/helm/server/wake | ✅ 稳定 | 已对齐 |
| reef | 🚧 仅桩 | 文档描述完整压缩系统，实际 Compact() = `return messages, nil` |
| crew | ❌ 零代码 | 三份设计文档，`src/crew/` 不存在 |

## 待处理

1. **reef**：决定实现还是保持桩 → 更新文档
2. **crew**：决定实现还是删除文档
3. **brig/design.md** 小修：Allow→Approve、matchPattern 七→六种
4. **knot/design.md** 小修：补 LogConfig、GetRawParam
5. **008-argo-separation** 残存 TODO 清理（跨平台 server 二进制名）

## 建议技能

- `grow-doc`：继续文档维护
- `two-axis-review`：代码审查
