# 002-helm-mvp

## 目标

搭建 helm Agent 核心循环的 MVP 版本。通过 mock/stub 依赖跑通 "用户输入 → LLM 调用 → 工具执行 → 结果返回 → 循环" 的完整链路。

## 范围

- [x] 核心循环 RunLoop()：ReAct 循环，无工具调用即退出，无 maxTurns 硬停止
- [x] system prompt 组装 buildSystemPrompt()：四块拼接，见 DEC-005
- [x] 流式事件推送：<-chan knot.Event，12 种类型（start / textStart / textDelta / textDone / thinkingStart / thinkingDelta / thinkingDone / toolUseStart / toolUseDelta / toolUseDone / done / error）
- [x] 类型定义：TurnState（含 ModelName）/ SkillInfo / MemoryEntry（共享类型移入 knot 包，见 DEC-008）

## 关键决策

- [DEC-005](../../decisions.md)：System prompt 四块结构——行为核心 → 环境 → 工具 → 用户指令
- [DEC-008](../../decisions.md)：knot 包——跨模块流转类型的独立包
- [DEC-009](../../decisions.md)：模型选择单一来源——TurnState.ModelName
- [DEC-011](../../decisions.md)：reef 裁剪失败降级

## 文件

| 文件 | 内容 |
|------|------|
| `src/knot/types.go` | 跨模块共享类型：Message / Event / ChatRequest / ToolUse / DonePayload + 12 个 Event 常量 |
| `src/helm/loop.go` | RunLoop() 五步闭环：Compact → Chat → 消费 → 执行 → 注入 |
| `src/helm/prompt.go` | buildSystemPrompt() 四块拼装 |
| `src/helm/stream.go` | newEventChannel() channel 创建 |
| `src/helm/types.go` | TurnState（含 ModelName）/ SkillInfo / MemoryEntry |
| `src/sail/sail.go` | Chat() / Window() 桩 |
| `src/sail/config.go` | LoadConfig / DefaultModel / LookupModel + Provider/Model/Limit 配置结构 |
| `src/reef/reef.go` | Compact() 桩（原样返回） |

## 状态

✅ 完成（桩阶段）
