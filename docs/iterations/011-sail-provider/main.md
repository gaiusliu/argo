# 011：sail Provider 接口重构

**日期**：2026-07-13
**状态**：✅ 已完成（闭环 → `docs/sail/design.md`）
**涉及模块**：sail、helm、knot

## 目标

将 sail 推至其原本意图的形态：`Provider` 接口 + 按接入方式区分的具体实现。Provider 在 `PhaseRunnerModel.Run()` 中根据 `st.Model` 动态创建，替代 4 散字段和 `getModelConfig()`。

## 范围

### 已完成

| # | 改造项 | 涉及文件 |
|---|--------|---------|
| 1 | `sail.Provider` 接口（`Name`/`Model`/`Chat`）+ `NewProvider` 工厂 | `src/sail/provider.go`（新建） |
| 2 | `SailOpenAI` → `ProviderOpenAI`，实现 Provider 接口 | `src/sail/sail.go` |
| 3 | `ProviderConfig.API` 字段（`"openai-completions"`） | `src/knot/types.go` |
| 4 | `PhaseRunnerModel` 注入 Provider，删 4 散字段 + `getModelConfig`/`isOpenAI` | `src/helm/phase_model.go` |

### 不含

- Anthropic / 其他 Provider 实现（未来扩展）
- `deck.Registry` 实例化（已跳过）
- `crew` 模块实现（#7）

## 公共设计约束

1. **Provider = 一个可用的 LLM 连接**，包含 Name/Model/Chat，由 `sail.NewProvider(model)` 从配置 + 环境变量解析
2. **Provider 在 `Run()` 中创建**——`NewPhaseRunnerModel()` 保持零参，`Run()` 内部调 `sail.NewProvider(st.Model)`，每轮可重建
3. **未来子代理**：各自 `NewProvider(subAgentModel)`，独立 Provider
4. **API 协议显式声明**：`ProviderConfig.API` 字段（参照 pi/opencode/openclaw），`"openai-completions"` 或空默认。`NewProvider` 据此路由

## 设计决策

| 决策 | 说明 |
|------|------|
| `ProviderConfig.API` 字段 | 显式声明协议，不从 provider 名称推断（参照三个参考项目） |
| `Model` 字段 unexported | 避免与方法 `Model()` 同名冲突 |
| `NewProviderOpenAI` 保留 | 供测试等场景直接构造注入 httpClient |

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-07-13 | 全部完成：Provider 接口 + ProviderOpenAI + NewProvider + PhaseRunnerModel 重构 |
| 2026-07-13 | 迭代开始 |
