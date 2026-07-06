# brig

## 概述

brig 是 Argo 的权限审批门控模块。在 helm.RunLoop 执行工具前，brig 按规则判定 Allow/Ask/Deny，决定工具调用是直通执行、需用户确认、还是直接拒绝。brig 不依赖 helm/deck/sail，只依赖 `knot.ToolUse`。

## 核心概念

| 概念 | 说明 |
|------|------|
| Verdict | 三态裁决：Allow（直通执行）/ Ask（征求用户确认）/ Deny（直接拒绝，不可绕过） |
| Rule | 统一结构 `{Tool, Pattern, Action}`，仅用于静态规则 |
| ToolCall | 工具调用指纹 `{ToolName, RawParam, WorkingDir}`——动态审批的精确匹配键 |
| Engine | 规则引擎，持有静态规则列表 + 用户审批 map + 互斥锁，每个 session 一个独立实例 |
| Static Rule | 代码内置规则（deny 库 + 命令分类表），NewEngine 时加载，只读不修改 |
| Dynamic Approval | 用户确认后以精确 ToolCall 指纹存入 `userApprovals` map，同指纹再次出现自动 Allow |
| AskUser | 用户确认回调 `func(tu ToolUse, reason string) (bool, error)`，定义在 knot，由调用方注入 |

## 流程

### 1. 静态规则求值（resolveStaticRules）

静态规则按 `loadStaticRules()` 的固定顺序排列（5 层），遍历时**首个命中即返回**，不再继续：

```
规则顺序（即优先级）：
  1. denyRules           → Deny    破坏性操作 / SSRF / 系统文件写入
  2. safeCommands        → Allow   纯读取命令
  3. dangerCommands      → Ask     可能修改状态的命令
  4. interpreterCommands → Ask     脚本解释器
  5. sensitiveFiles      → Ask     敏感文件读取/搜索

匹配方式：matchPattern(rule.Pattern, tc.RawParam)，支持 6 种通配符
无命中：默认返回 Ask
```

关键设计：(a) deny 规则放在最前面，确保命中即 Deny，不被后续 Allow 覆盖——此顺序由 `loadStaticRules` 的 append 顺序保证，`resolveStaticRules` 和 `loadStaticRules` 的注释中均明确声明了这一依赖；(b) "首个命中"语义明确，不依赖"最后命中者胜"的隐式顺序假设；(c) 同一 raw 不会同时命中 Allow 和 Ask——如果命中了 safeCommands 的 Allow，就不会再走到 dangerCommands 的 Ask。

### 2. Evaluate 完整流程

```
helm 收到 ToolUse
  → eng.Evaluate(tu)
      │
      ├─ getRawParam(tu)                  ← 提取原始参数字符串（命令/路径/URL）
      │   └─ fieldFromArgs(name, params)  ← 查 toolParamKeys 表，按工具名取参数字段
      │
      ├─ scopeKey(tu.Name, raw)           ← 审批粒度转换（web_fetch → 域名，其余 → 原值）
      │
      ├─ 构造 ToolCall{ToolName, RawParam, WorkingDir}
      │
      ├─ resolveStaticRules(staticRules, tc)  ← 遍历 5 层静态规则，首命中即返回
      │   ├─ 命中 Deny  → 返回 Deny（不可绕过）
      │   ├─ 命中 Allow → 返回 Allow（免审批）
      │   └─ 无命中 / 命中 Ask → 返回 Ask
      │
      └─ verdict == Ask ？
          ├─ tc 在 userApprovals 中  → Allow（用户之前批准过同一指纹）
          └─ tc 不在                  → Ask（弹出确认框）
```

### 3. Approve：精确指纹匹配的动态审批

用户确认后，RunLoop 调用 `eng.Approve(tu)`：加锁 → 取 `getRawParam` + `scopeKey` 构造 `ToolCall` 指纹 → 存入 `userApprovals` map。

**不做泛化**——不将 `rm -rf /tmp/test` 概括为 `rm -rf*`，只有**完全相同的 ToolCall 指纹**（工具名 + 审批键 + 工作目录）再次出现时才自动 Allow。唯一例外：`web_fetch` 通过 `scopeKey` 将完整 URL 转为域名作为审批键，与 pi/opencode 的 `allowedDomains` 一致。这是一种保守的安全策略——只记住"用户同意的那个确切操作"。

### 4. RunLoop 集成

brig 在 helm 工具执行前做门控：`Evaluate` 返回 Allow → 执行；Ask → 弹出确认框，用户同意则 `Approve` 后执行；Deny → 返回拒绝原因。结果注入 `state.Messages` 进入下一轮 LLM。

### scopeKey：审批粒度转换

`scopeKey` 在 `getRawParam` 之后、构造 `ToolCall` 指纹之前调用，决定审批的匹配粒度。`web_fetch` 工具将完整 URL 转为小写域名，其余工具保持原值不变：

| 工具 | getRawParam 输出 | scopeKey 输出 | 审批粒度 |
|------|-----------------|--------------|---------|
| bash | `"rm -rf /tmp/test"` | `"rm -rf /tmp/test"` | 精确命令 |
| write/edit | `"/src/brig/file.go"` | `"/src/brig/file.go"` | 精确路径 |
| web_fetch | `"https://api.github.com/repos/foo"` | `"api.github.com"` | 域名级 |

此函数是未来扩展审批粒度的唯一入口点——如需对 write 做目录级审批，只需在此加一条 case。

## 静态规则清单

5 层静态规则，定义在 [deny.go](../../src/brig/deny.go)，加载顺序即优先级：

| 层 | 规则组 | 动作 | 示例 Pattern |
|----|--------|------|-------------|
| 1 | denyRules | Deny | `rm -rf /`、`mkfs`、`127.0.0.1`、`/etc/shadow` |
| 2 | safeCommands | Allow | `ls`、`cat`、`git log`、`go vet` |
| 3 | dangerCommands | Ask | `rm`、`curl`、`git push`、`kill` |
| 4 | interpreterCommands | Ask | `python`、`node`、`go run`、`eval` |
| 5 | sensitiveFiles | Ask | `.env*`、`*.pem`、`*secret*`、`id_rsa*` |

**注意**：
- 第 2 层 safeCommands 只会 Allow 纯读取类命令（如 `git log`），写操作（如 `git push`）由第 3 层 dangerCommands 拦截为 Ask。
- 第 1 层 denyRules 中 `rm -rf /` 用精确匹配（matchPattern case 1），而 dangerCommands 中 `rm` 用反向前缀匹配（case 6）拦截所有以 `rm` 开头的命令。
- 未知工具（静态规则无匹配）默认 Ask。

## matchPattern 七种匹配方式

定义在 [brig.go](../../src/brig/brig.go)，按优先级依次尝试，命中即返回：

| # | 方式 | Pattern 示例 | 匹配行为 | 典型用途 |
|---|------|-------------|---------|---------|
| 1 | 精确匹配 | `rm -rf /` | `target == pattern` | deny 精确命令 |
| 2 | 中间通配 | `*credentials*` | target 任意位置包含 word | 敏感文件名 |
| 3 | 前导通配 | `*.pem` | target 以该后缀结尾 | 凭证文件后缀 |
| 4 | 目录通配 | `dir/*` | target 在目录下（要求 pattern 长度 > 2，防止 `/*` 匹配全部绝对路径） | 同目录批量 Allow |
| 5 | 后缀通配 | `.env*` | target（或 basename）以 prefix 开头 | 敏感文件前缀 |
| 6 | 反向前缀 + 边界检查 | `rm` | target（或 basename）以 pattern 开头，且 pattern 后必须为**边界**（空串/空格/tab/路径分隔符 `/` `\`） | 危险命令带任意参数；防止 `"rm"` 误匹配 `"rmdir"` |

case 6 通过 `isBoundary` 辅助函数检查前缀匹配后的剩余文本首字符——必须为空、空格、tab、`/` 或 `\`。防止 `"rm"` 误匹配 `"rmdir"`，同时保留 `"C:\Windows\System32"` 匹配子目录的能力。

## getRawParam 与 toolParamKeys

`getRawParam` 从工具参数中提取原始输入字符串供规则匹配。上游（sail adapter）已在 `flush()` 中将 API 返回的 JSON arguments 解析为扁平 `map[string]any`，因此 `getRawParam` 无需再做嵌套 JSON 解包。字段映射由 `toolParamKeys` 表维护（`fieldFromArgs` 查表取参数值），新增工具只需追加一行映射。

## sensitiveFilePatterns

敏感文件规则由 `sensitiveFilePatterns` 统一维护——11 个匹配模式对 `read` 和 `grep` 通过 `init()` 循环生成，避免两组规则重复定义。新增敏感文件类型只需在 `sensitiveFilePatterns` 中追加一项。

## 行为合约

### 规则引擎

- 静态规则在 NewEngine() 时加载一份副本，所有 session 共享同一来源但各自独立
- 静态规则求值：5 层固定顺序遍历，**首个命中即返回**（非"最后命中者胜"）
- 动态审批：用户确认后以精确 ToolCall 指纹存入 `userApprovals` map（非泛化 pattern），同指纹再出现自动 Allow
- `userApprovals` 由 mu 保护并发写入，读取无锁（Go map 并发读安全，写由 mu 保护）

### 会话隔离

- 每个 session 持有独立 Engine 实例，动态审批不跨 session 共享
- Snapshot() / Restore() 预留持久化接口，当前为空桩

### brig 边界

- brig 只依赖 knot，不依赖 helm/deck/sail
- Evaluate 返回 `(Verdict, reason)`，不再返回 pattern
- AskUser 定义在 knot，实现在调用方——brig 只判定，不参与用户交互
- 静态规则 5 层固定顺序确保 deny > allow 语义，无需额外优先级计算
