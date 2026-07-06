# knot

## 概述

knot 是 Argo 的跨模块共享类型包。存放不属于任何单一模块但在核心流程中跨模块流转的类型——消息、事件、工具调用、配置、AskUser 回调签名。knot 是一个叶子包，不依赖 Argo 其他模块（DEC-008）。

## 核心概念

| 概念 | 说明 |
|------|------|
| Message | 单条对话消息（Role + Content），贯穿 helm ↔ sail 全程 |
| Event | 流式事件载体，通过 channel 在 helm → 调用方之间推送 |
| ToolUse | 工具调用载体——LLM 发起时 Result.Status=StatusDefault，执行后 Result 填充 |
| ToolResult | 工具执行结果（Output + Status + Metadata） |
| Tool | 工具接口——Name / Description / Source / ParametersSchema / Execute，由 deck 实现 |
| AskUser | 用户确认回调签名——`func(tu ToolUse, reason string) (bool, error)`，brig 判定 Ask 时 RunLoop 调用 |
| Config | 全局配置聚合——DeckConfig + SailConfig，JSON 序列化 |
| TokenLimit / TokenUsage | 模型 token 限制与消耗统计 |

## 设计原则

### 单向依赖

knot 是依赖链的起点，不 import 任何 Argo 内部包。模块关系：

```
knot  ←  deck  ←  helm  ←  cmd/run
  ↑        ↑        ↑
  └── sail ┘        └── brig（knot + helm + deck）
  └── reef
  └── brig（仅依赖 knot.ToolUse）
```

### 类型归属判定

类型放入 knot 的标准：**被 ≥2 个模块引用，且不属于任一模块的专属职责**。

- `Message`、`Event`、`ToolUse`：helm ↔ sail 之间流式通信
- `Tool`、`ToolResult`：deck 定义，但 helm/brig/sail 都引用（knot 中只有接口，实现在 deck）——`ParametersSchema()` 方法供 sail adapter 构造 OpenAI function calling 的 `tools` 字段
- `AskUser`：brig 判定、helm 调用、cmd/run 实现——三方的桥梁类型
- `Config`：deck + sail + vault 共用的配置结构

### 不放入 knot 的

- 模块内部类型（如 `streamState`、`builtinTool`）留在各自包内
- 模块专属数据结构（如 `TurnState` 在 helm、`Rule` 在 brig）
