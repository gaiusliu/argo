# Handoff — 2026-07-14（Interrupt 完成 + Asking 延迟持久化）

## 历史会话成果

- 2026-07-12：架构全景评审、kimi 提案、命名决策（commit `80c2538`）
- 2026-07-13：Tale 引入 + Voyage 合并 + DI 改造 #1 #2 #4 #6（commit `b30b187`、`a3a7847`）

## 本次会话成果（2026-07-14）

### Interrupt 能力

| 变更 | 文件 | 说明 |
|------|------|------|
| run.go ctx 检查 | `src/helm/run.go` | for 循环前 `select { case <-ctx.Done(): return }` |
| Signal 中断 | `src/cli/main.go` | `signal.Notify(os.Interrupt)` + `select` 双通道消费 events/sigCh。弃用 raw mode + ESC 方案 |
| Interrupt 端点 | `src/server/server.go` | `POST /session/interrupt` → `voyage.DeleteState(id)` |
| DeleteState | `src/voyage/voyage.go` | 删 state.json，不存在忽略 |
| askCLI 三态 | `src/cli/ask.go` | approve / deny / interrupt（输入 `stop` 或 `/interrupt`） |
| interruptSession | `src/cli/crud.go` | 客户端方法，调 `/interrupt` 端点 |

### Asking 延迟持久化

| 变更 | 文件 | 说明 |
|------|------|------|
| PendingMessages | `src/voyage/voyage.go` | Voyage 新增字段，Asking 时暂存未落盘的 user + assistant(tool_calls) |
| 去 checkpoint | `src/helm/phase_asking.go` | 不调 `checkpoint(v)`，改为 `v.PendingMessages = v.Tale().Unsaved()` |
| AppendMessages | `src/tale/tale.go` | 新方法，追加消息不改变持久化游标 |
| LoadState 恢复 | `src/voyage/voyage.go` | 反序列化后将 PendingMessages 恢复到 Tale |

### 会话历史

| 变更 | 文件 | 说明 |
|------|------|------|
| ResumeSessionResponse | `src/server/types.go` | 新增 Messages 字段，`/session/resume` 返回完整对话 |
| 历史展示 | `src/cli/session_pick.go` | 恢复已有会话时展示对话历史（截断 200 字符） |

### 新增设计决策

| 决策 | 说明 |
|------|------|
| DEC-039 | Signal 中断——Ctrl+C 截获触发 agent loop 终止，不用 raw mode + ESC |
| DEC-040 | Asking 延迟持久化——PhaseAsking 不写 vault，PendingMessages 暂存 state.json |

### 文档更新

| 文档 | 变更 |
|------|------|
| `docs/decisions.md` | +DEC-039、DEC-040 |
| `docs/voyage/design.md` | PendingMessages、中断回滚、DeleteState |
| `docs/modules/helm/design.md` | 全文重写：新签名、Asking 流程、中断机制 |
| `docs/modules/server/design.md` | 全文重写：`/interrupt` 端点、ResumeSessionResponse |
| `docs/architecture.md` | 中断流程（运行中 + Asking） |

### 未 commit

所有代码变更和文档更新均未提交，在 working tree 中。

## 已知残留问题（从前期继承）

| 问题 | 位置 | 说明 |
|------|------|------|
| 死代码 `sail.Window()` | `src/sail/sail.go:218` | 0 调用者 |
| `http.DefaultClient` 残留 | `src/sail/adapter.go:315`、`src/deck/builtin_tool.go:611,674` | 不走 ProviderOpenAI 路径 |
| `NewProviderOpenAI` 冗余参数 | `src/sail/sail.go` | messages/tools 参数已无意义 |
| 旧 `Sail` 接口 | `src/sail/sail.go:16` | 0 外部调用者 |
| `knot.LookupModel()` | `src/knot/config.go:150` | 仅被 `sail.Window()` 调用 |

## 待办清单

| # | 任务 | 复杂度 | 说明 |
|---|------|--------|------|
| 9 | **Asking 不断连** | 中 | 当前 goroutine 退出 + 客户端走新 HTTP 请求。改为同 SSE 连接内收 verdicts |
| 10 | **reef compact** | 高 | Tale 消息裁剪 + LLM 摘要 + 上下文重组。当前是桩。规约见 `docs/modules/reef/what.md` |
| 7 | **crew** | 高 | SKILL.md 扫描 + 渐进式披露 |
| 8 | **AgentService** | 中 | `src/server/prompt.go` handler 瘦身 |

## 未来可做

| 项 | 触发条件 |
|----|----------|
| 事件总线 | 多客户端同时操作同一 session |
| `helm.Machine` | 当 `newRunner` 需要超过 3 个共享参数时 |

## 建议技能

- `two-axis-review`：全部改造完成后做代码审查
- `grow-doc`：后续迭代生成迭代文档 + 模块 design.md
