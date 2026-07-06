# 005-brig-fix

**日期**：2026-07-05 ~ 2026-07-06
**状态**：✅ 已完成（被全量 v2 重写 + 后续代码审查修复取代）
**涉及模块**：brig、helm、knot
**来源**：code-review 对 004-brig-mvp 的审查

> ⚠️ **本文档描述的是对 brig v1 的 code-review 修复计划**。这些修复已执行，但随后 brig 被全量重写为 v2（见 [docs/modules/brig/design.md](../../modules/brig/design.md)）。
>
> 本文档不再反映当前代码状态，仅保留作为历史参考。

## 目标

修复 code-review 在 brig 模块中发现的 15 个 bug。核心问题是 **matchPattern 通配符语义不足**和**extractSubCommands 拆分与 deny 规则的匹配矛盾**——导致 deny、dangerCommands、sensitiveFiles 三大规则组大面积失效。

## 审核发现全貌

经过 9 角度代码审查（逐行扫描 / 删除行为审计 / 跨文件追踪 / Go 语言陷阱 / 门控逻辑正确性 / 复用 / 简化 / 效率 / 架构层级），共发现 15 个问题。
按根因归类为 4 个核心问题 + 3 个独立修复：

### 根因 1：matchPattern 通配符语义不足

**影响的 bug**：#1（sensitiveFiles 全部 22 条失效）、#2（dangerCommands 全部单语 pattern 失效）

`matchPattern` 仅支持精确匹配、`/*` 目录通配、`*` 后缀通配三种方式。缺失：

| 缺失的模式 | pattern 示例 | 影响的规则 |
|-----------|-------------|-----------|
| 前导通配 `*.ext` | `*.pem`、`*.key`、`*.pfx` | sensitiveFiles 的 6 条 |
| 中间通配 `*word*` | `*credentials*`、`*secret*`、`*token*` | sensitiveFiles 的 5 条 |
| 反向前缀 target以pattern开头 | `rm` 匹配 `rm -rf` | dangerCommands 全部 ~40 条 |

**manifest**：`read /app/.env` → extractPatterns 返回 `["/app/.env"]` → matchPattern(`"/app/.env"`, `".env*"`) → HasSuffix(`".env*"`, `"*"`) = true → prefix=`"."` → HasPrefix(`"/app/.env"`, `"."`) = false → 不匹配。sensitiveFiles 规则的 Ask 从未触发。

**修复**：重写 matchPattern，新增前导通配、中间通配、反向前缀三种匹配，共 6 条匹配路径。

### 根因 2：extractSubCommands 拆分破坏 deny 精确匹配

**影响的 bug**：#3（fork bomb）、#5（`rm -rf /`）、#6（`dd if=`）

denyRules 中写的是精确完整命令，但 extractSubCommands 把命令拆成了前 2 词的碎片，导致精确匹配永远失败：

| denyRule pattern | 实际命令 | extractSubCommands 产出 | matchPattern 结果 |
|-----------------|---------|------------------------|-------------------|
| `rm -rf /`（3 词） | `rm -rf /` | `["rm -rf"]`（2 词） | 不匹配 |
| `dd if=`（2 词） | `dd if=/dev/zero` | `["dd if=/dev/zero"]`（2 词含参数） | 不匹配 |
| `:(){ :\|:& };:` | `:(){ :\|:& };:` | `[":(){ :", ":& };:"]`（`|` `;` 拆分） | 不匹配 |

**修复**：denyRules 的匹配不走 extractSubCommands，直接用原始参数字符串做精确/前缀匹配。

### 根因 3：URL 解析失败时静默放行

**影响的 bug**：#4

`url="127.0.0.1:8080/admin"`（缺 scheme）→ `url.Parse` 将全部输入当 path → Host 为空 → extractPatterns 返回 nil → Evaluate 直接 Allow。SSRF 防护完全跳过。

**修复**：extractDomain 中补全 scheme 后重试。extractPatterns 中 domain 为空返回 Ask 而非 Allow。

### 根因 4：路径未规范化

**影响的 bug**：#8（denyRules 绕过 + 重复 Ask）

- `filepath.Dir(path)` 不调用 Clean/Abs，`./src/main.go` 和 `src/main.go` 产生不同 pattern
- denyRules 写的是完整路径（`/etc/shadow`），extractPatterns 返回父目录（`/etc`），不匹配

**修复**：extractPatterns 中对 write/edit 路径做 `filepath.Abs` + `filepath.Clean`。

### 独立修复

| Bug | 问题 | 修复 |
|-----|------|------|
| #7 | `go fmt` 被归类为"只读"Allow | 从 safeCommands 移到 dangerCommands |
| #9 | SSRF denyRules 仅 3 个地址 | 增加 IPv6、RFC 1918 私有段、链路本地 |
| #10 | 未知工具默认 Allow | default 分支改为 Ask |
| #11 | Restore 不去重 | 追加前检查是否已存在等价规则 |
| #12 | sensitiveFiles read/grep 重复 | 提取公共 pattern 列表循环生成 |
| #13 | denyRules write/edit 重复 | 合并为多工具规则 |
| #14 | 静态规则硬编码 | 本次不修，预留 `$ARGO_HOME/rules.json` 加载点 |
| #15 | 缺少 RemoveDynamic | 本次不修，待持久化方案确定后一并设计 |

## 修复分组

```
第一轮 — matchPattern 重写（改动集中在 brig.go）：
├── 根因 1：matchPattern 新增 3 种匹配路径
│   影响文件：brig.go（~25 行改动）
│   同时修复：#1（sensitiveFiles）、#2（dangerCommands）

第二轮 — Evaluate + extractPatterns 重构：
├── 根因 2：denyRules 用原始参数匹配
│   同时修复：#3（fork bomb）、#5（rm -rf /）、#6（dd if=）
├── 根因 3：extractDomain 补全 scheme
│   同时修复：#4（URL 解析静默放行）
├── 根因 4：路径规范化
│   同时修复：#8（路径不一致）
│   影响文件：brig.go + webfetch.go（~30 行改动）

第三轮 — 低风险独立修复：
├── #7：go fmt 移入 dangerCommands（1 行）
├── #9：SSRF 补充地址 + IPv6/私有段（~12 行）
├── #10：default Ask（1 行 + 确认语义影响）
├── #11：Restore 去重（~5 行）
├── #12：sensitiveFiles 去重（~15 行）
├── #13：denyRules write/edit 去重（~4 行）
│   影响文件：deny.go + brig.go
```

> 建议第一轮 + 第二轮放在一个 PR，因为都涉及 brig.go 核心逻辑改动。第三轮独立 PR。

## 公共设计约束

- 不改变 brig 的接口（Engine/Evaluate/Append/Snapshot/Restore 签名不变）
- 不改变 RunLoop 签名
- 不改变 Rule 结构体
- matchPattern 语义向后兼容——现有通过的测试继续保持通过
- 每个函数不超过 50 行
- 无匹配规则时的默认行为从 Allow 改为 Ask（#10），需要确认调用方兼容

## 整体开发状态（历史记录）

> 以下功能已在 v1 代码上完成修复，随后 v1 被废弃、v2 重写取代。当前代码状态见 `src/brig/` 和 `docs/modules/brig/design.md`。

| 功能 | 状态 | 备注 |
|------|------|------|
| matchPattern 重写（3→6 种） | ✅ | 已重写，并融入 v2 设计 |
| denyRules 原始参数匹配 | ✅ | 已重写为 resolveStaticRules 首命中机制 |
| extractDomain scheme 补全 | ✅ | 已重写为 scopeKey 域名提取 |
| 路径规范化 | ✅ | filepath.Abs + Clean |
| 独立修复 #7 #9 #10 #11 #12 #13 | ✅ | go fmt 归类、SSRF 扩展、default Ask、Restore 去重、sensitiveFiles 去重 |
