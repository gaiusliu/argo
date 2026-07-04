# Handoff — sail MVP 启动

## 背景

上一轮交接完成了 deck 模块的 WebFetch/WebSearch 工具。本次会话确定了下一步开发方向：**sail 模块 MVP**——将模型 API 调用的桩实现替换为真实的 LLM 接入。

## 上轮 TODO 处理结论

上一轮 handoff 的 6 项待办，本次确认：

| 待办 | 结论 |
|------|------|
| 权限/授权机制 (DEC-017) | 仍为最高优先级，但需等 sail 实现后才能在真实工具调用链中测试 |
| 分流机制 → session-based | 已完成 query hash 分流 |
| SSE 容错解析 | 暂不做 |
| CheckRedirect 跨 host 限制 | 暂不做 |
| HTML→Markdown 转换 | 暂不做 |
| WebSearch 暴露更多参数 | 暂不做 |

## 本次会话产出

### 决策：下一个模块选 sail

- helm 的 RunLoop 已完整但 `sail.Chat()` 是桩，整个 Agent 系统无法端到端运行
- sail 是当前关键路径——不实现它，reef、vault、权限机制都无法真正测试

### 新建迭代文档

`docs/iterations/003-sail-mvp/main.md`，按 grow-doc 标准创建：

- **目标**：两个 adapter（openai + openai-compatible），覆盖 ChatGPT / DeepSeek / MiniMax / Kimi / 豆包
- **关键流程**：Chat 流程（查配置 → 选 adapter → 流式推送 → classifyError）+ Window 三级查找（用户覆盖 → 内置表 → 保守回退）
- **6 个功能项**：核心类型定义 → 配置加载完善 → Provider Adapter ×2 → Client 实现 → 错误分类 + 重试 → 模型元数据内置表
- **6 个错误场景**：auth / quota / rate_limit / server_error / context_overflow / network，各有 MVP 阶段考量说明
- **5 条公共设计约束**
- 全部 ⏳ 待开始，暂不拆功能文档

### 现有文档状态

| 文件 | 状态 |
|------|------|
| `docs/modules/sail/what.md` | 暂保留，开发参考，闭环后删除 |
| `docs/modules/sail/how.md` | 暂保留，开发参考，闭环后删除 |
| `docs/modules/sail/decisions.md` | DEC-S01~S06，符合 grow-doc 格式，不动 |
| `docs/modules/sail/todo.md` | 暂保留，开发参考，闭环后删除 |
| `docs/modules/sail/design.md` | **不生成**——grow-doc 规则：迭代闭环后才生成 |

## 下一步：开始开发

按 003-sail-mvp/main.md 的功能列表顺序：

1. 核心类型定义（APIError、ModelCapability）
2. 配置加载完善（GenerateDefaultConfig——扫描环境变量自动生成 `models.json`）
3. Provider Adapter ×2（openai + openai-compatible）
4. Client 实现（Chat + Window 替换桩）
5. 错误分类 + 重试
6. 模型元数据内置表（5 个厂家常用模型的 context window）

## 项目上下文

- Go 代码规范：`go-styleguide/`
- 接口由消费者（helm）定义，实现在 sail
- commit message 中文描述，不使用 Co-Authored-By
- 先说明想法征求同意，再修改代码

## Suggested Skills

- **grow-doc**：迭代闭环时提取 design.md，或开发中将功能文档从 main.md 拆分
- **code-review**：每完成一个功能后审查变更
- **tdd**：如需测试先行开发
