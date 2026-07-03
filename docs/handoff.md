# Handoff — WebFetch / WebSearch 工具实现

## 背景

001-deck-mvp 迭代原本只有 8 个内置工具（bash、read、write、edit、grep、glob、list、lookup），全是本地文件系统操作。本次新增 `web_fetch` 和 `web_search` 两个网络工具，使 MVP 具备基础的信息获取能力。

## 已完成的代码

### `src/deck/builtin_tool.go`

新增约 220 行，4 个函数 + 3 个类型 + 3 个包级常量：

| 函数 | 功能 |
|------|------|
| `webFetchHandler` | HTTP GET 获取 URL，MIME 分发（图片→base64、HTML→去标签、其余透传），30s 超时，5MB 上限 |
| `webSearchHandler` | `fnvHash(query) % 2` 分流到 Parallel 或 Exa MCP 端点搜索，零配置零 API Key |
| `callSearchMCP` | 直接 JSON-RPC 2.0 `tools/call` 请求，不引入 MCP SDK，纯标准库 |
| `stripHTML` | 正则去标签，跳过 script/style/noscript/iframe/object/embed 块 |

| 常量 | 值 |
|------|-----|
| `webFetchMaxSize` | 5MB |
| `webFetchTimeout` | 30s |
| `searchRPCTimeout` | 25s |

| 类型 | 用途 |
|------|------|
| `mcpRPCParams` / `mcpRPCRequest` / `mcpRPCResult` | JSON-RPC 2.0 请求/响应结构体 |

## 全部 21 条决策

`docs/decisions.md`（DEC-001 ~ DEC-021）。本次新增 DEC-012 ~ DEC-021：

| 编号 | 主题 |
|------|------|
| DEC-012 | WebFetch MVP 不限制跨 host 重定向 |
| DEC-013 | WebFetch MVP 不设置 User-Agent |
| DEC-014 | WebFetch 固定输出纯文本，不做 HTML→Markdown 转换 |
| DEC-015 | WebSearch MVP 只解析 JSON 响应，不处理 SSE |
| DEC-016 | WebSearch MVP 只暴露 query 参数 |
| DEC-017 | deck MVP 无权限/授权机制（**后续最高优先级**） |
| DEC-018 | WebSearch 使用免费 MCP 端点（Parallel + Exa），不自建爬虫 |
| DEC-019 | WebSearch 直接 JSON-RPC 调用，不引入 MCP SDK |
| DEC-020 | WebSearch 分流使用 query hash 而非 session ID |
| DEC-021 | WebFetch MIME 分发策略（image→base64、HTML→去标签、其余透传） |

## 迭代文档

- `docs/iterations/001-deck-mvp/main.md`：范围更新为 10 个工具，状态 ✅ 完成
- `docs/modules/deck/design.md`：内置工具集表格新增 `web_fetch` 和 `web_search`

## 关键设计选择（非决策层面的理解要点）

1. **Parallel/Exa 双端点**：两者均为免费 MCP（无需 API Key 即可调用），国内无同等替代
2. **不引入 MCP 协议栈**：参考 KiloCode `mcp-websearch.ts`（97 行无 SDK），直接 HTTP POST JSON-RPC
3. **不伪装浏览器 UA**：Go `net/http` 默认指纹不做伪装，避免 TLS 指纹与 UA 不匹配触发 Cloudflare 欺骗惩罚
4. **工具名用下划线**：`web_fetch`、`web_search`（非 `webfetch`），handler 名用 MixedCaps（`webFetchHandler`、`webSearchHandler`）

## 待办（按优先级）

1. **权限/授权机制**（DEC-017）—— deck 或 helm 层引入 `ctx.ask` 式确认，参考 OpenCode `ctx.ask({permission, patterns, always})`
2. 在 Tool 接口扩展 session 上下文 → WebSearch 改为 session-based 分流（DEC-020）
3. SSE 响应格式容错解析（DEC-015）
4. WebFetch 引入 `CheckRedirect` 跨 host 安全限制（DEC-012）
5. HTML→Markdown 转换——候选库 `github.com/JohannesKaufmann/html-to-markdown`（DEC-014）
6. WebSearch 暴露 `type`、`numResults` 等参数（DEC-016）

## 项目上下文

- Go 代码规范：`go-styleguide/`
- commit message 中文描述，不使用 Co-Authored-By
- 先说明想法征求同意，再修改代码

## Suggested Skills

无特定 skill 建议。如需扩展 WebFetch/WebSearch，直接参考 `docs/decisions.md` 中 DEC-012 ~ DEC-021。
