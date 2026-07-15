# Compactor / Truncator 架构设计

## 1. 问题背景：为什么要做上下文管理

### 1.1 Lost in the Middle — LLM 的注意力是 U 形的

斯坦福 2023 年首次证实，ICLR 2026 获杰出论文奖进一步验证：

| 模型 | 单轮准确率 | 多轮准确率 | 下降 |
|------|-----------|-----------|------|
| GPT-4.1 | 91.7% | 70.7% | -21 pp |
| Claude 3.7 Sonnet | 85.4% | 70.0% | -15 pp |
| Gemini 2.5 Pro | 90.2% | 64.3% | -26 pp |

这不是训练质量问题——2025 年研究发现**未经训练的随机权重模型**就存在 U 形曲线。因果掩码 + 残差连接的图拓扑决定了：中间位置 token 的梯度传播路径随层数阶乘衰减（H 层时约 `1/(H-1)!`），训练无法消除。

**启示**：模型天然擅长记住第一条指令（首因效应）和最新状态（近因效应），对上下文中间的信息几乎"看不见"。Agent 中老旧的工具结果恰好落在这个 U 形谷底——它们不仅在浪费 token，也在浪费注意力。

### 1.2 Context Drag — 已完成使命的工具结果长期滞留在上下文

工具调用通常是为一个局部问题找证据："这个函数在哪定义？""哪些地方调用了它？""测试失败在哪个步骤？"一旦模型据此做出判断，大部分工具输出的价值就急剧下降。

但传统做法默认把完整结果留在消息历史里。典型会话上下文 8-46MB，其中 40-67% 是已无价值的陈旧内容。到第 16 轮，约束遵守率从 73% 降至 33%。

工具结果越冗长，模型看到的旁支线索越多——一段完整日志可能包含已恢复的旧错误，一个大文件可能暴露无关的异常代码。这些线索可能诱发新一轮工具调用，进而产生更多上下文。有从业者基准测试发现 Agent 质量在约 15 次工具调用后急剧下降。

**启示**：工具结果更适合作为**局部证据**——在调用发生后的少数几轮内使用，而非作为**长期记忆**一直保留。

### 1.3 关于激进截断的取舍

社区存在两种立场：

**激进派（2048-token 截断）**：工具结果压到约 2K token，保留开头+结尾（结构信息），原文存外置存储，按需重取。实验数据：累计 token 消耗下降约 40%，每成功成本下降约 30%，成功率无回退甚至微升。代表：Sophon（Rust，确定性规则，94.6% 历史压缩率）、Codex CLI（10K token 硬限）。

**保守派**：输出只占 token 的 ~4%，输入才是大头（93.4%），输出压缩解决了次要问题。应优先优化代码库上下文的注入方式而非工具输出。

Argo 的立场：两条线都做，但不预设谁更重要。通过策略接口让两者可独立切换，实际数据决定优先级。

---

## 2. 策略体系

### 2.1 两条线

| | Compactor | Truncator |
|------|-----------|-----------|
| **做什么** | 压缩消息历史 | 控制单次工具结果入上下文 |
| **何时** | LLM 调用前（helm） | 工具执行后（deck） |
| **怎么压** | 可能调 LLM 做语义摘要 | 纯文本/结构化处理 |
| **失败** | 回退 passthrough，不中断 | 回退 passthrough，不中断 |

### 2.2 Compactor 策略目录

| 策略 | 阶段 | 说明 |
|------|------|------|
| `passthrough` | MVP | 原样返回，零操作 |
| `llm-summary` | Phase 3 | 调 LLM 生成 5 字段结构化摘要，旧消息被摘要替代，原文由 vault 保管。支持增量 UPDATE。断路器：连续 2 次节省 < 10% 则跳过 |

### 2.3 Truncator 策略目录

| 策略 | 阶段 | 说明 |
|------|------|------|
| `passthrough` | MVP | 不截断 |
| `head-tail` | MVP | 保留首尾各约 8K 字符，中间截断。原文存临时文件，TTL 7 天 |
| `intent-evidence` | Phase 2 | 工具调用前声明意图（ToolIntent），返回后按证据槽位提取。原文落盘为临时文件，可重取 |

### 2.4 ToolIntent → Observation 流程（Phase 2 核心理念）

这不是"更狠的截断"，而是把工具结果从"默认进入长期上下文"改为"意图约束的证据提取 + 按需重取"。

```
工具调用前: 模型声明 ToolIntent
  - 当前假设
  - 这次调用要回答的问题
  - 需要什么证据才算够
  - 什么情况下应该停止调查

工具返回后: 系统提取 Observation
  - answers:          回答原问题的证据
  - counter_evidence: 推翻当前假设的证据（强制保留，防御确认偏差）
  - unexpected:       虽然没被问到，但明显异常的信息（强制保留）
  - deferred_leads:   可能有用但不该立刻展开的方向
  - originalPath:     原文临时文件路径
```

**与普通截断的本质区别**：普通截断是"输出太长，砍掉"，防爆机制。Observation 是上下文准入控制——完整结果存在，但默认不进入消息历史。进入上下文的不是原始输出，而是经过任务意图过滤的证据包。

**必须坚持的约束**：
- 对代码、日志、错误信息，尽量保留原文片段 + 行号/范围，不做纯 NL 总结
- `counter_evidence` 和 `unexpected` 槽位不可被过滤规则忽略
- 每个结论都要能回指到原文文件及原文范围

### 2.5 新方向的三级处理（Phase 3）

工具结果中发现的额外线索，不应全部自动展开，也不应全部封堵：

- **auto**：不探索就无法完成原任务 / 测试失败直接指向 / 推翻了当前假设
- **queued**：看起来相关但不是当前 blocker / 可能是重构或额外 bug / 需要较大范围读取
- **ask_user**：明显扩大任务边界 / 涉及产品决策 / 大量额外 token 开销

---

## 3. 架构

### 3.1 包布局

```
src/reef/
  strategy.go              ← Compactor + Truncator 接口
  registry.go              ← 注册表 + 工厂
  types.go                 ← ToolIntent / Observation / EvidenceSlot（Phase 1 预声明）
  compact_passthrough.go   ← PassthroughCompactor
  compact_llm_summary.go   ← LLMSummaryCompactor（Phase 3）
  truncation_passthrough.go ← PassthroughTruncator
  truncation_headtail.go   ← HeadTailTruncator（从 deck/truncation.go 迁入）
  truncation_intent.go     ← IntentTruncator（Phase 2）
  truncation_adaptive.go   ← 自适应预算（Phase 3）
```

### 3.2 依赖方向

```
knot (types, config)
  ↑
reef (接口 + 实现)          ← 不 import sail / deck / helm
  ↑           ↑
deck          helm
(Truncator)   (Compactor)
```

### 3.3 字段注入

Truncator 和 Compactor 不通过装饰器、不靠包级变量，而是构造时注入到使用者结构体：

- `builtinTool.truncator` / `cliTool.truncator` — `RegisterTools(trunc)` 时传入构造器
- `PhaseRunnerModel.Compactor` — `NewPhaseRunnerModel()` 时从配置解析注入

消除 `postProcess(result)` 硬编码，但不引入额外抽象层。

### 3.4 SummaryFunc 适配器（reef 无感）

Compactor 的 LLM 摘要策略需要调 LLM，但 reef 不依赖 sail。通过 `SummaryFunc` 函数类型解耦——reef 定义签名，helm 用 `sail.Provider` 实现适配器注入。

### 3.5 配置驱动

```json
{
  "context": {
    "compact": "passthrough",
    "truncation": "head-tail"
  }
}
```

策略名 → 注册表查找 → 工厂构建。`config.json` 缺少 `context` key 时默认 compact=`passthrough`, truncation=`head-tail`。

### 3.6 降级路径

- Resolve 失败 → warn + 回退 passthrough
- Compact 失败 → warn + 原消息继续，不中断 agent loop
- Truncation 的 store 失败 → 截断照常，仅不加 originalPath
- 断路器（Phase 3）：连续 2 次 compact 节省 < 10% → 本次 session 跳过

### 3.7 新策略接入代价

```go
// 1. 写 struct
type MyCompactor struct{ ... }
func (MyCompactor) Info() StrategyInfo { ... }
func (MyCompactor) Compact(...) { ... }

// 2. init 注册
func init() { RegisterCompactor("my-strategy", ...) }

// 3. config.json 改一行
{ "context": { "compact": "my-strategy" } }
```

零侵入现有核心代码。

---

## 4. 阶段规划

| 阶段 | Compactor | Truncator |
|------|-----------|-----------|
| MVP | 接口 + passthrough | 接口 + head-tail（迁现）、passthrough |
| Phase 2 | 不变 | + intent-evidence：ToolIntent → Observation |
| Phase 3 | + llm-summary | + 自适应预算 + deferred_lead 三级 |

详见 `docs/mvp-compactor-truncator.md`（MVP 执行计划）。
