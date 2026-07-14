# Argo

极简 AI Coding Agent 框架。Go 实现，状态机驱动 "模型调用 → 工具执行 → 循环" 的 ReAct 链路。

## 解决什么问题

为 AI coding 场景提供最小可用的 Agent 运行基座——工具注册、权限门控、流式 LLM 调用、对话持久化、会话管理。模块各自独立、通过接口协作，任一模块可替换。

## 架构

```
CLI（HTTP 客户端）──▶ argo-server（HTTP + SSE）──▶ helm（状态机）
                                                    │
           ┌────────────────────────┬───────────────┴───────────────┬────────────────┐
           ▼                        ▼                               ▼                ▼
          sail（模型）            deck（工具）                    brig（权限）     crew（技能）
                                    │
          vault（存储）──▶ voyage（会话聚合）
```

## 快速开始

```bash
# 启动服务端
./argo-server

# 启动 CLI 客户端
./argo-cli
```

## 文档

| 文档 | 说明 |
|------|------|
| [architecture.md](docs/architecture.md) | 模块职责 + 数据流 + 模块间关系 |
| [decisions.md](docs/decisions.md) | 技术决策（append-only） |
| [docs/modules/](docs/modules/) | 各模块设计文档 |
| [docs/iterations/](docs/iterations/) | 迭代开发记录 |
