# vault 行为规约

## 概述

vault 是 Argo 的统一存储层，实现 helm 定义的 Memory 接口。两层职责：

| 层 | 存什么 | 生命周期 | 写入者 |
|------|------|------|------|
| 会话级 | transcript.jsonl（对话原文） | 单次会话 | helm 每轮结束后 |
| 框架级 | profile.md + rules.md | 跨会话，预算淘汰 | LLM 工具调用 + 用户手写 |

支持热插拔后端，默认文件系统实现。

## 需求

### 需求：对话记录持久化

vault SHALL 在每轮对话结束后将新增消息同步追加写入 transcript.jsonl，格式为 JSONL（4 字段：role / content / tool_calls / tool_call_id / timestamp）。

#### 场景：每轮对话后落盘

- 给定：helm 完成一轮对话，产生了 2 条新消息（assistant + tool_result）
- 当：helm 调用 vault.Append(newMessages)
- 则：transcript.jsonl 追加 2 行，不修改已有内容，不阻塞主循环

### 需求：跨会话用户画像

vault SHALL 提供 profile.md 存储用户画像和偏好。由 LLM 通过工具调用 `vault.Store()` 写入。5,000 字符预算——新内容进入时 LLM 自行合并或丢弃最不重要的旧条目。

#### 场景：LLM 发现用户偏好

- 给定：用户在对话中说"以后所有回复用中文"
- 当：LLM 调用 vault.Store({type:"profile", content:"用户要求所有回复使用中文", source:"llm"})
- 则：profile.md 追加一条，写入前 LLM 检查预算，超限则合并旧条目

### 需求：跨会话规则管理

vault SHALL 提供 rules.md 存储硬性约束和规则。用户可手动编辑，LLM 也可通过工具调用追加。用户手写内容永不淘汰。

#### 场景：用户设定硬性约束

- 给定：用户在 rules.md 中写入"所有 Go 测试必须连真实数据库"
- 当：会话开始时 helm 调用 vault.Recall()
- 则：此规则出现在返回的 RecallResult.Rules 中，注入 system prompt

### 需求：会话开始时注入框架级记忆

vault SHALL 在会话初始化时返回 profile.md + rules.md 的全文，供 helm 注入 system prompt。

#### 场景：新会话继承用户偏好

- 给定：上会话中 LLM 写入了"用户偏好 Go 方案，10 年经验"
- 当：新会话启动，helm 调用 vault.Recall()
- 则：返回的 RecallResult.Profile 包含此偏好，注入 system prompt

### 需求：后端切换

vault SHALL 支持通过 Agent 配置切换后端实现。切换后新记忆写入新后端，旧后端数据保留在原地。

#### 场景：从默认实现切换到自定义后端

- 给定：Agent 配置中 memory.backend = "my-custom"
- 当：Argo 启动
- 则：vault 通过 BackendRegistry 加载 my-custom 实现，后续 Store/Recall 路由到新后端

## 边界情况

- transcript.jsonl 写入失败：不阻塞主流程，仅记日志
- profile.md / rules.md 不存在：视为空，不报错
- LLM 重复写入相同的偏好：vault 透传，LLM 自行去重
- 自定义后端加载失败：回退到默认 FileVault
- 并发写入 profile.md：Go 文件锁保护，写时独占
