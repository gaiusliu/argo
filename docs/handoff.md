# Handoff — sail MVP 启动

## 背景

本会话完成了 sail 模块 MVP 的前置基础工作：统一配置系统 + 类型迁移。为下一步 adapter 开发铺平了道路。

## 本次产出

### 1. 统一配置系统

配置加载入口统一为 `knot.LoadConfig()`：

```
LoadConfig()
├─ GenerateDefaultConfig()        → 扫描 env 生成 provider 骨架
├─ 读 config.json (不存在 → 写骨架到磁盘)
└─ mergeConfig(骨架, 用户配置)     → 字段级合并
```

- `knownProviders`：内置 provider → env var 映射（anthropic/openai/deepseek）
- 用户配置只需写 models，env 等字段由骨架自动补齐
- 首次启动自动生成 `config.json`

### 2. 类型迁移至 knot

| 类型 | 来源 | 说明 |
|------|------|------|
| `Tool` / `ToolResult` / `ToolSource` / `ResultStatus` | deck → knot | 解除 knot → deck 循环依赖 |
| `EventType` string | 新增 | Event 类型常量改为自定义类型 |
| `MessageRole` string | 新增 | Message 角色改为自定义类型 |
| `Config` / `DeckConfig` / `SailConfig` | 新增 | 统一配置结构 |
| `ProviderConfig` / `ModelConfig` / `ModelLimit` | sail → knot | 旧 sail config 类型 |
| `ConfigPath` / `LoadConfig` / `DefaultModel` / `LookupModel` | 各模块 → knot | 统一配置函数 |

### 3. 依赖关系

```
knot (零外部依赖)
├── deck → knot
├── sail → knot
├── reef → knot
└── helm → deck + sail + reef + knot
```

## 当前进度

| # | 功能 | 状态 |
|---|------|------|
| 1 | 核心类型定义 | ✅ 完成 |
| 2 | 配置加载完善 | ✅ 完成 |
| 3 | Provider Adapter ×2 | ⏳ 待开始 |
| 4 | Client 实现 | ⏳ 待开始 |
| 5 | 错误分类 + 重试 | ⏳ 待开始 |
| 6 | 模型元数据内置表 | ⏳ 待开始 |

## 下一步：Provider Adapter ×2

### 设计要点（已达成共识）

- 内部 adapter 接口，helm 只收 `<-chan knot.Event`
- 两个 adapter：`openaiAdapter`（ChatGPT）+ `compatibleAdapter`（DeepSeek/MiniMax/Kimi/豆包）
- 共用 OpenAI Chat Completions SSE 格式，区别在 base_url
- HTTP 超时通过 `context.Context` 传导
- API Key 的 `${ENV_VAR}` 解析在 Client 层统一完成

### 核心流程

```
adapter.Chat(ctx, modelName, req)
├─ knot.ChatRequest → OpenAI JSON body
├─ HTTP POST stream=true
├─ SSE 逐行解析
├─ chunk → knot.Event (textDelta / toolUse / done / error)
└─ close channel
```

### 相关决策

- 见 `docs/modules/sail/decisions.md` DEC-S01~S06
- 迭代文档：`docs/iterations/003-sail-mvp/main.md`

## 项目上下文

- Go 代码规范：`go-styleguide/`（待创建）
- 接口由消费者（helm）定义，实现在 sail
- commit message 中文描述
- 先说明想法征求同意，再修改代码
- 每次代码生成不超过 50 行
