# helm

## 概述

helm 是 Argo 的 Agent 核心循环。它是一个无状态的自由函数 `runLoop`，接收一次用户输入，驱动 "LLM 调用 → 工具执行 → 结果注入 → 循环" 的完整链路，返回最终回复。helm 不持有会话状态——消息历史、回合计数等由调用方（CLI / Gateway）维护并传入。各模块通过包级公开函数 + 单例模式提供能力，helm 直接调用（如 `deck.List()`、`sail.Chat()`），不通过依赖注入。

## 核心概念

| 概念 | 说明 |
|---|---|
| runLoop | 无状态自由函数，执行 ReAct 循环，返回 `<-chan Event` 供调用方流式消费 |
| TurnState | 会话状态（消息历史、回合计数、当前模型名），由调用方持有并传入传出 |
| Event | 流式事件，12 种类型（见下方"流式事件类型"） |
| ReAct 循环 | 思考（LLM）→ 行动（工具执行）→ 观察（结果注入）的闭环，LLM 无 tool_calls 即退出 |

## 流程

### 单轮主循环

```
调用方传入 state（含首次 user message）
  │
  └─ RunLoop(ctx, &state)
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
       │    │      ├─ toolUseStart             → 收集待执行工具
       │    │      └─ done / error             → 最终回复或错误
       │    │
       │    ├─ 3. 无 tool_use？→ 退出循环
       │    │
       │    ├─ 4. deck.Lookup() + Tool.Execute() → 逐条执行，填充 Result
       │    │      └─ Event.tool_use（Result 已填）→ 转发给调用方
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
  start → textDelta → toolUseStart → toolUseDelta → ... → toolUseDone
        → done
        （helm 执行工具后发 toolUseStart → toolUseDone，Result 已填）
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
| toolUseStart | ToolUse（Result=nil） | 工具调用开始（sail 发 = LLM 请求，helm 发 = 开始执行） |
| toolUseDelta | Delta string | 流式工具参数 |
| toolUseDone | ToolUse（Result 已填） | 工具执行完成 |
| done | DonePayload（Text + TokenUsage） | 全部完成，channel 关闭 |
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
- 同一 tool_use 的 ID 出现两次：Result=nil（待执行）→ Result 填充（执行完成）
- done 事件后 channel 关闭

### 错误处理

- sail.Chat 返回的错误直接透传给调用方，不做重试或分类恢复
- 工具执行部分失败：失败的 ToolResult.Status="error"，成功的正常注入，LLM 自行决定是否重试

## 消费接口

| 模块 | 关键方法 |
|---|---|
| deck | `List()`, `Lookup(name)`, `Tool.Execute(ctx, params)` |
| reef | `Compact(ctx, messages, tokenLimit)` |
| sail | `Chat(ctx, modelName, req) → <-chan Event`, `Window(modelName)` |
| vault | （待建） |
| crew | （待建） |
