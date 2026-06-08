# vault 实现设计

## 概述

vault 是 Argo 的统一存储层，提供 `Memory` 接口的默认文件系统实现。两层职责：会话级 transcript.jsonl + 框架级 profile.md / rules.md。支持通过注册表切换后端。

## 内部结构

```
vault/
├── vault.go        // Memory 接口 + BackendRegistry 后端注册表
├── file.go         // FileVault 默认文件系统实现
├── transcript.go   // transcript.jsonl 追加写入
├── profile.go      // profile.md / rules.md 读写 + 预算控制
├── types.go        // MemoryEntry, BackendRegistry
└── vault_test.go
```

## 核心类型

```go
// Memory — helm 定义，vault 提供实现
type Memory interface {
    Append(ctx context.Context, messages []Message) error
    Recall(ctx context.Context) (*RecallResult, error)
    Store(ctx context.Context, entry MemoryEntry) error
}

type MemoryEntry struct {
    Type    string // "profile" | "rule"
    Content string
    Source  string // "llm" | "user"
}

type RecallResult struct {
    Profile string // profile.md 全文
    Rules   string // rules.md 全文
}

// BackendRegistry — 支持自定义后端热插拔
type BackendRegistry struct { ... }
func Register(name string, factory func(cfg Config) (Memory, error))
func Load(name string, cfg Config) (Memory, error)
```

## 函数签名

```go
// FileVault — 默认文件系统实现
type FileVault struct { ... }
func NewFileVault(sessionDir string) *FileVault

func (v *FileVault) Append(ctx context.Context, messages []Message) error
func (v *FileVault) Recall(ctx context.Context) (*RecallResult, error)
func (v *FileVault) Store(ctx context.Context, entry MemoryEntry) error

// budgetCheck — profile.md 超出预算时返回 true
func budgetCheck(path string, maxChars int) bool
```

## 数据流

```
启动时:
  vault.LoadBackend(agent.cfg.MemoryBackend) → Memory 实例

每轮结束后（helm 调）:
  vault.Append(newMessages)
    → ~/.argo/sessions/{id}/transcript.jsonl 追加

LLM 发现偏好/约束时（LLM 工具调用）:
  vault.Store(MemoryEntry{Type:"profile", Content:"用户偏好 Go 方案", Source:"llm"})
    → profile.md 追加一条，写入前检查预算

会话开始时（helm 调）:
  vault.Recall()
    → 返回 profile.md + rules.md 全文
    → helm 注入 system prompt
```

## 目录结构

```
~/.argo/sessions/{session_id}/
├── transcript.jsonl       // 对话记录（4 字段 JSONL）
├── profile.md             // 用户画像/偏好（LLM 维护，5K 字符预算）
└── rules.md               // 硬性约束（用户 + LLM 共建，用户手写永不淘汰）
```
