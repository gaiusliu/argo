# Handoff — 2026-07-14（crew 平坦模式 完成）

## 本次会话成果

### crew 平坦模式（迭代 012，已闭环）

实现完整的技能管理模块。代码 + 测试 + 文档全链路交付。

| 变更 | 文件 | 说明 |
|------|------|------|
| crew 核心包 | `src/crew/types.go` `scanner.go` `loader.go` `registry.go` `crew_test.go` | SkillInfo + Skills 接口 + 磁盘实时扫描（不缓存），6 个测试 PASS |
| System Prompt 注入 | `src/helm/prompt.go` `src/helm/phase_model.go` | `<available_skills>` XML 块（L1 披露） |
| skill 工具 | `src/deck/builtin_tool.go` | `skillHandler` — LLM 自主激活路径（L2） |
| /skill-name 检测 | `src/server/prompt.go` | `resolveSlashCommand()` — server 层拦截，所有客户端通用 |
| 全局初始化 | `src/argo-server/main.go` | 启动时 `crew.Init()` |
| 文档 | `docs/modules/crew/design.md` `docs/architecture.md` `docs/iterations/012-crew-flat/main.md` | 设计文档 + 架构更新 + 迭代归档 |
| README | `README.md` | 加入 crew 模块，模块关系图改为 ASCII |

### 关键设计决策

| 决策 | 说明 |
|------|------|
| pi-style 磁盘读取 | Crew 不缓存——每次 `List()` / `Instructions()` 从磁盘重新扫描，新安装的技能立即可见，无需 reload |
| 双激活路径 | skill 工具（LLM 自主） + /skill-name（用户显式），server 层统一检测 |
| 轻量定位 | 不做技能安装工具、不做条件激活、不做热更新、不做安全扫描 |
| 不做 Instructions() 路径直读优化 | 技能 < 20 时无感知，> 50 时再做 |

### commit 记录

```
f171ec8 architecture.md 模块关系图换为 ASCII
e4a9438 修正 architecture.md mermaid 图
a0d9fc8 更新 README 架构图：加入 crew 模块
88358b6 crew 平坦模式：技能发现 + 渐进式披露 + skill 工具 + /skill-name
```

## 已知残留问题（从前期继承，本会话未触及）

| 问题 | 位置 | 说明 |
|------|------|------|
| 死代码 `sail.Window()` | `src/sail/sail.go:218` | 0 调用者 |
| `http.DefaultClient` 残留 | `src/sail/adapter.go:315`、`src/deck/builtin_tool.go:611,674` | 不走 ProviderOpenAI 路径 |
| `NewProviderOpenAI` 冗余参数 | `src/sail/sail.go` | messages/tools 参数已无意义 |
| 旧 `Sail` 接口 | `src/sail/sail.go:16` | 0 外部调用者 |
| `knot.LookupModel()` | `src/knot/config.go:150` | 仅被 `sail.Window()` 调用 |

## 待办清单（从前期继承 + 新识别）

| # | 任务 | 复杂度 | 说明 |
|---|------|--------|------|
| **crew** | | | |
| — | 技能树模式 | 中 | `Tree()` 接口 + `browse` 工具 + 分类模式触发（技能 > N） |
| — | L3 资源文件 | 低 | 技能目录下 references/ 的索引和暴露 |
| — | Instructions() 路径优化 | 低 | 技能 > 50 时直接读路径而非全量扫描 |
| **待办（handoff 前期）** | | | |
| 9 | Asking 不断连 | 中 | 当前 goroutine 退出 + 客户端走新 HTTP 请求。改为同 SSE 连接内收 verdicts |
| 10 | reef compact | 高 | Tale 消息裁剪 + LLM 摘要 + 上下文重组。当前是桩 |
| 7 | crew（原） | ✅ 已完成 | 见本次会话成果 |
| 8 | AgentService | 中 | `src/server/prompt.go` handler 瘦身 |

## 未来可做

| 项 | 触发条件 |
|----|----------|
| 事件总线 | 多客户端同时操作同一 session |
| `helm.Machine` | 当 `newRunner` 需要超过 3 个共享参数时 |

## 建议技能

- `two-axis-review`：crew 实现完成后做代码审查
- `grow-doc`：后续迭代生成迭代文档 + 更新模块 design.md
