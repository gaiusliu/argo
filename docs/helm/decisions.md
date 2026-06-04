# helm 技术决策记录

---

## DEC-H01：runLoop 为无状态自由函数

**日期**：2026-06-04
**状态**：已决定

**决策**：`runLoop` 不绑定对象方法，state（消息历史、回合计数）由调用方（CLI/Gateway）持有并传入传出。

**原因**：Argo 需要同时支持 CLI 和 Gateway 两种产品形态。CLI 需要内存中的交互状态，Gateway 需要跨请求持久化。helm 自己不持有状态，调用方决定状态的存储方式，最解耦。

**参考**：AgentCore 和 pi 都采用自由函数模式。Hermes Agent 的 `AIAgent` 类（804KB 单体）是反面教材。

---

## DEC-H02：流式输出一步到位

**日期**：2026-06-04
**状态**：已决定

**决策**：MVP 直接做流式输出。`runLoop` 返回 `<-chan Event`，事件类型包括 `text_delta`、`tool_call`、`tool_result`、`done`。调用方（CLI 输出到终端，Gateway 转发到客户端）统一消费同一 channel。

**原因**：流式输出在 Go 中实现成本极低（带缓冲的 channel + goroutine），不需要额外依赖。后期从完整回复改流式需要重构调用方接口，一步到位成本更低。

---

## DEC-H03：system prompt 由 helm 主动组装

**日期**：2026-06-04
**状态**：已决定

**决策**：helm 扮演"CEO"角色——每轮 LLM 调用前主动向 deck（工具列表）、talents（Skill 指令）、vault（用户偏好）、reef（裁剪提示）索取信息，自行拼接为完整 system prompt。

**原因**：各模块的职责是"提供特定领域的上下文片段"，不负责"知道 system prompt 的最终结构"。helm 是唯一知道所有片段如何排列的模块。

**拒绝**：talents 作为唯一 system prompt 来源——会导致 Skill 需要知道 deck 的工具描述格式、vault 的记忆格式，模块边界崩溃。

---

## DEC-H04：不做 maxTurns 硬停止

**日期**：2026-06-04
**状态**：已决定

**决策**：循环内部不做 maxTurns 限制。纯 `for` 无终止条件，LLM 自己决定何时停（无 tool_calls 即停）。

**原因**：

1. 成熟产品全都不强制终止——Claude-Code 的 maxTurns 是可选参数（不传则永不触发），OpenCode 的 maxSteps 默认 Infinity，pi 是纯 `while(true)`
2. LLM 陷入死循环（反复调同一个工具、永远不输出最终回复）是极罕见异常，ctx 超时或用户 Ctrl+C 即可处理
3. 强制终止反而可能打断正在正确执行的长任务

---

## DEC-H05：helm 不做错误分类恢复

**日期**：2026-06-04
**状态**：已决定

**决策**：sail.Chat 返回的错误直接透传给调用方，helm 不做重试决策。重试和 fallback 是 sail 的实现细节。

**原因**：OpenClaw 的 ~1100 行错误分类（rate_limit→换 Key，auth_error→刷新 Token，overload→退避）是成熟产品的做法。MVP 只透传，后续按需引入。

---

## DEC-H06：reef 在每轮 LLM 调用前钩入

**日期**：2026-06-04
**状态**：已决定

**决策**：每轮 LLM 调用前，helm 将消息历史交给 `deps.Reef.Compact()`，用裁剪后的结果调用 LLM。reef 为 nil 则跳过（无裁剪）。

**原因**：helm 不感知上下文是否溢出——那是 reef 的判断逻辑。helm 只管把消息传进去，reef 决定剪不剪。
