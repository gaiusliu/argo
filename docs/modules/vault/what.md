# vault 行为规约

## 概述

vault 是 Argo 的统一存储层，实现 helm 定义的 Memory 接口。两层职责：

| 层 | 存什么 | 生命周期 | 写入者 |
|------|------|------|------|
| 会话级 | transcript.jsonl（对话原文） | 单次会话 | helm 每轮结束后 |
| 框架级 | profile.md + AGENTS.md | 跨会话 | profile.md 由 LLM 维护（5K 预算）；AGENTS.md 由用户手写（永不淘汰） |

支持热插拔后端，默认文件系统实现。

## 需求

### 需求：对话记录持久化

vault SHALL 在每轮对话结束后将本轮产生的所有 record 同步追加写入 transcript.jsonl。每条 record 含 `type` 字段标识类型，共六种：user / llm / tool / system / compact / profile。详见 [transcript.md](transcript.md)。

#### 场景：每轮对话后落盘

- 给定：helm 完成一轮对话，产生了 1 条 llm record + 1 条 tool record
- 当：helm 调用 vault.Append(messages)
- 则：transcript.jsonl 追加 2 行，不修改已有内容，不阻塞主循环

### 需求：从 transcript 重建 messages

vault SHALL 提供 Load 方法读取 transcript.jsonl，过滤出 user / llm / tool 三种 type 的 record，映射回 []Message，供 Gateway 模式下每轮请求时重建 LLM 上下文。

#### 场景：Gateway 模式重建上下文

- 给定：Gateway 收到新一轮请求，transcript.jsonl 中有 50 条 record，其中 40 条为 user/llm/tool
- 当：helm 调用 vault.Load()
- 则：返回的 []Message 包含 40 条，system / compact / profile record 被过滤掉

### 需求：跨会话用户画像

vault SHALL 提供 profile.md 存储用户画像和偏好。由 LLM 通过工具调用 `vault.Store()` 写入。5,000 字符预算——新内容进入时 LLM 自行合并或丢弃最不重要的旧条目。

#### 场景：LLM 发现用户偏好

- 给定：用户在对话中说"以后所有回复用中文"
- 当：LLM 调用 vault.Store(ctx, "用户要求所有回复使用中文")
- 则：profile.md 追加一条，写入前 LLM 检查预算，超限则合并旧条目

### 需求：跨会话硬性约束

vault SHALL 加载 AGENTS.md 存储硬性约束和规则。用户直接编辑，LLM 只读，永不淘汰。vault.Store() 不写 AGENTS.md。

#### 场景：用户设定硬性约束

- 给定：用户在 AGENTS.md 中写入"所有 Go 测试必须连真实数据库"
- 当：会话开始时 helm 调用 vault.Recall()
- 则：此规则出现在返回的 RecallResult.Agents 中，注入 system prompt

### 需求：会话开始时注入框架级记忆

vault SHALL 在会话初始化时返回 profile.md + AGENTS.md 的全文，供 helm 注入 system prompt。

#### 场景：新会话继承用户偏好

- 给定：上会话中 LLM 写入了"用户偏好 Go 方案，10 年经验"
- 当：新会话启动，helm 调用 vault.Recall()
- 则：返回的 RecallResult.Profile 包含此偏好，注入 system prompt

### 需求：后端切换

vault SHALL 支持通过 Agent 配置切换后端实现。切换后新记忆写入新后端，旧后端数据保留在原地。

#### 场景：从默认实现切换到自定义后端

- 给定：Agent 配置中 memory.backend = "my-custom"
- 当：Argo 启动
- 则：vault 通过包级 `Load` 函数加载 my-custom 实现，后续 Store/Recall 路由到新后端

## 边界情况

- transcript.jsonl 写入失败：不阻塞主流程，仅记日志
- profile.md / AGENTS.md 不存在：视为空，不报错
- LLM 重复写入相同的偏好：vault 透传，LLM 自行去重
- 自定义后端加载失败：回退到默认 FileVault
- 并发写入 profile.md：Go 文件锁保护，写时独占
