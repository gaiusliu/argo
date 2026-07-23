# 016：轻量 Blob Store — 文件变更快照与恢复

**日期**：2026-07-23 ~
**状态**：⏳ 待开始
**涉及模块**：deck（write/edit 包装）、voyage（Effect Journal 集成）、vault（新增 blobs 存储）

## 目标

为 Argo 提供 write/edit 工具的单文件变更快照与恢复能力。不依赖外部 git，使用纯 Go 标准库实现内容寻址存储。核心收益：断连/中断后能准确判断文件变更是否生效，支持安全回滚。

## 范围

### 包含

- **Blob Store 存储层**：`~/.argo/sessions/{id}/blobs/` 下的内容寻址存储，sha256 → zlib 压缩 → 按前 2 位分目录
- **Effect Journal**：`effects.jsonl` 独立日志，append-only，执行前先落盘
- **write/edit 执行包装**：工具执行前拍 before blob、执行后比 hash、写 Effect 记录
- **恢复判断**：刷入前扫描未 Committed 的 Effect，按 hash 判定 Applied / NoEffect / Conflict
- **安全回滚**：Applied 状态下用户手动触发 restore（before blob → 覆盖磁盘文件）
- **bash 策略**：默认 Unknown，中断后禁止自动重试，不拍快照

### 不含

- 任意 bash 命令的快照（无法预知变更文件）
- 整目录回滚
- diff 生成
- 跨轮多文件原子回滚
- Git 快照（方案 B，后续可选升级）

## 公共设计约束

- 零外部依赖，只用 Go 标准库（`compress/zlib`、`crypto/sha256`、`encoding/json`）
- Blob Store 去重：相同内容只存一份，通过 sha256 编码自动实现
- Effect Journal 独立于 transcript.jsonl：执行前必须落盘，不能随 checkpoint 批量写入
- 回滚不覆盖用户新修改：currentHash ≠ BeforeHash 且 ≠ AfterHash 时禁止自动回滚，标记为 Conflict

## 数据结构

### Blob Store

```
~/.argo/sessions/{sessionID}/
└── blobs/
    ├── a1/
    │   └── b2c3d4e5...          ← sha256 前 2 位分目录，后续为文件名
    └── f6/
        └── g7h8i9j0...
```

### EffectRecord

```go
type EffectRecord struct {
    EffectID   string `json:"effect_id"`
    TurnID     string `json:"turn_id"`
    ToolCallID string `json:"tool_call_id"`
    ToolName   string `json:"tool_name"`
    Path       string `json:"path"`
    BeforeHash string `json:"before_hash"`           // "" 表示文件本不存在
    AfterHash  string `json:"after_hash,omitempty"`  // 执行后填入
    Status     string `json:"status"`                // prepared | started | applied | no_effect | conflict
    StartedAt  int64  `json:"started_at"`
    FinishedAt int64  `json:"finished_at,omitempty"`
}
```

## 执行流程

```
1. 生成 effectID
2. 读取文件 → 算 sha256 → storeBlob(hash, content)
3. 写 effects.jsonl: status=prepared
4. 执行 tool.Execute()
5. 读取文件 → 算新 sha256 → 比 hash
6. 更新 effects.jsonl: status=applied | no_effect, after_hash
7. 才向 UI 发 EventToolUseDone
```

## 恢复流程

```
新一轮开始前，扫描 effects.jsonl 中 status != committed 的记录：
    currentHash == BeforeHash → NoEffect → 可安全重试
    currentHash == AfterHash  → Applied  → 禁止重试，用户可手动回滚
    两者都不等                  → Conflict → 标记，禁止变更工具
存在 Conflict → 锁定修改类工具，只允许读取/检查
```

## 整体开发状态

| 功能 | 状态 |
|------|------|
| Blob Store（sha256 + zlib + 目录存储） | ⏳ 待开始 |
| Effect Journal（effects.jsonl 读写） | ⏳ 待开始 |
| write Handler 包装 | ⏳ 待开始 |
| edit Handler 包装 | ⏳ 待开始 |
| 恢复判断与 Session Recovery Gate | ⏳ 待开始 |
| 用户回滚入口（CLI 命令 / TUI 按钮） | ⏳ 待开始 |
| bash Unknown 策略 | ⏳ 待开始 |

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-07-23 | 迭代文档生成 |
