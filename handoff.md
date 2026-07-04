# Handoff — sail MVP 收尾 → CLI + 权限审批 → vault

## 背景

sail MVP 全部 9 项功能已完成：SSE 解析 + 状态机 + ctx 安全退出 + JSON 请求体转换 + 错误分类/重试 + models.dev 元数据 + tools 占位。代码能编译但**还不能跑**——缺少 CLI 入口和权限审批。

## 当前进度总览

| 模块 | 状态 | 产出 |
|------|------|------|
| deck | ✅ | 工具注册/发现/调用，内置工具 + CLI 外部工具 |
| sail | ✅ | LLM 流式接入，OpenAI + Compatible adapter，6 类错误分类，models.dev 元数据 |
| helm | 🚧 | RunLoop 五步闭环可编译，缺 Approver 注入，未端到端测试 |
| reef | 🚧 | Compact() 桩（原样返回），待实现真实压缩 |
| vault | ❌ | 未开工 |
| crew | ❌ | 未开工 |
| cmd | ❌ | 未开工 |

## 下一步

决定先跳过 vault/crew，优先做能**让整个流程跑起来**的功能：

### 1. 权限审批（Approver 接口 + CLI 实现）

**理由**：没有审批的 Agent 是危险的——所有工具直接执行。审批是流程测试的前置条件。

- `knot/types.go`：新增 `Approver` 接口（`Approve(toolUse ToolUse) (bool, error)`）
- `helm/loop.go`：在工具执行前注入 Approver——`Lookup` 命中后、`Execute` 之前，调用 `Approver.Approve()`
- 首次用 CLI 审批实现：工具名称 + 参数打印到 stderr，等用户输入 `y/n`

### 2. 极简 CLI（`cmd/run/main.go`）

**理由**：有了它才能实际跑 agent 循环、验证 sail→helm 链路。

- 加载配置 → 注册工具 → 循环读 stdin → `helm.RunLoop` → stdout 流式输出
- 硬编码默认模型名（从 config 取），不做 CLI 参数解析
- Ctrl+C 退出

### 3. vault — session 持久化 + transcript 写入

**理由**：跑完的会话没有记录等于白跑。

- session 创建/加载/列表（归属 vault，不作为独立模块）
- transcript.jsonl 追加写入（6 种 type）
- profile.md 读写、AGENTS.md 只读加载

### 4. session 切换

在 vault 基础上支持多会话管理。

## 重要决策

### session 归属

**session 归属 vault，不自成模块。**

理由：
- vault 职责是"所有文件持久化"——session 的创建/加载/列表本质是文件 IO
- 参照 pi（agent-session.ts 在 coding-agent 内）和 kilocode（session/ 自成模块），Argo MVP 选最简单的方案
- helm 已有 `TurnState` 运行时状态，vault 提供 `LoadSession(id) → TurnState` 和 `SaveSession(state)` 即可
- 初期用户不需要多会话切换，一个 session 跑通就是里程碑

## 文件清单（本阶段新增）

- `src/sail/errors.go` — `APIError` + `classifyError` + `parseOpenAIError`
- `src/knot/metadata.go` — `fetchMetadata` + 本地缓存 + `LookupWindow`

## 已有决策

- DEC-022：一份 SSE + 两个入口
- DEC-023：`context.AfterFunc` 替代手动 goroutine
- DEC-024：tools 字段占位不填
- DEC-025：models.dev 运行时拉取元数据
- DEC-026：6 类错误分类 + adapter 层短延迟重试
