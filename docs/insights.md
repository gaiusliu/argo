# 设计洞见

> 来自外部项目的设计思想参考，不是 Argo 的具体方案。仅记录"某个项目怎么设计的、为什么好、Argo 可以借鉴什么"。

---

## Trae：确定性工具约束概率性模型

**来源**：Trae Agent（字节跳动）

**核心哲学**：LLM 输出是不可靠的（概率性的），工具的校验逻辑是绝对可靠的（确定性的）。桥接方式——LLM 只负责提供内容锚点，工具负责精确验证。不信任 LLM 生成的行号、完整文件内容、diff。

**在 Argo 内部工具实现中的应用**：

```
str_replace 风格校验：
  工具收到 (old_str, new_str) → 工具自己搜索 old_str 在文件中出现几次
    ├── count == 0 → LLM 幻觉，返回 "未找到匹配文本"
    ├── count == 1 → 精确替换，成功
    └── count > 1  → LLM 给的上下文不足，返回 "找到 X 处匹配，请给出更多上下文"

通用原则：
  Read 工具：文件不存在、行号越界、权限不足 → 不同的精确错误消息
  Write 工具：目标目录不存在、权限不足、磁盘满 → 不同的精确错误消息
```

所有内部工具的错误消息应区分具体失败原因，让 LLM 能自我修正而非反复重试。

---

## reef 压缩质量衰减：多轮压缩后 Agent 变笨的问题

**来源**：11 个项目的调研 + 大量用户反馈

**核心洞见**：多次压缩后 Agent 确实会变笨/健忘。根源三层——信息不可逆丢失（错误堆栈、文件路径、原始数据变成模糊摘要）、语义漂移（每次摘要都是一次投影，三轮后失真）、上下文腐败（压缩模型可能误读约束甚至反转语义）。

**各项目的应对**：

| 机制 | 项目 | 要点 |
|------|------|------|
| P0 永不压缩 | Qoder | 任务目标、钉住文件硬保 |
| 增量更新 > 全文重摘要 | OpenCode/pi/Hermes | 在旧摘要基础上追加新内容，不重写 |
| Active Task 逐字引用 | pi/Hermes | "THE SINGLE MOST IMPORTANT FIELD" |
| SUMMARY_PREFIX 防混淆 | Hermes | `[CONTEXT COMPACTION -- REFERENCE ONLY]` |
| 反抖动断路器 | Claude-Code/Hermes | 连续压缩无效则停止 |
| 子 Agent 上下文隔离 | Eino DeepAgents | 子 Agent 不污染主 Agent 上下文 |
| 完整转录备份 | Claude-Code #26771 提议 | 保存完整原文供 LLM 按需回查 |

**用户真实反馈**：
- OpenCode #3099: "explicit rule prohibiting commit+push works fine until compacted... rules are lost"
- OpenCode #1483: 用户说"立即发布无需确认"，压缩后模型反着干
- Claude-Code #26771: 原始 DOM 标记被替换为 "User provided 8,200 characters of DOM markup"
- Codex CLI #4106: "after compaction, the agent loses essential parts... sometimes even the exact objective"

---

## reef 演进路线：多维度索引 + 多窗口 + 三级缓存

**来源**：argo 讨论中提出的创新方案

**核心思路**：不修补"一个窗口"，而是换锅——旧窗口内容按维度分类归档到文件系统，新窗口干净开始，LLM 按需渐进式检索旧窗口内容。

**演进三步走**：

1. **Phase 2: 多维度索引** —— LLM 压缩时按主题维度（项目/偏好/决策/调研）拆分对话，每个维度独立的摘要文件 + `_index.md` 目录
2. **Phase 3: 多窗口 + 渐进式检索** —— 压缩触发时创建新 context window，旧窗口归档。LLM 通过 reef_retrieve 工具像 SKILL 渐进式披露一样按需查阅
3. **Phase 4: 三级缓存淘汰** —— P0 硬保 / L1 LRU 热缓存 / L2 冷合并 / L3 可丢弃，淘汰时更新索引标记而非静默删除

**设计思想参考**：pi 的树状 JSONL（追加原文）+ Qoder 的 P0-P3（分级保留）+ CodeBuddy 的四层分类（按变化频率分）+ 数据库缓冲池管理（热/冷/淘汰三级）

---

## pi 树形会话分叉：验证有效但场景受限

**来源**：pi（earendil-works/pi-mono）的 JSONL `id`/`parentId` 树状会话模型

**核心设计**：会话不是线性数组，而是一棵树。每条 JSONL 条目通过 `parentId` 链接到父节点。用户通过 `/tree` 命令跳转到任意历史节点继续对话，新消息追加在旧节点下形成新分支。所有分支共存于同一个 `.jsonl` 文件中，不做拷贝。

**价值验证**（有用户反馈背书）：
- "支持 session branching，形成对话树来探索不同方案" —— ewaldbenes.com 独立评测，结论"我再也不会回到厂商特定的 CLI Agent 了"
- "Sessions as Trees, Code as Clay" —— qmx.me 博客整篇论证树形会话
- go-code 项目（Codex CLI 替代）将 pi 的会话分叉列为参考 feature request

**支撑的编程工作流**：冲刺-回归（spike-and-return）、角色分离（分析/实现分支并存）、并行假设调试（同 bug 多方案并行探索）。

## OpenAI 兼容 API 的 SSE 流式 tool call 格式

**来源**：对 DeepSeek API 的三次实际调用测试（2026-07-05）

**核心发现**：

1. **`function.arguments` 是逐字符增量片段，不是完整 JSON** — 一次 `{"cmd": "ls -la /tmp"}`（21 字符 JSON）被拆成了 14 个 SSE chunk 逐字推送。每个 chunk 的 `arguments` 只有 1-3 个字符。

2. **首块含 id + name + 空 arguments** — tool call 的第一个 chunk 同时携带 `id`、`function.name` 和空的 `function.arguments`。后续 chunk 只含 `function.arguments` 片段，不含 id 和 name。必须靠 `index` 维持关联。

3. **多个 tool call 可以在同一个 chunk 中同时出现** — 一个 SSE chunk 的 `delta.tool_calls` 数组可以包含多个不同 index 的 tool call。

4. **参数字段名由工具 schema 完全决定** — 模型生成的 `arguments` JSON 中的键名严格遵循 API 请求中 `tools[x].function.parameters.properties` 的定义。定义为 `"cmd"` 则模型填 `"cmd"`，定义为 `"path"` 则填 `"path"`。

**实际数据示例**：

```
第一个 tool call (bash):
  Chunk 1: index=0, id=call_00_xxx, name="bash",   arguments=""       ← 首块
  Chunk 2: index=0,                                 arguments="{"      ← 逐字增量
  Chunk 3: index=0,                                 arguments="""
  Chunk 4: index=0,                                 arguments="cmd"
  ...
  Chunk 14: index=0,                                arguments="}"
  拼接后: {"cmd": "ls -la /tmp"}

同时出两个 tool call:
  Chunk 1: index=0, id=call_00_xxx, name="web_fetch", arguments=""
           index=1, id=call_01_xxx, name="read",     arguments=""      ← 同一 chunk
```

**对 Argo sail adapter 的设计影响**：

- 不能拿单个 chunk 的 `arguments` 直接当参数用，必须在 `streamState` 中按 index 累积
- `EventToolUseStart` 在首块发出（只有 id + name，给 UI 用），`EventToolUseDelta` 在 flush 时发出（累积完成的完整 Parameters）
- `[DONE]` 标记触发 flush，刚好是 data → 门控 → 执行的正确时序

---
通用 Agent（客服、教练、调研）没有"回到过去节点重试另一条路"的需求——对话分叉是代码探索的刚需，不是通用对话的刚需。保留此洞见，等 Argo 的 coding 场景用户需求出现时再引入。
