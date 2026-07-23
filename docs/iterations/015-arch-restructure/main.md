# 015：三级作用域架构重构

**日期**：2026-07-23 ~
**状态**：⏳ 待开始
**涉及模块**：全部模块（pact 置换 knot、新增 sight/board/press/clip/lore/logx、删除 helm）

## 目标

将 Argo 从「全局单例 + helm 驱动的单体」重构为「三级作用域 + Voyage 自驱动 + 注入式依赖 + 持久化原语统一」的模块化架构。核心收益：支持热重载配置、多租户部署、后端可替换（File → DB）。

## 范围

### 包含

- **pact**：knot 重命名为 pact，清理全局函数，只留类型定义
- **L1-store**：Journal + Checkpoint 接口 + file 实现（不依赖现有代码）
- **L1-sight**：Sail → Sight，无状态 Take，New 签名增加 hands + client 参数
- **L1-press**：Compactor → Press，删除全局注册表，New 改为纯函数
- **L1-clip**：Truncator → Clip，删除全局注册表
- **L1-board**：Brig → Board，保持（原本无全局单例）
- **L1-deck**：Deck 改名 Hand，消灭 defaultRegistry，NewDeck 传入 builtin
- **L1-lore**：Crew → Lore，保持（原本已按实例设计）
- **voyage 自驱动**：helm 循环并入 Voyage.SetSail()，Phase 五态：Sight → Read → Call → Heave → Dock
- **server 装配层**：handlePrompt 成为唯一装配点，三级作用域隔离（进程级 / 请求级 / 会话级）
- **logx**：wake 重命名，不撞标准库 `log`

### 不含

- Blob Store 快照恢复（见迭代 016）
- 多租户 / DB 后端实际实现
- TokenUsage 重构

## 公共设计约束

- 所有包只 import pact，不互相依赖（press 可额外 import sight）
- voyage import 所有域包
- server 是唯一装配点
- 无循环依赖
- 迁移过程中每步保持编译通过

## 整体开发状态

| 功能 | 状态 | 关联 demo |
|------|------|----------|
| L1-store：Journal + Checkpoint | ⏳ 待开始 | [.plan-demos/arch-restructure/L1-store/](../../.plan-demos/arch-restructure/L1-store/) |
| L1-sight：无状态 Take | ⏳ 待开始 | [.plan-demos/arch-restructure/L1-sight/](../../.plan-demos/arch-restructure/L1-sight/) |
| L1-press：删除全局注册表 | ⏳ 待开始 | [.plan-demos/arch-restructure/L1-press/](../../.plan-demos/arch-restructure/L1-press/) |
| L1-clip：删除全局注册表 | ⏳ 待开始 | [.plan-demos/arch-restructure/L1-clip/](../../.plan-demos/arch-restructure/L1-clip/) |
| L1-board：保持 | ⏳ 待开始 | [.plan-demos/arch-restructure/L1-board/](../../.plan-demos/arch-restructure/L1-board/) |
| L1-deck：消灭默认注册表 | ⏳ 待开始 | [.plan-demos/arch-restructure/L1-deck/](../../.plan-demos/arch-restructure/L1-deck/) |
| pact：knot → pact 重命名 | ⏳ 待开始 | — |
| voyage 自驱动：helm 删除 | ⏳ 待开始 | [.plan-demos/arch-restructure/L0-arch/](../../.plan-demos/arch-restructure/L0-arch/) |
| server 装配层 | ⏳ 待开始 | — |
| logx：wake 重命名 | ⏳ 待开始 | — |

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-07-23 | 迭代文档生成 |
