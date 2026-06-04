# deck 行为规约

## 概述

deck 负责工具的注册、可用性判定、发现、使用和结果后处理。对上（helm）暴露统一的工具查询和调用接口，对下屏蔽内部工具、外部 CLI 工具和 MCP 工具的差异。

## 三大工具来源

### 内部工具

```
init() + [barrel 触发](../glossary.md#barrel-触发) 自注册 → 编译时嵌入 → ToolRegistry
```

### 外部 CLI 工具

```
~/.argo/tools.json → cli 列表 → 启动时扫描 → ToolRegistry

每个条目：{ "name": "gh", "description": "..." }
- name：LLM 调用名 + 默认可执行文件名
- description：LLM 判定何时使用的唯一依据
- 参数 schema 不写入配置，LLM 通过 bash 执行 --help 现场获取
```

### MCP 工具

```
~/.argo/tools.json → mcp 列表 → 启动时 spawn 子进程 → tools/list 获取清单 → ToolRegistry

每个条目：{ "name": "github", "command": ["npx", "-y", "@anthropic/mcp-server-github"] }
- name：服务器标识，工具注册名 = mcp__<name>__<toolName>
- command：启动命令数组（stdio 子进程模式）
- 参数 schema 由服务器通过 tools/list 协议提供，配置里不写
```

### tools.json 格式

```json
{
  "cli": [
    { "name": "gh", "description": "GitHub CLI — 管理 issue、PR、仓库" },
    { "name": "docker", "description": "Docker 容器管理" }
  ],
  "mcp": [
    { "name": "github", "command": ["npx", "-y", "@anthropic/mcp-server-github"] }
  ]
}
```

### CLI vs MCP 的选择

| | CLI | MCP |
|------|-----|-----|
| token 消耗 | 低（schema 不进 prompt，--help 按需读） | 高（每个工具带完整 JSON Schema） |
| 权限管理 | 做不到（参数是字符串，无法事前拦截） | 能做（参数是结构化 JSON，调用前可判定） |
| 启动成本 | 无（用时才 exec） | 有（启动时起子进程，保持常驻） |
| 配置字段 | name + description | name + command |

CLI 和 MCP 之间不造中间层。CLI 做不了权限管理但省 token，MCP 能管权限但贵 token。用户按需选择，配置文件不预设立场。

---

## 五大能力

```
注册 ──→ 可用性判定 ──→ 发现 ──→ 使用 ──→ 结果后处理
```

---

### 能力 1：工具注册

```
Argo 启动时，三来源各自完成注册，全部写入统一 ToolRegistry：

  ┌─── 内部工具
  │     argo/deck/tools/ 下各 Go 包通过 init() 自注册
  │     barrel 文件 import 触发所有 init()
  │     → ToolDef 由开发者手写，Conditions 为空（编译时保证存在）
  │
  ├─── 外部 CLI
  │     LoadConfig() 读 ~/.argo/tools.json 的 cli 列表
  │     对每条 "{ name, description }" 构造 ToolDef
  │     Conditions 自动生成 [{Kind:"binary", Key: name}]
  │     execute 自动生成为通用 exec.Command 适配器
  │
  └─── MCP
        LoadConfig() 读 ~/.argo/tools.json 的 mcp 列表
        对每条 "{ name, command }" spawn 子进程
        tools/list 协议获取工具清单 → 逐个构造 ToolDef
        Conditions 不写，可用性由 MCPManager.IsAlive() 判定

同名冲突：内部 > 外部。CLI 和 MCP 平权——启动时报错，不做静默覆盖。
```

### 能力 2：可用性判定

声明式条件 + 来源分流。MVP 支持两种条件类型：`binary`（可执行文件在 PATH）和 `env`（环境变量已设置）。

```
EvaluateAvailability(ctx)
  │
  ├── 遍历 ToolRegistry.entries
  │     │
  │     ├── MCP 工具 → mcpManager.IsAlive(serverName)
  │     │     ├── 子进程存活 → visible
  │     │     └── 子进程退出 → hidden，诊断信息: "serverName disconnected at <time>"
  │     │
  │     └── 内部 / CLI 工具 → evaluateConditions(ctx, def.Conditions)
  │           ├── len(Conditions)==0 → 无条件 → visible（内部工具默认）
  │           ├── 遍历 Conditions:
  │           │     ├── Kind="binary" → exec.LookPath(Key)
  │           │     │     ├── 找到 → visible
  │           │     │     └── 未找到 → hidden，诊断: "binary 'gh' not in PATH"
  │           │     └── Kind="env" → os.Getenv(Key) != ""
  │           │           ├── 已设置 → visible
  │           │           └── 未设置 → hidden，诊断: "env 'GITHUB_TOKEN' not set"
  │           └── 任意条件失败 → hidden
  │
  ▼
  每轮对话前 List() 只返回 visible 的工具

MCP 子进程存活检测：cmd.Wait() + 后台 goroutine
  ├── Start() 时启动，非主动 ping，被动响应进程退出
  └── 进程退出时 Wait() 返回 → goroutine 更新 connected→disconnected
```

### 能力 3：工具发现

```
ToolRegistry.list(agentCfg)
  │
  ├── Step 1: 可用性滤除
  │     去掉所有 visible=false 的条目
  │
  ├── Step 2: 权限滤除
  │     按 agentCfg 去掉不允许的工具（MVP 先全量放行，留 hook）
  │
  ▼
  Step 3: 排序输出
     按名称排序（保证缓存稳定性）
     → ToolDef[] → helm 注入 system prompt
```

> `agentCfg` 是尚未定义的占位符，预留用于多 Agent 场景下不同 Agent 拥有不同工具权限。MVP 只有一个 Agent 实例，`List()` 实际调用无需参数，该 hook 暂不生效。

同名冲突：内部 > CLI（注册阶段报错）。CLI 和 MCP 平权——用户自己配置的信任级别相同，同名冲突启动时报错让用户解决。不做静默覆盖。

### 能力 4：工具使用

不做独立参数校验步骤。内部/MCP 工具的 schema 校验由工具 handler 内部自行处理，外部 CLI 工具无法预先校验。所有错误——类型错误、参数错误、业务错误——统一走 ToolResult.Status="error" 返回，由 LLM 解读并自我修正。

分区：每个工具声明静态 `ConcurrencyMode`（concurrent | sequential）。读操作为 concurrent（不修改状态），写操作为 sequential（避免并发踩踏）。工具作者预设，不做运行时判断。

```
LLM 返回 tool_use 块
  │
  ├── 名称查找
  │     精确匹配 ToolRegistry
  │     未找到 → ToolResult{Status:"error", Output:"未知工具: xxx"}
  │
  ├── 权限检查
  │     PreToolUse hook（可阻断）
  │     阻断 → ToolResult{Status:"error", Output:"工具 xxx 未被允许"}
  │
  ├── 分区
  │     按 ConcurrencyMode 拆组：
  │       ├── concurrent → 并行组（goroutine + sync.WaitGroup）
  │       └── sequential  → 串行组（for 逐个执行）
  │
  ├── 执行调度
  │     ├── builtin → ref.Builtin(ctx, params)
  │     ├── cli     → exec.Command(binary, args...)
  │     └── mcp     → mcpManager.CallTool(server, tool, params)
  │
  ▼
  工具返回 (stdout + stderr + exit_code)
    │
    ▼
  包装为 ToolResult
    ├── Status: "success" | "error" | "timeout"
    ├── Output: stdout（成功时）| stderr（失败时，原样交给 LLM 自行修正）
    └── Metadata: exit_code, duration
    │
    ▼
  注入会话上下文 → helm 下一轮 → LLM 看到错误自行调整
```

### 能力 5：结果后处理

MVP 保留头 40% + 尾 60%。不做持久化文件、不做聚合预算。

```
ToolResult
  │
  ├── 截断（单工具字符上限 MaxOutputChars，默认 50K，可配置）
  │     if len(Output) > MaxOutputChars:
  │       head = Output[:MaxOutputChars * 40%]
  │       tail = Output[末尾 MaxOutputChars * 60%]
  │       mid_omitted = 原始长度 - MaxOutputChars
  │       Output = head + "\n\n[... 输出被截断，省略 mid_omitted 字符 ...]\n\n" + tail
  │
  ├── 错误格式化
  │     exit_code ≠ 0 → 包装 stderr 为统一 ToolResult，原样交给 LLM
  │
  ▼
  注入会话上下文 → helm 进入下一轮
```

## 边界情况

- 工具重名：内部 > 外部。CLI 和 MCP 同名冲突启动报错，不做静默覆盖
- MCP 服务器断连：标记该服务器所有工具为 hidden，定时重连
- MCP 服务器启动失败：记录诊断信息，该服务器工具不加入注册表
- 工具执行超时：按工具自身 timeout 配置终止，返回超时错误
- 工具未注册：返回"未知工具"错误消息（帮助 LLM 自我纠正）
