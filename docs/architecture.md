# Argo 技术架构

## 模块清单（9 模块 + knot + server）

| 模块 | 职责 | 状态 |
|------|------|------|
| **knot** | 跨模块共享类型：Message / Event / ToolUse / Config / ToolVerdict | ✅ 稳定 |
| **deck** | 工具注册、发现、调用 + Truncator 截断。内置工具 + CLI 外部工具，统一 Tool 接口 | ✅ 稳定 |
| **sail** | LLM 接入层。Provider 接口（Name/Model/Chat）+ ProviderOpenAI 实现，`NewProvider` 工厂按 `api` 协议路由 | ✅ 稳定 |
| **vault** | 纯存储层。transcript.jsonl 读写 + Message ↔ Record 转换 | ✅ 稳定 |
| **brig** | 权限门控。Gate 接口 + Warden 分层裁决（Iron 红线 → Cord 可配置 → 用户审批 → Ask）+ RuleLoader 规则来源 | ✅ 稳定 |
| **voyage** | 会话运行时聚合体。Voyage = Info + Vault + Gate + Tale + Phase/Model/ToolUses，会话 CRUD + 控制流持久化 | ✅ 稳定 |
| **helm** | Agent 核心循环。状态机四 Phase + PhaseRunner（`Run(ctx, v, emit) bool`）+ Compactor 压缩 + checkpoint | ✅ 稳定 |
| **server** | 无状态 HTTP 后端。chi 路由 + SSE 流式事件 | ✅ 稳定 |
| **wake** | 日志模块。日期+大小自动轮转，过期清理 | ✅ 稳定 |
| **crew** | 技能管理。SKILL.md 扫描 + 渐进式披露 + skill 工具 + /skill-name | ✅ 稳定 |

## 模块关系

```
CLI ──→ Server ──→ Helm ──→ Voyage ─┬──→ Vault
                │                   └──→ Brig
                │
                ├── Sail
                └── Deck ──→ Crew

Server ──→ Crew        (/skill-name 快捷入口)

Sail, Deck, Vault, Brig ──→ Knot（共享类型）
```

- helm 直接使用 `*voyage.Voyage`，无中间接口
- vault 绑定 session 目录，不知道 session ID
- brig 绑定 WorkingDir，通过 Gate 接口分层裁决
- helm → crew：`crew.List()` 注入 L1 技能列表到 system prompt（未在图中画线）
- deck → crew：`skill` 工具调用 `crew.Instructions()` 加载 L2 全文
- server → crew：`/skill-name` 检测调用 `crew.Instructions()`
- server 每次请求独立：从磁盘恢复 → Run → 流式转发 → 持久化

## 数据流（一回合）

```
POST /session/prompt
  │
  ▼
handlePrompt
  ├─ 新消息路径：v.Phase = PhaseModel → Tale.AppendUser
  └─ Ask 回复路径：LoadState → 写入 ToolUseVerdicts
  │
  ▼
helm.Run() goroutine 循环：
  │
  ├─ PhaseModel ────────── PhaseRunnerModel
  │   ├─ Tale.BuildRequest → Compactor.Compact() → sail.Chat（SSE 流式消费）
  │   ├─ 无工具调用 → Tale.AppendAssistant + Phase = Done
  │   └─ 有工具调用 → Tale.AppendAssistant + Phase = ExecTools
  │
  ├─ PhaseExecTools ───── PhaseRunnerExecTools
  │   ├─ Evaluate → Allow/Deny/Ask
  │   ├─ Allow → tool.Execute → Tale.AppendTool
  │   ├─ Ask → Phase = Asking（return false；run.go 调 Asking runner 后退出）
  │   └─ 全部完成 → Phase = Model（循环）
  │
  ├─ PhaseAsking ──────── PhaseRunnerAsking（return true）
  │   └─ PendingMessages + SaveState（不写 vault） → goroutine 退出
  │
  └─ PhaseDone ────────── PhaseRunnerDone（return true）
      └─ checkpoint → goroutine 退出
```

所有事件通过 `<-chan knot.Event` 流式发出，server 以 SSE 格式转发给 CLI。

## 中断流程

### 运行中中断（Ctrl+C）

```
CLI: Ctrl+C → signal.Notify → cancel()
  → server: ctx.Done → helm.Run goroutine 退出
  → 不调 checkpoint，内存态丢弃
  → 下次 Resume 从 vault 加载 → 回到上一断点
```

### Asking 阶段终止（/interrupt）

```
CLI: /interrupt 输入 → POST /session/interrupt
  → server: DeleteState(sessionID) → 删 state.json
  → PendingMessages（user + assistant(tool_calls)）随文件丢失
  → 下次 Resume 从 vault 加载 → 回到断点前干净状态
```

参考 [DEC-039](/docs/decisions.md#dec-039signal-中断ctrlc-截获触发-agent-loop-终止) 和 [DEC-040](/docs/decisions.md#dec-040asking-延迟持久化phaseasking-不写-vaultpendingmessages-暂存-statejson)。

## 会话目录结构

```
~/.argo/sessions/{sessionID}/
  ├── session.json        ← Info 元数据（voyage 管理）
  ├── transcript.jsonl    ← 对话记录（vault 管理）
  ├── approvals.json      ← 用户审批记录（brig 管理）
  └── state.json          ← 控制流状态（voyage 管理）
```

## 代码目录

```
src/
├── knot/           共享类型（Message / Event / ToolUse / Config）
├── deck/           工具管理（Tool 接口 + 注册表 + 内置工具）
├── sail/           LLM 接入（Provider 接口 + ProviderOpenAI）
├── vault/          纯存储（Vault 接口 + FileVault + Record）
├── brig/           权限门控（Gate 接口 + Warden + RuleLoader）
├── voyage/         会话运行时聚合体（Voyage + Info + CRUD）
├── helm/           核心循环（状态机 + PhaseRunner + checkpoint）
├── wake/           日志轮转（LogWriter）
├── tale/           对话历史封装（Tale：Messages + Saved 游标）
├── crew/           技能管理（SkillInfo + Crew + 渐进式披露）
├── server/         无状态 HTTP 后端（chi + SSE）
├── cli/            CLI HTTP 客户端
└── argo-server/    HTTP 服务入口 main
```
