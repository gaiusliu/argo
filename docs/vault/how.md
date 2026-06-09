# vault 实现设计

## 概述

vault 是 Argo 的统一存储层，提供 `Memory` 接口的默认文件系统实现。两层职责：会话级 transcript.jsonl + 框架级 profile.md / AGENTS.md。通过包级 `Register` / `Load` 函数支持 Memory 实现的热插拔。

## 内部结构

```
vault/
├── vault.go        // Memory 接口 + Register / Load 包级函数
├── file.go         // FileVault 默认文件系统实现
├── transcript.go   // transcript.jsonl 7 种 type 的 record 写入
├── profile.go      // profile.md 读写 + 预算控制 + 变更跟踪 / AGENTS.md 只读加载
├── types.go        // MemoryEntry, RecallResult
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
	Content string // profile.md 写入内容
}

type RecallResult struct {
	Profile string // profile.md 全文
	Agents  string // AGENTS.md 全文
}

// 包级函数 — 管理 Memory 实现的热插拔
// 不暴露额外类型，vault. 前缀已在语义上约束了"记忆存储注册"的领域
func Register(name string, factory func(cfg Config) (Memory, error))
func Load(name string, cfg Config) (Memory, error)
```

调用方示例：

```go
vault.Register("file", func(cfg vault.Config) (vault.Memory, error) {
    return NewFileVault(cfg.SessionDir)
})
mem, err := vault.Load("file", cfg)
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
  vault.Load(agent.cfg.MemoryBackend, cfg) → Memory 实例

每轮结束后（helm 调）:
  vault.Append(newMessages)
    → ~/.argo/sessions/{id}/transcript.jsonl 追加

LLM 发现用户偏好时（LLM 工具调用）:
  vault.Store(MemoryEntry{Type:"profile", Content:"用户偏好 Go 方案", Source:"llm"})
    → profile.md 追加一条，写入前检查预算

LLM 更新用户画像时（LLM 工具调用）:
  vault.Store(MemoryEntry{Type:"profile", Content:"用户偏好 Rust 方案", Reason:"用户说以后新项目都用 Rust"})
    → profile.md 写入或更新一条，写入前检查 5,000 字符预算

会话开始时（helm 调）:
  vault.Recall()
    → 返回 profile.md + AGENTS.md 全文
    → helm 注入 system prompt
```

## 目录结构

```
~/.argo/sessions/{session_id}/
├── transcript.jsonl       // 对话记录（7 种 type 的 JSONL record）
├── profile.md             // 用户画像/偏好（LLM 维护，5K 字符预算）
└── AGENTS.md              // 硬性约束（用户管理，LLM 只读）
```
