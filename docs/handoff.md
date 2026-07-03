# Handoff — RunLoop 闭环完成

## 背景

Argo（极简 AI Agent 运行基座）。5 个 Go 包，R eAct 核心循环已跑通（桩阶段）。

## 当前状态

```
src/
├── knot/types.go      — 跨模块共享类型（12 个 Event 常量 + Message/ChatRequest/Event/ToolUse 等）
├── deck/              — ✅ 工具管理 MVP（5 文件）
├── sail/              — 🚧 桩
│   ├── sail.go         — Chat()/Window() 桩
│   └── config.go       — LoadConfig/DefaultModel/LookupModel + Provider/Model/Limit 配置结构
├── reef/reef.go       — 🚧 桩，Compact() 原样返回
└── helm/              — ✅ 核心循环
    ├── loop.go         — RunLoop() 五步闭环
    ├── prompt.go       — buildSystemPrompt() 四块拼装
    ├── stream.go       — newEventChannel()
    └── types.go        — TurnState（含 ModelName）
```

## RunLoop 五步

```
1. reef.Compact(ctx, messages, sail.Window(modelName))  ✅ 失败降级
2. sail.Chat(ctx, state.ModelName, req)                 ✅ 桩返回 done
3. 流式消费：累计 textDelta → assistant message         ✅
            收集 toolUseStart                          ✅
            转发所有事件到 out channel                   ✅
4. 逐条 deck.Lookup + Tool.Execute + 发 toolUseDone     ✅
5. assistant message + tool result 注入 state.Messages   ✅
   无 tool_use → 退出；有 tool_use → 继续循环
```

## Event 类型（12 个）

start / textStart / textDelta / textDone / thinkingStart / thinkingDelta / thinkingDone / toolUseStart / toolUseDelta / toolUseDone / done / error

## 关键决策

| 编号 | 决策 |
|---|---|
| - | 各模块包级单例（deck.List()、sail.Chat()），废弃 HelmDeps |
| - | 跨模块类型统一进 knot 包 |
| - | 模型选择权在 TurnState.ModelName，空值 = sail 取 config.json 的 model 字段 |
| - | 出错的 event type 保留，不混入 done |
| - | Event 字段精简为 Delta（text/thinking 共用）+ ToolUse + Done + Err |

## 下一步

### P0 — 让 sail 真正调用 LLM

- 实现 adapter（anthropic/openai/openai-compatible），把 HTTP SSE 流转为 knot.Event
- 接上 models.json → config.json 的 provider 配置

### P1 — reef 压缩逻辑

- shouldCompact 触发判定（80% 阈值）
- findCutPoint 切割 + LLM 摘要生成

### P2 — 其余模块

- vault、crew 模块
- cmd/run（CLI）、cmd/serve（Gateway）入口

## 关键参考

| 文档 | 位置 |
|---|---|
| 项目架构 | [docs/what.md](docs/what.md) / [docs/how.md](docs/how.md) |
| 开发路线 | [docs/todo.md](docs/todo.md) |
| helm 设计 | [docs/modules/helm/design.md](docs/modules/helm/design.md) |
| helm 迭代 | [docs/iterations/002-helm-mvp/main.md](docs/iterations/002-helm-mvp/main.md) |
| sail 设计 | [docs/modules/sail/](docs/modules/sail/)（what/how/decisions） |
| Go 代码规范 | `go-styleguide/` |
