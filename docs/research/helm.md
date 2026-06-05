# Agent 核心循环（helm）设计调研

> 调研对象：Claude-Code、OpenCode、Hermes Agent、CrewAI、OpenClaw、Nanoclaw、pi

---

## 1. 全局共识：所有项目都遵循 "ReAct 循环"

七个项目无一例外采用 **思考-行动-观察（ReAct）** 循环模式：

```
用户输入 → LLM调用(思考) → 工具调用(行动) → 结果注入(观察) → LLM再调用 → ... → 最终回复
```

差异在于：循环的退出条件、错误恢复策略、并行化程度、以及"轮次"的边界划分方式。

---

## 2. Claude-Code：生成器驱动的四阶段循环

**核心文件**：`src/query.ts`（~1800 行）

### 流程决策树

```
query() 入口
  │
  └── while(true) 主循环 ← 永不终止，直到满足退出条件
        │
        ├── 阶段1：上下文准备（多层压缩）
        │   ├── getMessagesAfterCompactBoundary()  裁剪已压缩区
        │   ├── applyToolResultBudget()            控制工具结果总长度
        │   ├── snipCompactIfNeeded()              尾部剪裁
        │   ├── microcompact()                     缓存感知微压缩
        │   ├── contextCollapse.applyCollapses()    折叠旧助理消息
        │   └── autocompact()                      自动摘要压缩
        │       └── 如果触发压缩 → yield summaries → 重置追踪 → 继续
        │
        ├── 阶段2：API调用
        │   ├── callModel() → Anthropic SDK stream()
        │   ├── 流式 yield 每个事件（文本增量/工具调用/错误）
        │   ├── 收集 assistantMessages + tool_use 块
        │   └── 错误处理：FallbackTriggeredError → 切换备用模型 → 重试
        │
        ├── 阶段3：终止检查（无工具调用时）
        │   ├── max_output_tokens 截断？→ 注入恢复消息 → continue
        │   ├── prompt_too_long？→ 上下文折叠排空 → 反应式压缩 → 重试
        │   ├── 停止钩子检查
        │   └── 否则 → 返回 'completed'
        │
        ├── 阶段4：工具执行
        │   ├── partitionToolCalls() → 并发安全组 + 顺序组
        │   ├── 并发安全工具 → Promise.all()（10 并发上限）
        │   └── 顺序工具 → for/of 逐个执行
        │
        └── 轮次切换 → state.messages 追加新消息 → 回到阶段1
```

### 核心逻辑伪代码

```
async function* query(params):
    state = { messages: params.messages, turnCount: 1, ... }

    while true:
        // 阶段1：上下文管理
        msgs = getMessagesAfterCompactBoundary(state.messages)
        msgs = snipCompactIfNeeded(msgs)
        msgs = await contextCollapse.applyCollapses(msgs)
        { compactionResult } = await autocompact(msgs)
        if compactionResult:
            yield compact_boundary
            msgs = buildPostCompactMessages(compactionResult)

        // 阶段2：API 调用
        toolUseBlocks = []
        for await (event of callModel(msgs)):
            yield event  // 流式传递
            collect tool_use blocks

        // 阶段3：终止条件
        if toolUseBlocks.length == 0:
            if needsRetry: continue
            return { reason: 'completed' }

        // 阶段4：工具执行
        { concurrent, sequential } = partitionToolCalls(toolUseBlocks)
        for result in runConcurrent(concurrent) + runSerial(sequential):
            yield result

        // 轮次推进
        state.messages = [...msgs, ...assistantMsgs, ...toolResults]
        state.turnCount++
```

### 设计思想

1. **不可变参数 / 可变状态**：外部参数（model、tools）在循环中永不改变，但内部 `state` 随轮次推进而变化
2. **分层压缩排序**：最便宜的压缩（删除缓存、剪裁尾部）优先，最贵的（LLM 摘要）最后
3. **"扣留" 恢复模式**：遇到 prompt_too_long 或 max_tokens 截断时，不立刻 yield 错误，而是扣留消息直到恢复成功或彻底失败——防止 SDK 消费者过早终止
4. **独立计数器的多种重试策略**：overflow_compaction、timeout_compaction、auth_profile_rotation 各有独立计数器，互不扣减

---

## 3. OpenCode：Effect-TS 函数式 + 事件驱动状态机

**核心文件**：`packages/opencode/src/session/prompt.ts`（`SessionPrompt.loop()`）、`processor.ts`

### 流程决策树

```
SessionPrompt.prompt()
  │
  └── SessionPrompt.loop()
        │
        └── runLoop(sessionID)
              │
              ├── 加载消息历史（跳过已压缩）
              ├── 找到 lastUser、lastAssistant、tasks
              │
              ├── 检查 finish reason：
              │   ├── finish != "tool-calls" 且无待处理工具 → BREAK
              │   └── finish == "tool-calls" → 继续
              │
              ├── 检查 task 类型：
              │   ├── subtask → handleSubtask()
              │   ├── compaction → compaction.process()
              │   └── overflow → compaction.create()
              │
              ├── resolveTools() → builtin + MCP
              ├── build system prompt → env + instructions + skills
              │
              └── SessionProcessor.create()
                    ├── snapshot.track()（步骤前快照）
                    ├── LLM.stream() → vercel/ai-sdk streamText()
                    └── handleEvent() 事件驱动状态机
                          ├── "start" → status=busy
                          ├── "reasoning-delta" → 更新推理文本
                          ├── "tool-input-delta" → 构建工具调用块
                          ├── "tool-call" → 检测 doom loop → 执行工具
                          ├── "tool-result" → completeToolCall()
                          ├── "finish-step" → snapshot diff + overflow 检查
                          └── Result: "compact" | "stop" | "continue"
```

### 设计思想

1. **Effect-TS 依赖注入**：每个服务是 `Context.Service`，依赖以 `Layer` 注入——测试时可直接替换
2. **单例互斥**：`SessionRunState` 阻止同会话的并发流式调用
3. **压缩是第一公民**：压缩不是异常处理，而是循环中正常的 task 类型——overflow 触发 → 创建 compaction task → 下次迭代执行
4. **竞态保护**：`SessionRunState` 支持 "shell" 模式（终端与会话并行运行），状态机 `Idle → Shell → ShellThenRun → Idle`

---

## 4. Hermes Agent：ReAct + 原生工具调用双通道

**核心文件**：`run_agent.py`（804KB 巨型文件，`AIAgent.run_conversation()`）

### 流程决策树

```
run_conversation(user_message, system_message, history)
  │
  ├── 阶段1：准备
  │   ├── 构建消息列表 = history + new_user_msg
  │   ├── _build_system_prompt()（缓存复用）
  │   ├── 预飞行压缩检查（threshold_tokens）
  │   ├── 记忆预取：_memory_manager.prefetch_all()
  │   └── 中断/转向信号设置
  │
  ├── 阶段2：工具调用循环
  │   └── while api_call_count < max_iterations AND budget > 0:
  │         │
  │         ├── 2a. 构建 API 消息
  │         │   ├── 注入记忆上下文 → user message
  │         │   ├── 注入 prefills + ephemeral messages
  │         │   ├── Anthropic prompt caching markers
  │         │   ├── 清理孤儿 tool_result、修复 message 交替
  │         │   └── 归一化空白 + tool-call JSON（KV cache 前缀匹配）
  │         │
  │         ├── 2b. API 调用（重试循环）
  │         │   ├── transport.build_kwargs() + 流式调用
  │         │   └── 错误：分类 → 回退激活 → jittered backoff → 重试
  │         │
  │         ├── 2c. 处理 finish_reason
  │         │   ├── "stop" → 构建 assistant 消息 → EXIT
  │         │   ├── "length" → 截断？注入继续 user_msg → continue
  │         │   └── "tool_calls" → 执行工具 → continue
  │         │
  │         └── 2d. 工具执行
  │               ├── _should_parallelize_tool_batch()? → concurrent | sequential
  │               ├── Agent-level 工具（todo/memory/delegate_task）→ AIAgent 直接处理
  │               └── registry.dispatch() → 调用 handler
  │
  └── 阶段3：后处理
        ├── 记忆同步：_memory_manager.sync_all()
        ├── 技能催促检查（_skill_nudge_interval）
        └── 持久化到会话 DB
```

### 设计思想

1. **预算共享**：`IterationBudget` 对象在父 agent 和所有子 agent 之间共享，防止无限委派
2. **流式优先**：即使没有流式消费者，也优先流式——用于 90s 滞流检测健康检查
3. **双通道执行**：文本 ReAct（解析 "Action:" 文本）+ 原生函数调用（结构化 tool_calls），兼容新老模型
4. **KV cache 前缀匹配**：归一化 tool-call JSON 格式使跨轮次的 Anthropic prompt cache 命中率最大化

---

## 5. CrewAI：三层架构 (Crew → Agent → Executor)

**核心文件**：`agent/core.py`、`agents/crew_agent_executor.py`、`crew.py`

### 流程决策树

```
Crew.kickoff(inputs)
  │
  ├── Process.sequential?
  │   └── _run_sequential_process()
  │         └── _execute_tasks(tasks)  // 管道-过滤器模式
  │               ├── 对于每个 task：
  │               ├── ConditionalTask → 评估条件 → 跳过?
  │               ├── async_execution? → Future 并行
  │               └── 否则同步：
  │                     ├── agent.execute_task(task, context, tools)
  │                     │     ├── _prepare_task_execution()  记忆召回+提示构建
  │                     │     ├── handle_knowledge_retrieval()
  │                     │     └── executor.invoke()
  │                     │
  │                     └── _process_task_result() / _store_execution_log()
  │
  └── Process.hierarchical?
        └── _run_hierarchical_process()
              └── ManagerAgent 将任务委派给下属 Agent
```

内层 `AgentExecutor._invoke_loop()`：

```
while not isinstance(answer, AgentFinish):
    │
    ├── 1. enforce_rpm_limit()       // 速率限制
    ├── 2. get_llm_response()        // LLM 调用
    ├── 3. process_llm_response()    // 解析为 AgentAction 或 AgentFinish
    ├── 4. IF AgentAction:
    │     ├── _tool_calling()        // 解析工具调用（文本或结构化）
    │     ├── ToolUsage.use()        // 执行工具
    │     ├── check result_as_answer // 工具结果是否=最终答案？
    │     └── _handle_agent_action()
    ├── 5. IF AgentFinish: BREAK
    └── 6. 异常处理:
          ├── OutputParserError → handle_output_parser_exception()
          ├── ContextLengthExceeded → handle_context_length() [摘要/截断]
          └── 其他 → handle_unknown_error()
```

### 设计思想

1. **流程与 Agent 分离**：`Crew` 管理流程（串行/层级），`Agent` 管理行为（角色/目标），`Executor` 管理 LLM 交互
2. **层级流程**：创建特殊的 ManagerAgent，使用 `delegate_work` 和 `ask_question` 工具实现多级管理
3. **检查点恢复**：支持从检查点恢复，失败时从中断处继续
4. **双代码路径共存**：旧版 ReAct（解析 "Action:" 文本）和新版原生工具调用（结构化 tool_calls）同时存在

---

## 6. OpenClaw：自愈型 while(true) + 分层 Failover

**核心文件**：`src/agents/pi-embedded-runner/run.ts`（~3140 行）

### 流程决策树

```
runEmbeddedPiAgent()
  │
  ├── 阶段0：会话排队（同 session 串行，跨 session 受控并行）
  ├── 阶段1：模型解析（5 级回退链）
  ├── 阶段2：Auth Profile 排序 + 冷却状态检查
  ├── 阶段3：Harness 选择（pi / plugin / auto）
  ├── 阶段4：上下文预算分析
  │
  └── while(true) 主循环：
        │
        ├── 5a. 构建 attempt 参数（注入重试指令到 prompt）
        ├── 5b. runEmbeddedAttemptWithBackend()  LLM+工具
        ├── 5c. attempt 结果处理（~1100 行决策树）
        │     ├── preflightRecovery 已处理? → continue
        │     ├── idleTimeout 断路器? → 返回错误
        │     ├── timedOut AND 使用率>65%? → 压缩会话 → continue
        │     ├── contextOverflow?
        │     │   ├── 已压缩过? → retry prompt
        │     │   ├── 有压缩次数? → 压缩 → continue
        │     │   └── 截断工具结果? → 截断 → continue
        │     ├── prompt_error?
        │     │   ├── rate_limit → 换Key
        │     │   ├── auth_error → 刷新Token
        │     │   ├── overload → 退避+换Key
        │     │   └── role_ordering → 不可恢复
        │     └── "假成功"?
        │         ├── planning-only → 追加指令
        │         ├── reasoning-only → 追加指令
        │         └── empty-response → 追加指令
        │
        └── 5d. resolveRunFailoverDecision()
              → continue_normal / rotate_profile / fallback_model / surface_error
```

### 设计思想

1. **优先用便宜手段**：prompt 指令修正（零 API 成本）→ 截断工具结果 → 换 Key → 换 Provider
2. **每种失败独立管理**：overflowCompaction 最多 3 次、timeoutCompaction 最多 2 次、authProfileRotation 最多 N 次——互不扣减
3. **把失败原因翻译成 LLM 语言**：重试指令直接注入 prompt，告诉 LLM 上一次哪做错了

---

## 7. Nanoclaw：SQLite 双数据库 + 主动查询内同步轮询

**核心文件**：`container/agent-runner/src/poll-loop.ts`

### 流程决策树

```
runPollLoop(config)
  │
  └── while true:
        │
        ├── 1. getPendingMessages() ─ 从 inbound.db 读取
        ├── 2. [仅 accumulate?] → sleep(1s) → continue
        ├── 3. markProcessing(ids) ─ 写入 processing_ack
        ├── 4. /clear 命令处理
        ├── 5. 任务前脚本（调度模块钩子）
        ├── 6. formatMessagesWithCommands() ─ XML 格式化
        ├── 7. provider.query({ prompt, continuation })
        │     │
        │     └── 启动同步 follow-up 轮询（500ms 间隔）：
        │           ├── 读取新消息
        │           ├── 运行任务前脚本
        │           └── provider.push(prompt)  注入到活跃查询
        │
        ├── 8. processQuery() ─ 遍历 provider.events
        │     ├── 'init' → 存储 continuation（持久化到 DB）
        │     ├── 'result' → 解析 <message to="name"> XML
        │     ├── 'error' → 日志 / 重试决策
        │     └── 'activity' → touchHeartbeat()
        │
        └── 9. markCompleted(ids) → 回到 1
```

### 设计思想

1. **一切皆消息**：宿主和容器之间没有 IPC 通道，交互全靠 inbound/outbound 两个 SQLite 文件
2. **活跃查询内的同步轮询**：provider.query() 执行期间，500ms 间隔的轮询器通过 provider.push() 把新消息注入活跃会话——避免关闭并重新打开 SDK 子进程（每次 ~2 秒开销）
3. **每个 provider 独立 continuation**：SDK 会话 ID 按 provider 名称键值存储，切换 provider 无损
4. **"无响应"纠正**：Agent 的输出如果没有包裹在 `<message to="name">` 内，系统自动注入格式提醒

---

## 8. pi：事件驱动 + 双抽象层次

**核心文件**：`packages/agent/src/agent-loop.ts`、`harness/agent-harness.ts`

### 流程决策树

```
runLoop(context, newMessages, config)
  │
  └── while true:
        │
        └── while hasMoreToolCalls or pendingMessages:
              │
              ├── 注入 pending steering 消息
              ├── streamAssistantResponse()
              │     ├── config.transformContext()?  // 转换上下文
              │     ├── config.convertToLlm()       // AgentMessage → LLM Message
              │     └── streamFunction(model, context)
              │           → 事件流：start → text_delta → toolcall_delta → done
              │
              ├── 工具调用？
              │   ├── "sequential" 或工具标记为 sequential → 逐个执行
              │   └── 否则 → executeToolCallsParallel()
              │         ├── prepareToolCall() 验证+权限
              │         ├── tool.execute(id, params, signal, onUpdate)
              │         └── afterToolCall() 可能覆写结果
              │
              ├── shouldStopAfterTurn?() → 提前退出
              ├── prepareNextTurn?() → 替换 context/model/thinking 状态
              └── 检查 steering/follow-up 消息
```

### 设计思想

1. **事件驱动，非命令式**：循环不直接与 UI 或存储交互，订阅者监听 `AgentEvent` 更新 UI
2. **双抽象层次**：`Agent` 类（有状态，拥有消息队列）→ `AgentHarness`（添加会话持久化、钩子、工具注册、压缩）
3. **每个 LLM 调用动态解析 API Key**：因为 OAuth 令牌会过期
4. **AgentMessage 类型系统**：允许自定义消息类型（bashExecution、branchSummary），在到达 LLM 前才转换为标准 Message

---

## 9. 横向对比总结

| 维度 | Claude-Code | OpenCode | Hermes Agent | CrewAI | OpenClaw | Nanoclaw | pi |
|------|------------|----------|-------------|--------|----------|----------|-----|
| **循环模式** | AsyncGenerator | Effect-TS 状态机 | 经典 while | Flow 编排 | while(true) 自愈 | 轮询 + 同步注入 | 事件驱动 |
| **退出条件** | 无工具调用且满足停止条件 | finish != tool-calls | stop + budget 耗尽 | AgentFinish flag | while(true) 返回/错误 | 消息消费完毕 | shouldStopAfterTurn |
| **错误恢复** | 多层 fallback + 重试指令注入 | Effect.retry + 回退策略 | 分类恢复 + jittered backoff | max_retry_limit + 检查点 | 每种失败独立计数 | 心跳挂起检测 + 指数退避 | 流事件编码错误 |
| **并行工具** | 并发安全分区 (10上限) | AI SDK 并行 | 并行/顺序判断 | ThreadPoolExecutor x8 | 无显式并行 | N/A（单工具单次） | 并行组 + 顺序组 |
| **上下文压缩** | 5层（snip→micro→collapse→auto→reactive） | 压缩是第一公民 task | 阈值触发 + 辅助模型摘要 | 截断/摘要 | 阈值压缩 + 预飞行恢复 | PreCompact hook + 转录归档 | 增量结构化摘要 |
| **核心创新** | 扣留恢复、prompt 注入重试指令 | Effect-TS 依赖注入、竞态保护 | 预算共享、KV cache 前缀匹配 | 层级 Manager Agent、检查点恢复 | 独立计数器、便宜手段优先 | 双 SQLite、活跃查询内推送 | AgentMessage 类型系统 |


## 10. 对 Argo 的启示

1. **ReAct 循环是唯一正确答案**：7 个项目无一例外采用此模式，Argo 应当直接采用
2. **while(true) 优于 for(N)**：OpenClaw 和 Claude-Code 的不限次数循环 + 智能退出条件比硬编码最大轮次更灵活
3. **错误恢复应分层**：先 prompt 修正（零成本），再截断/换 Key，最后换 Provider
4. **工具执行需要分区**：读操作可并行，写操作应串行——Claude-Code 的分区逻辑值得借鉴
5. **压缩策略要由轻到重**：微压缩 → 缓存编辑 → 上下文折叠 → 摘要压缩的层级递进
6. **Nanoclaw 的"活跃查询内推送"**对于 Gateway 模式有直接参考价值——用户的后续消息通过 push() 注入正在运行的 Agent 会话，避免重复启动
