# Handoff — sail MVP 流式事件处理

## 背景

本会话完成了 sail 模块的流式事件处理核心链路：SSE 解析 + 状态机 + ctx 安全退出 + JSON 请求体转换。从 `Chat()` 入口到 `<-chan knot.Event` 出口的完整数据通路已打通。

## 本次产出

### 1. 流式事件状态机（`streamState`）

参考 kilocode 的 `transform()` 和 pi 的 `ensureXxxBlock()` 模式，在 `streamState.handleChunk()` 中实现了 text/thinking/tool_use 三块的 start/delta 自动判断：

- 用 `textActive` / `thinkingActive` 布尔 + `toolCalls map[int]string` 跟踪状态
- 首次出现 content → `EventTextStart`；后续 → `EventTextDelta`
- 首次出现 reasoning → `EventThinkingStart`；后续 → `EventThinkingDelta`
- 首次出现新 tool_call index → `EventToolUseStart`；后续 → `EventToolUseDelta`
- 状态转换（reasoning → text 或 tool_calls）时自动关闭 reasoning block
- `flush()` 在 `[DONE]` / EOF 时关闭所有活跃 block

### 2. SSE 解析 + ctx 安全退出

- `context.AfterFunc(ctx, body.Close)` 替代手动 goroutine——回调挂到 ctx 取消传播链，零泄漏
- `defer stop()` 在 scanner 先结束时摘除回调，避免双 Close
- scanner.Err 和 ctx.Err 各自独立判断，分别推 EventError

### 3. JSON 请求体转换

- `buildOpenAIBody` 将 `knot.ChatRequest.Messages` 转为 OpenAI Chat Completions JSON
- `openAIMsg` 独立于 `knot.Message`——领域类型和协议适配各司其职，为 Anthropic adapter 预留
- 当前不含 `tools` 字段

### 4. ChatRequest 简化

- 删除 `SystemPrompt` 字段，system prompt 作为 `MessageRoleSystem` 消息放入 Messages 首位
- `helm.RunLoop` 中 `buildSystemPrompt` 结果包装为 `knot.Message{Role: MessageRoleSystem}`

### 5. Adapter 架构

- 一份 SSE 实现 + 两个入口函数：`OpenAIChat` + `CompatibleChat`
- 参考 pi 的 provider=配置对象模式（DEC-022）
- baseURL 常量 `openAIBaseURL` 在 `openai.go` 定义

### 6. 配置加载

- `knot.GetConfig()` 用 `sync.Once` 惰性加载

## 文件清单

新增：
- `src/sail/adapter.go` — `streamState` + `parseSSE` + `doChat` + `buildOpenAIBody`
- `src/sail/openai.go` — `OpenAIChat`
- `src/sail/compatible.go` — `CompatibleChat`
- `docs/modules/sail/design.md` — sail 模块设计文档

修改：
- `src/knot/types.go` — 删 `ChatRequest.SystemPrompt`、加 `MessageRoleSystem`
- `src/knot/config.go` — 加 `GetConfig()`（sync.Once）
- `src/sail/sail.go` — `Chat` 实现配置路由 + provider dispatch
- `src/helm/loop.go` — system prompt 改为 Message 入队
- `docs/decisions.md` — 加 DEC-022、DEC-023
- `docs/glossary.md` — 加 SSE、choices
- `docs/modules/sail/decisions.md` — 删除（已合并至全局 decisions.md）

## 依赖关系

```
knot (零外部依赖)
├── deck → knot
├── sail → knot
├── reef → knot
└── helm → deck + sail + reef + knot
```

## 当前进度

| # | 功能 | 状态 |
|---|------|------|
| 1 | 核心类型定义 | ✅ |
| 2 | 配置加载（GetConfig） | ✅ |
| 3 | SSE 解析 + 流式状态机 | ✅ |
| 4 | JSON 请求体转换 | ✅（缺 tools 字段） |
| 5 | Provider Adapter ×2 | ✅ |
| 6 | Chat 配置路由 | ✅ |
| 7 | 错误分类 + 重试 | ⏳ |
| 8 | 模型元数据内置表（Window） | ⏳ |
| 9 | Tools 字段加入请求体 | ⏳ |

## 下一步

1. **`tools` 字段**：`buildOpenAIBody` 中加 tools 字段，需要确定 `knot.Tool` 接口是否需要 `Parameters()` 方法
2. **错误分类**：DEC-S03 的 6 类错误 + 重试逻辑在 `doChat` 层实现
3. **模型元数据内置表**：实现 `Window()` 从内置表查 context window

## 重要决策

- DEC-022：一份 SSE 实现 + 两个入口函数
- DEC-023：`context.AfterFunc` 替代手动 goroutine
- 状态机设计见 [docs/modules/sail/design.md](docs/modules/sail/design.md)

## suggested skills

- code-review：审查本次改动
