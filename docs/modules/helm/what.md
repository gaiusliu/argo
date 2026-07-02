# helm 行为规约

## 概述

helm 是 Argo 的核心 Agent 循环。它是一个无状态的自由函数，接收一次用户输入，驱动 "LLM 调用 → 工具执行 → 结果注入 → 循环" 的完整链路，返回最终回复。

helm 不持有会话状态——消息历史、回合计数等由调用方（CLI / Gateway）维护并传入。每轮 LLM 调用前，helm 向各模块主动索取所需信息（工具列表、记忆、Skill 指令），并委托 reef 做上下文裁剪。

---

## 需求

### 需求：执行单次用户请求

helm SHALL 接收一次用户输入，通过 ReAct 循环处理，返回最终回复。

#### 场景：无工具调用的简单问答

- 给定：用户输入 "Go 中 interface 和 struct 的区别是什么"，且 deck 注册了 Read、Bash 等工具
- 当：runLoop 被调用
- 则：LLM 返回无 tool_calls 的文本回复，runLoop 返回该回复并标记 Done=true

#### 场景：需要工具调用的复杂任务

- 给定：用户输入 "帮我查一下当前目录的 Go 文件里有没有用到 context 包"，且 deck 提供了 Grep 工具
- 当：runLoop 被调用
- 则：LLM 返回 Grep 的 tool_call，helm 委托 deck 执行，结果注入消息历史，LLM 再次被调用返回最终回复

#### 场景：多轮工具调用

- 给定：用户输入 "创建一个 HTTP 服务器并测试它能正常启动"
- 当：runLoop 被调用
- 则：LLM 可能先 Write 文件 → 结果注入 → 再 Bash 编译 → 结果注入 → 再 Bash 启动测试 → 最终回复。每次工具调用后循环继续，直到 LLM 输出无 tool_calls 的最终回复

## 需求：上下文裁剪委托

helm SHALL 在每轮 LLM 调用前，将消息历史交给 reef 做裁剪，确保不超过模型 context window 上限。

#### 场景：消息历史接近窗口上限

- 给定：消息历史的 token 数接近 context window 上限
- 当：helm 准备调用 LLM
- 则：helm 调用 reef.Compact() 裁剪消息历史，用裁剪结果调用 LLM。helm 不感知裁剪策略（截断/摘要）的具体实现。

#### 场景：消息历史未达上限

- 给定：消息历史的 token 数远低于 context window 上限
- 当：helm 准备调用 LLM
- 则：reef 返回原消息列表不变，继续正常流程

---

## 需求：system prompt 主动组装

helm SHALL 在每轮 LLM 调用前，向各模块索取上下文片段，自行拼接为完整 system prompt。

#### 场景：组装 system prompt

- 给定：deck 注册了 5 个工具，talents 加载了 learn Skill，vault 有用户偏好记录
- 当：helm 准备调用 LLM
- 则：system prompt = 环境信息 + deck 的工具列表 + talents 的活跃 Skill 指令 + vault 的用户偏好 + reef 的裁剪提示（如有）

---

## 需求：流式输出

helm SHALL 通过事件 channel 向外推送 LLM 的生成增量，供调用方实时消费（CLI 输出到终端，Gateway 转发到客户端）。

#### 场景：流式推送文本

- 给定：LLM 正在生成文本回复
- 当：LLM 逐 token 返回
- 则：helm 通过 channel 推送 EventTextDelta{Content: token}

#### 场景：流式推送工具调用信息

- 给定：LLM 返回 tool_use
- 当：tool_use 被解析
- 则：helm 推送 Event{Type:"tool_use", ToolUse:{Result:nil}} 表示即将执行，deck 执行完成后再次推送同 ID 的 Event{Type:"tool_use", ToolUse:{Result:...}} 表示执行完成。调用方通过同一 ID 关联两次事件

#### 场景：流式结束

- 给定：LLM 生成完毕
- 当：最终回复确定
- 则：helm 推送 EventDone{Response, TokenUsage} 并关闭 channel

---

## 边界情况

- ctx 取消：上下文被取消时，runLoop 立即返回 ctx.Err()
- LLM 返回零个 tool_call 和空文本：视为异常，返回错误
- deck.ExecuteBatch 部分工具失败：失败的返回 ToolResult.Status="error"，成功的正常注入。LLM 看到错误自行决定是否重试
- sail.Chat 返回错误：直接返回错误，不做内部重试。重试逻辑属于 sail 的实现细节
