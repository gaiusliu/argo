# 记忆管理（memory / vault）设计调研

> 调研对象：Claude-Code、OpenCode、Hermes Agent、CrewAI、OpenClaw、Nanoclaw、pi

---

## 1. 全局共识

记忆管理在七个项目中呈现**两极分化**：

- **会话内记忆**（上下文压缩）：所有项目都有，是刚需
- **跨会话记忆**（长期记忆）：只有 Hermes Agent、OpenClaw、CrewAI 有完整实现
- 其余项目（Claude-Code、OpenCode、Nanoclaw、pi）依赖**文件系统**（MEMORY.md / CLAUDE.md）做"伪长期记忆"，LLM 自己 read 文件

---

## 2. Claude-Code：会话内双层记忆（会话记忆 + 5 级上下文压缩）

**核心文件**：`src/services/SessionMemory/`、`src/services/compact/`、`src/services/contextCollapse/`

### 会话记忆系统

```
会话启动 → initSessionMemory()

每个模型响应后（PostSampling Hook）：
  shouldExtractMemory(messages):
    ├── 门控检查：tengu_session_memory 功能
    ├── 初始化 token 阈值
    ├── 自上次提取以来的 token 和工具调用阈值
    └── 最后一个 assistant 轮次无工具调用？（自然断点）

  如果 true → runForkedAgent({
      promptMessages: [提取提示],
      canUseTool: createMemoryFileCanUseTool(memoryPath)
        // 仅允许 FileEdit 操作记忆文件！
    })
    // fork agent 读取现有记忆并写入新摘要
```

### 五级上下文压缩

```
每次 API 调用前：
  │
  ├── [1] Snip Compact ─── 截断最旧消息的尾部内容
  │                        保留消息结构，只剪裁内容
  │
  ├── [2] Microcompact ──── 缓存感知：删除已知的缓存工具结果
  │                        通过 cache_edits API 删除 token
  │
  ├── [3] Context Collapse ─ 将旧 assistant 消息折叠为摘要
  │                          投影视图：读取时投射，历史不变
  │
  └── [4] Autocompact ───── 上下文窗口剩余 < 13K tokens 时触发
         │                   用 fork agent + 结构化摘要模板
         │                   替换旧消息为 summaries + attachments
         │
         └── 失败（连续 >3 次）→ [5] Reactive Compact
               截断会话头部 → 重试
               最多 3 次 → 放弃 → 返回 blocking_limit
```

### 没有跨会话记忆

> **不确定**：代码中 `tengu_session_memory` 完全是会话内运行的。没有嵌入向量数据库或长期 RAG 系统。跨会话记忆可能通过 CLAUDE.md 项目指令隐式处理，但这不算是真正的"vault"。

### 设计思想

1. **分支式 Agent 压缩 = 隔离保证**：压缩不会污染主会话状态，fork agent 接收快照、生成摘要、然后死亡
2. **最便宜的手段优先**：删除缓存内容 → 截断 → 折叠 → 摘要，从不做比必需更贵的事
3. **断路器**：引用数据"1,279 个会话有 50+ 连续失败" → 超过 3 次连续失败激活断路器

---

## 3. OpenCode：压缩是第一公民 + 指令文件

**核心文件**：`session/compaction.ts`、`session/instruction.ts`、`session/overflow.ts`

### 溢出驱动的压缩

```
isOverflow(tokens, model) → tokens >= usable(context - reserved)

触发时：
  SessionCompaction.create(sessionID, auto=true)
    └── 创建带 compaction marker 的用户消息

下次循环迭代检测到 compaction task：
  SessionCompaction.process():
    │
    ├── 1. 选择保留的最近轮次
    │     budget = min(8K, max(2K, 0.25 * usable))
    │     保留 budget 内的最后 N 轮（默认 2 轮）
    │
    ├── 2. 构建压缩历史
    │     之前的压缩摘要被折叠
    │     旧轮次摘要化为模板：
    │       Goal | Constraints | Progress | Key Decisions |
    │       Next Steps | Critical Context | Relevant Files
    │
    ├── 3. REPLAY 机制
    │     压缩后重放最后一条用户消息的非媒体部分
    │     保证用户意图不丢失
    │
    └── 4. 执行压缩模型调用
          使用 "compaction" agent（只读、无工具）
          发送摘要历史 + 摘要 prompt
```

### 修剪策略

```
prune(sessionID):
  ├── 倒序扫描消息
  ├── 跳过前 2 轮
  ├── 停在第一个压缩摘要
  ├── PRUNE_PROTECT (40K) tokens 保留
  └── 剩余工具输出被 "压缩"（标记时间戳）
```

### 指令文件系统

```
Instruction.Service 向上扫描目录：
  AGENTS.md → CLAUDE.md → CONTEXT.md
  ~/.config/opencode/AGENTS.md + ~/.claude/CLAUDE.md
  URL 指令（可 HTTP 获取）

时间去重：每个 assistant 消息最多附加一次指令文件
共址：Read 工具加载文件时，自动附加上级目录的指令文件
```

### 设计思想

1. **压缩是任务，不是异常**：压缩是正常的 task 类型，在循环中与其他 task 一样处理
2. **REPLAY 保证意图不丢失**：压缩后重放用户的最后消息是保护用户体验的关键设计
3. **25% 预算分配给最近上下文**：默认保留 25% 上下文给最近轮次，确保最新对话完整

---

## 4. Hermes Agent：可插拔 MemoryProvider + 迭代压缩

**核心文件**：`agent/memory_manager.py`、`agent/context_compressor.py`、`agent/memory_provider.py`

### MemoryProvider 接口

```python
class MemoryProvider(ABC):
    name                    # "builtin", "honcho", "mem0", "holographic", ...
    system_prompt_block()   # 系统提示中注入的静态文本
    prefetch(query)         # 每轮前召回 → 注入到用户消息
    queue_prefetch(query)   # 下一轮的后台召回
    sync_turn(user, asst)   # 每轮后持久化
    get_tool_schemas()      # 暴露给模型的记忆工具
    handle_tool_call()      # 调度工具调用
    on_session_end()        # 会话结束时持久化
    on_pre_compress()       # 压缩前提取
```

### 可用后端

`plugins/memory/` 下有 8 个后端：
- **holographic**：全息缩减表示（HRR），相位编码，代数 bind/unbind/bundle 操作
- **honcho**：外部 REST API
- **mem0**：Mem0 云端记忆服务
- **hindsight**、**supermemory**、**retaindb**、**byterover**、**openviking**

### 上下文压缩策略

```
ContextCompressor.should_compress() → last_prompt_tokens >= 75% context

compress(messages):
  1. PRUNE 旧工具结果（便宜，无需 LLM）
  2. PROTECT head（前 ~3 条消息 = 系统 + 早期交换）
  3. FIND tail 边界（token 预算 ~20K）
  4. SUMMARIZE 中间轮次（结构化 LLM 提示）:
       - 使用辅助模型（便宜/快速）做摘要
       - 摘要格式：[CONTEXT COMPACTION - REFERENCE ONLY]
       - 模板：已解决问题、待处理事项、活跃任务
       - 缩放预算：压缩内容的 20%，上限 12K tokens
  5. 重压缩时：迭代 UPDATE 前次摘要
  6. CLEAN UP 孤儿 tool_call/tool_result 对
```

### 设计思想

1. **一次只能一个外部 Provider**：防止 schema 膨胀和冲突
2. **记忆注入到用户消息而非 system prompt**：保护 Anthropic prefix cache
3. **迭代摘要而非重建**：每次压缩更新前次摘要，不从头重建——信息跨压缩保留
4. **MEMORY.md 内置记忆与外部 Provider 分离**：两条路径独立运行

---

## 5. CrewAI：统一记忆 + 自适应深度召回

**核心文件**：`memory/unified_memory.py`、`memory/encoding_flow.py`、`memory/recall_flow.py`

### 统一记忆模型

```python
class Memory(BaseModel):
    llm: BaseLLM                          # 用于分析的 LLM
    storage: StorageBackend               # LanceDB 或 Qdrant
    embedder: Any                         # 向量嵌入
    recency_weight: float = 0.3           # 复合评分权重
    semantic_weight: float = 0.5
    importance_weight: float = 0.2
    confidence_threshold_high: float = 0.8
    confidence_threshold_low: float = 0.5
    exploration_budget: int = 1
```

### 编码管道（保存时）

```
Memory.remember(content)
  │
  └── EncodingFlow（基于 Flow 构建）:
        ├── batch_embed() ─── 批量嵌入
        ├── intra_batch_dedup() ─── 成对余弦相似度去重 (>0.98)
        ├── parallel_find_similar() ─── 每个项目并行搜索相似
        └── parallel_analyze() ─── 四组分类处理:
              ├── A: 有字段 + 无相似 → 快速插入 (0 LLM 调用)
              ├── B: 有字段 + 有相似 → 仅合并 (1 LLM 调用)
              ├── C: 缺字段 + 无相似 → 仅字段推断 (1 LLM 调用)
              └── D: 缺字段 + 有相似 → 字段推断+合并 (2 LLM 调用)
```

### 召回管道（读取时）

```
Memory.recall(query, depth="deep")
  │
  └── RecallFlow:
        ├── analyze_query_step():
        │     ├── 短查询 (<200字符) → 跳过 LLM，直接嵌入
        │     └── 长查询 → LLM 提炼子查询 + 范围 + 时间过滤器
        │
        ├── search_chunks():
        │     └── 并行搜索（子查询 × 范围）
        │
        ├── decide_depth() [路由器]:
        │     ├── 置信度 >= 0.8 → 合成（返回）
        │     ├── 置信度 < 0.5 AND 预算剩余 → explore_deeper
        │     ├── 复杂查询 AND 置信度 < 0.7 → explore_deeper
        │     └── 否则 → 合成
        │
        └── synthesize_results():
              └── 去重 + 复合评分 + 排名 + 附加证据差距
```

### 设计思想

1. **语义 + 新颖性 + 重要性复合评分**：不仅向量相似度，加上时间衰减（指数式，30 天半衰期）和显式重要性权重
2. **自适应深度召回**：置信度阈值引导快速（浅层）或彻底（深层）检索
3. **LLM 驱动的合并**：保存时检查新内容应替换、更新还是与现有记忆合并——不是盲目追加
4. **自举架构**：EncodingFlow 和 RecallFlow 都是用框架自己的 `Flow` 类构建的

---

## 6. OpenClaw：仿生三阶段梦境 + 多层可插拔后端

**核心文件**：`src/memory/`、`extensions/memory-core/`、`extensions/active-memory/`

### 存储体系

```
文件层（纯 Markdown）:
  MEMORY.md           → 长期记忆（每次会话注入）
  memory/YYYY-MM-DD.md → 每日笔记（今天/昨天自动加载）
  DREAMS.md           → 梦境日记（仅供人类审阅）

SQLite 索引（结构化）:
  files 表              → 被索引文件清单
  chunks 表             → 文件内容分块 + embedding
  FTS5 虚拟表           → 全文搜索（unicode61/trigram 分词）
  embedding_cache       → 嵌入向量缓存
```

### 搜索管道

```
memory_search（混合搜索）:
  向量搜索 (embedding)  +  关键词搜索 (FTS5)
        ↓              ↓
      合并 (weighted)
        ↓
  [可选] MMR 多样性重排
  [可选] 时间衰减
        ↓
  结果 → 短期召回追踪（异步）
```

### 三阶段梦境系统

```
定时后台（默认凌晨 3 点）:

  Light → REM → Deep
    │       │      │
    │       │      └── 六维评分晋升决策:
    │       │            Relevance (0.30) + Frequency (0.24)
    │       │            + Query Diversity (0.15) + Recency (0.15)
    │       │            + Consolidation (0.10)
    │       │            + Conceptual Richness (0.06)
    │       │            晋升门槛: minScore≥0.75 AND recallCount≥3
    │       │
    │       └── 短期→长期晋升（睡眠机制）
    │
    └── 每个扫描周期执行，生成第一人称诗意梦境日记
```

### Memory Flush（预防性保存）

压缩**之前**触发（非之后补救）：
- Token 门控：prompt ≥ contextWindow - reserveTokens
- 文件大小门控：活跃转录 ≥ 2MB
- 安全约束：只能追加到每日笔记，不能修改 MEMORY.md

### 设计思想

1. **"模型只记住写入磁盘的东西，没有隐藏状态"**——所有记忆最终以纯 Markdown 文件存储
2. **仿生梦境**是三阶段系统最独特的设计：模拟人类睡眠记忆巩固过程
3. **Active Memory 前置而非后置**：在 Agent 思考之前就把相关记忆准备好，不够强就返回 NONE
4. **四后端可插拔**：Builtin (SQLite)、QMD (二进制)、LanceDB、Honcho

---

## 7. Nanoclaw：Provider 管理的 Continuation + PreCompact 转录归档

**核心文件**：`src/db/session-state.ts`、`src/providers/claude.ts`

### 会话恢复

```
容器启动:
  migrateLegacyContinuation(providerName)
    → 读取 session_state 表中 "continuation:claude" 或 "continuation:codex"

查询期间:
  收到 'init' 事件 → 立即持久化 continuation token

/clear 命令:
  continuation = undefined → 从 DB 删除
```

### 上下文压缩

Nanoclaw 依赖 Claude Code SDK 的自动压缩（`CLAUDE_CODE_AUTO_COMPACT_WINDOW` = 165K tokens），但在压缩前有自定义钩子：

```
PreCompact hook:
  ├── 转录归档脚本:
  │     .jsonl → 解析为用户/助手消息 → conversations/*.md
  │
  └── 紧凑指令脚本:
        从 destinations 表读取 agent 目标
        输出 XML 结构保留指令
        传递给 SDK 的压缩 prompt
```

### 没有独立记忆系统

> Nanoclaw 不存储消息内容或会话摘要本身——它只持久化 provider 的 continuation token。记忆依赖于 Claude Code SDK 的内部机制。

### 设计思想

1. **跨会话记忆是 provider 的职责**：只保存不透明的 continuation token，Claude 恢复其 .jsonl 转录
2. **每个 provider 独立的 token**：切换到另一个 provider 不会留下垃圾
3. **紧凑指令防止丢失路由上下文**：Claude 的自动压缩可能会折叠消息结构——自定义指令让 agent 记住有哪些目的地

---

## 8. pi：JSONL 树状会话 + 增量结构化压缩

**核心文件**：`packages/agent/src/harness/session/`、`harness/compaction/`

### 会话存储格式

```jsonl
{"type":"session","version":3,"id":"...","cwd":"..."}
{"type":"message","id":"...","parentId":...,"message":{...}}
{"type":"compaction","id":"...","summary":"...","tokensBefore":12345}
{"type":"branch_summary","id":"...","summary":"...","fromId":"..."}
{"type":"label","id":"...","targetId":"...","label":"bookmark1"}
{"type":"leaf","id":"...","targetId":"..."}
```

`parentId` 形成树状结构，`leaf` 记录当前"头部"。支持分叉（分支对话）和回溯导航。

### 压缩结构

```
prepareCompaction():
  ├── findCutPoint():
  │     └── 向后扫描，保留 ~20K tokens (keepRecentTokens)
  │           在有效的切割点切割（user/branchSummary/custom 消息）
  │
  └── 返回 { firstKeptEntryId, messagesToSummarize[], isSplitTurn }

generateSummary(messagesToSummarize):
  结构化格式:
    ## Goal
    ## Constraints & Preferences
    ## Progress (Done / In Progress / Blocked)
    ## Key Decisions
    ## Next Steps
    ## Critical Context
  + 文件操作足迹

如果是二次压缩: UPDATE_SUMMARIZATION_PROMPT → 更新前次摘要
```

### 设计思想

1. **JSONL = 简单且可追加**：磁盘上的每个新条目只是追加，O(1) 复杂度
2. **树状结构**：parentId 指针支持分叉和回溯——像 git 分支一样管理对话
3. **增量压缩**：二次压缩使用 UPDATE 模式，不是重建——信息持续累积
4. **没有跨会话记忆**：会话是独立的 JSONL 文件

---

## 9. 横向对比总结

| 维度 | Claude-Code | OpenCode | Hermes Agent | CrewAI | OpenClaw | Nanoclaw | pi |
|------|------------|----------|-------------|--------|----------|----------|-----|
| **会话记忆** | Fork Agent 自动摘要 | compaction task 模板 | MemoryProvider 接口 | UnifiedMemory 模型 | 文件 + SQLite 索引 | Provider continuation | JSONL 树状存储 |
| **跨会话记忆** | 无（CLAUDE.md） | 无（指令文件） | 8 后端可插拔 | LanceDB/Qdrant | MEMORY.md + 梦境 | 无（provider 负责） | 无（分叉模拟） |
| **压缩方式** | 5 级分层压缩 | 摘要 + 修剪 + REPLAY | 迭代摘要（辅助模型） | 截断/摘要 | 阈值触发 + 预飞行 | PreCompact hook | 增量结构化摘要 |
| **触发条件** | 上下文 < 13K | isOverflow() | > 75% context | ContextLengthExceeded | 阈值 + 门控 | SDK 自动（165K） | token 估算 |
| **核心创新** | 5 级不破坏性排序 | 压缩是第一公民 task | 8 后端可插拔、迭代更新 | 自适应深度召回、LLM 合并 | 三阶段梦境、Active Memory | 紧凑指令防路由丢失 | 树状分支、增量 UPDATE |


## 10. 对 Argo 的启示

1. **vault 应分层**：短期（上下文压缩）+ 长期（文件/向量存储）两层架构是大多数项目的共识
2. **压缩策略由轻到重**：先截断 → 再折叠 → 最后 LLM 摘要。Claude-Code 的五级体系是最完善的参考
3. **MemoryProvider 可插拔接口**是长期记忆的正确抽象——Hermes Agent 的 8 后端设计证明了这一点
4. **OpenClaw 的梦境系统**虽然华丽，但对 MVP 过度设计。应先用文件存储（MEMORY.md 模式），后续再考虑向量+梦境
5. **pi 的树状 JSONL 存储**是对 stateful session 的优雅方案——可追加、可分叉、易持久化
6. **Nanoclaw 的 provider continuation 模式**对 Gateway 模式特别有价值：记忆管理可以部分委托给 provider
7. **CrewAI 的自适应深度召回**是长期记忆的最佳实践：浅搜索足够时不浪费 token，深搜索仅在置信度低时触发
