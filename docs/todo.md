# Argo 开发路线

## 阶段 1：基础模块 — deck, vault, sail, crew ▶

搭建四个零外部依赖的底层模块，为上层模块（reef、helm）提供工具、存储、模型、技能四个维度的能力基座。四个模块互不依赖，可并行开发。

### 模块依赖

  deck ──┐
  vault ─┤
  sail ──┼──→ reef ──→ helm ──→ cmd
  crew ─┘

### 模块目标

- [ ] deck — 工具注册表（ToolRegistry）+ 执行器（ToolExecutor）+ 三种 Executor（builtin/CLI/MCP）+ MCPManager 子进程生命周期管理
- [ ] vault — FileVault 默认文件系统实现、transcript.jsonl 追加写入、profile.md / rules.md 读写与预算控制
- [ ] sail — models.json 配置加载与自动生成、anthropic / openai / openai-compatible 三个 provider adapter、错误分类
- [ ] crew — SKILL.md 文件扫描、YAML frontmatter 解析、List / Instructions 渐进式披露接口

## 阶段 2：上下文压缩 — reef

依赖 sail（LLM 调用生成摘要）和 crew（加载压缩模板 SKILL.md），实现会话消息历史的自动裁剪与摘要压缩，防止 context window 溢出。

### 模块依赖

  sail ──→ reef ←── crew

### 模块目标

- [ ] reef — Compact() 入口、shouldCompact 触发判定（80% 阈值）、findCutPoint 切割算法、LLM 摘要生成与错误降级、消息重组

## 阶段 3：核心循环 — helm

依赖全部五个底层模块（deck / vault / sail / crew / reef），实现 Agent 主循环：用户输入 → LLM 调用 → 工具执行 → 结果返回 → 循环。通过 channel 流式推送事件，调用方持有会话状态。

### 模块依赖

  deck ──┐
  vault ─┤
  sail ──┼──→ helm
  crew ─┤
  reef ─┘

### 模块目标

- [ ] helm — runLoop() 核心循环（会话级缓存 system prompt + tools）、buildSystemPrompt() 多源拼装、channel 流式事件推送（text_delta / tool_use / done / error）

## 阶段 4：双入口与集成 — cmd

依赖 helm 及全部底层模块，提供 CLI 和 Gateway 两种运行形态入口，完成端到端集成。

### 模块依赖

  helm ──→ cmd/run
    └────→ cmd/serve

### 模块目标

- [ ] cmd/run — CLI 交互式入口（stdin 输入 / stdout 流式输出）
- [ ] cmd/serve — Gateway RPC 服务端入口（常驻进程，客户端远程连接）
