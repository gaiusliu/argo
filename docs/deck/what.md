# deck 行为规约

## 概述

deck 负责工具的注册、可用性判定、发现、使用和结果后处理。对上（helm）暴露统一的工具查询和调用接口，对下屏蔽内部工具、外部 CLI 工具和 MCP 工具的差异。

## 三大工具来源

### 内部工具

```
init() + barrel 自注册 → 编译时嵌入 → ToolRegistry
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
工具来源
  ├── 内部工具 → init() 自注册 + barrel 触发 → 编译时嵌入
  │              Conditions 不写则默认永远可见
  │
  ├── 外部 CLI  → 读 tools.json → 构造 ToolDef + 自动生成 Conditions
  │              [{ "name": "gh", "description": "..." }]
  │              → Conditions: [{Kind:"binary", Key:"gh"}]  // 自动生成
  │              → execute: 通用 exec.Command 适配器
  │
  └── MCP 工具 → 读 tools.json → spawn 子进程 → tools/list → 构造 ToolDef
                 Conditions 不写，可用性由 MCPManager.IsAlive() 判定
                    │
                    ▼
              写入 ToolRegistry（按名称索引，内部 > MCP 同名优先）
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
ToolRegistry.list(agent, model)
  │
  ├── 滤除 hidden 工具
  ├── 按 agent 权限二次过滤
  │
  ▼
  输出 ToolDef[]（name + description + JSON Schema）
    → helm 注入 system prompt
```

### 能力 4：工具使用

```
LLM 返回 tool_use 块
  │
  ├── 名称查找（精确匹配）
  ├── 参数校验（JSON Schema 验证）
  ├── 权限检查（MCP 可做；CLI 做完事日志审计，不做事前拦截）
  │
  ├── 分区执行：
  │     ├── 并发安全工具 → 并行执行（上限 N）
  │     └── 非并发安全工具 → 串行执行
  │
  ├── 执行调度：
  │     ├── 内部工具 → Go 函数调用
  │     ├── 外部 CLI  → exec.Command
  │     └── MCP 工具 → MCP client.(tools/call)
  │
  ▼
  返回 ToolResult（输出 + 状态 + 元数据）
```

### 能力 5：结果后处理

```
ToolResult
  │
  ├── 截断：超长输出按 token 预算裁剪
  ├── 格式化：统一包装为 LLM 可消费的消息结构
  ├── 错误转换：异常/堆栈 → LLM 可理解的错误描述
  │
  ▼
  注入会话上下文 → helm 进入下一轮
```

## 边界情况

- 工具重名：内部工具 > CLI 工具 > MCP 工具
- MCP 服务器断连：标记该服务器所有工具为 hidden，定时重连
- MCP 服务器启动失败：记录诊断信息，该服务器工具不加入注册表
- 工具执行超时：按工具自身 timeout 配置终止，返回超时错误
- 工具未注册：返回"未知工具"错误消息（帮助 LLM 自我纠正）
