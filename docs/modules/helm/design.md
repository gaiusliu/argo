# helm

## 概述

helm 是 Argo 的 Agent 核心循环。它是一个无状态的自由函数 `RunLoop`，接收一次用户输入，驱动 "LLM 调用 → 工具执行 → 结果注入 → 循环" 的完整链路，返回最终回复。helm 不持有会话状态——消息历史、回合计数等由调用方（CLI / Gateway）维护并传入。brig 规则引擎由调用方注入，helm 在工具执行前调 brig 做权限门控。Ask 确认通过 `EventAsk` 事件与调用方交互。

## 核心概念

| 概念 | 说明 |
|---|---|
| runLoop | 无状态自由函数，执行 ReAct 循环，返回 `<-chan Event` 供调用方流式消费 |
| TurnState | 会话状态（消息历史、回合计数、当前模型名），由调用方持有并传入传出 |
| Event | 流式事件，13 种类型（见下方"流式事件类型"） |
| ReAct 循环 | 思考（LLM）→ 行动（工具执行）→ 观察（结果注入）的闭环，LLM 无 tool_calls 即退出 |

## 流程

### 单轮主循环

```
调用方传入 state（含首次 user message）
  │
  └─ RunLoop(ctx, &state, eng)
       │
       ├─ 会话级缓存（循环前执行一次）
       │    ├─ deck.List()               → 工具列表
       │    └─ buildSystemPrompt()       → system prompt
       │
       ├─ for（无终止条件，LLM 自行决定退出）
       │    │
       │    ├─ 1. reef.Compact()              → 裁剪消息历史（失败降级）
       │    ├─ 2. sail.Chat(state.ModelName)  → 流式 LLM 调用
       │    │      ├─ start / textStart / textDelta / textDone  → 转发 + 累计
       │    │      ├─ thinkingStart / thinkingDelta / thinkingDone → 转发 + 累计
       │    │      └─ 收集 tool_use（不立即转发，等获批后再发）
       │    │
       │    ├─ 3. 无 tool_use？→ 注入纯文本 assistant message → 退出循环
       │    │
       │    ├─ 4. 有 tool_use → 注入带 tool_calls 的 assistant message
       │    │      → deck.Lookup() → brig 门控 → Tool.Execute()
       │    │      ├─ brig.Evaluate(tu) → Allow / Ask / Deny
       │    │      ├─ Ask → EventAsk{AskReply} → 等调用方回传 → eng.Approve()
       │    │      ├─ 获批 → EventToolUseDelta → Execute → EventToolUseDone
       │    │      └─ 被拒 → EventToolUseDone（Status=Error）
       │    │
       │    └─ 5. 结果注入 state.Messages → 回到步骤 1
       │
       └─ 返回最终 state
```

### system prompt 组装

```
buildSystemPrompt(tools)
  ├─ 行为核心（常量）
  ├─ 环境信息（Platform / 日期 / Global config）
  ├─ 工具列表（deck 遍历）
  └─ AGENTS.md（不存在跳过）
```

### 流式事件类型

一次完整调用的事件序列：

```
纯文本回复：
  start → textStart → textDelta → ... → textDone → done

带思考（reasoning）：
  start → thinkingStart → thinkingDelta → ... → thinkingDone
        → textStart → textDelta → ... → textDone → done

带工具调用：
  start → thinkingDelta... → textDelta...
        → （循环：ask? → EventAsk → EventToolUseDelta → EventToolUseDone）
        → done
```

| 事件 | 携带数据 | 说明 |
|---|---|---|
| start | 无 | 开始生成 |
| textStart | 无 | 文本块开始 |
| textDelta | Delta string | 增量文本 |
| textDone | 无 | 文本块结束 |
| thinkingStart | 无 | 思考开始 |
| thinkingDelta | Delta string | 增量思考 |
| thinkingDone | 无 | 思考结束 |
| toolUseStart | ToolUse | 工具调用开始 |
| toolUseDelta | ToolUse | 工具调用（获批后发出，Result 待填） |
| toolUseDone | ToolUse（Result 已填） | 工具执行完成 |
| ask | ToolUse + AskReason + AskReply | 需用户确认，调用方通过 AskReply 回传结果 |
| done | Usage TokenUsage | 全部完成，channel 关闭 |
| error | Err error | 出错，channel 关闭 |

text 和 thinking 共用 Event.Delta 字段，由 Type 区分。

## 行为合约

### 循环控制

- runLoop 是纯自由函数，不绑定方法，state 由调用方持有和传入
- LLM 返回零个 tool_use 时退出循环，不做 maxTurns 硬停止
- ctx 取消时立即返回 ctx.Err()

### 上下文裁剪委托

- 每轮 LLM 调用前，将消息历史交给 `reef.Compact()` 处理
- helm 不感知裁剪策略（截断 / 摘要），reef 为 nil 则跳过
- 裁剪失败不阻塞，降级使用原始消息

### system prompt 组装

- helm 主动向各模块索取上下文片段（工具列表、Skill 指令、用户偏好），自行拼接
- 会话级缓存：同一会话中 system prompt 和工具列表只构建一次，跨轮次复用

### 流式输出

- 通过 `<-chan Event` 推送事件，调用方统一消费（CLI 输出终端，Gateway 转发客户端）
- ToolUseDelta 延后到工具获批后才发出——避免 UI 在用户确认前泄露工具调用信息
- Ask 确认通过 `EventAsk` 事件驱动：helm 发事件并阻塞等调用方通过 `AskReply` channel 回传，消费者串行处理确保 UI 不交错
- done 事件后 channel 关闭

### 错误处理

- sail.Chat 返回的错误直接透传给调用方，不做重试或分类恢复
- 工具执行部分失败：失败的 ToolResult.Status="error"，成功的正常注入，LLM 自行决定是否重试

### tool_calls 协议兼容

- assistant message 有工具调用时携带 `tool_calls` 元数据（ID + Name + Arguments）
- tool message 携带 `tool_call_id` 关联到对应的 assistant tool_call
- 满足 DeepSeek 等严格遵循 OpenAI 协议的厂商要求

## 消费接口

| 模块 | 关键方法 |
|---|---|
| deck | `List()`, `Lookup(name)`, `Tool.Execute(ctx, params)` |
| reef | `Compact(ctx, messages, tokenLimit)` |
| sail | `Chat(ctx, modelName, req) → <-chan Event`, `Window(modelName)` |
| brig | `Evaluate(tu) → (Verdict, reason)`, `Approve(tu)` |
| knot | `GetRawParam(tu)` |
| vault | （待建） |
| crew | （待建） |
