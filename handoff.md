# Handoff — wake 日志模块 + CLI REPL + 集成联调

## 背景

本会话完成了三件事：新建 wake 日志模块、新建 CLI REPL、端到端集成联调（DeepSeek V4 Pro），修复了若干协议兼容和 UI 竞争问题。

## 本次新增

### wake 日志模块（`src/wake/writer.go`）

自定义 `LogWriter`（实现 `io.Writer`），按日期+大小自动轮转，零外部依赖。喂给 `slog.TextHandler`，各模块直用 `slog.Info/Debug/Error`。

- 文件命名：`wake-2006-01-02.log`，同日超额切 `.001`、`.002`…
- 轮转策略：跨天优先，同日超过 MaxSize 追加序号
- 启动时扫描续接当日文件、清理过期日志

### CLI REPL（`src/cli/main.go`）

交互式 TUI，`argo>` 提示符等待输入，支持多行（`\` 续行）、`/exit` 退出。事件循环串行消费 13 种事件类型，文本/思考/工具状态实时展示。

辅助函数：
- `confirmTool` — 独立函数，通过 stdin 询问用户确认
- `parseLevel` — 配置字符串 → slog.Level

### CLI 辅助函数（`askCLI`、`parseLevel`、`confirmTool`）

`askCLI` 已改为返回 `knot.AskResult`（非 `bool`），对接 EventAsk 机制。

## 关键架构改动

### EventAsk 事件驱动（替代 AskUser 回调）

`knot/types.go` 新增 `EventAsk` 事件类型 + `AskResult`（`AskAllowed`/`AskDenied`）。`RunLoop` 签名从 `(ctx, state, ask, eng)` 简化为 `(ctx, state, eng)`。helm 发 `EventAsk` 后阻塞等 `AskReply` channel，cli 消费者串行处理确保 UI 不交错。

### Message 扩展 tool_call 关联

`knot/types.go` 新增 `ToolCallDetail` 类型，`Message` 加 `ToolCallID`（role=tool 用）和 `ToolCalls`（role=assistant 用）。`sail/adapter.go` 的 `openAIMsg` 对应加了 `tool_call_id` 和 `tool_calls` JSON 字段。满足 DeepSeek 严格 OpenAI 协议要求。

### getRawParam 迁移到 knot

从 `brig/brig.go` 搬到 `knot/types.go`，导出为 `GetRawParam`。cli 可直接引用，不再依赖 brig。

### SSE Content-Type 解析

`deck/builtin_tool.go` — `callSearchMCP` 按 `Content-Type: text/event-stream` 剥 SSE 壳（`\n\n` 切事件 → 扫 `data:` 行拼接），其余 Content-Type 直解 JSON。

### base_url 不做路径拼接

用户从官方文档 copy-paste 完整端点 URL，`adapter.go` 不做任何拼接。

## 配置

`~/.argo/config.json` 当前配置 DeepSeek V4 Pro：

```json
{
  "sail": {
    "model": "deepseek-v4-pro",
    "provider": {
      "deepseek": {
        "env": ["DEEPSEEK_API_KEY"],
        "options": { "base_url": "https://api.deepseek.com/v1/chat/completions" },
        "models": { "deepseek-v4-pro": { "name": "deepseek-v4-pro" } }
      }
    }
  },
  "log": { "level": "debug", "maxSize": 100, "maxAge": 7 }
}
```

## 当前模块状态

| 模块 | 状态 |
|------|------|
| wake | ✅ 新增 |
| cli | ✅ 新增 |
| deck | ✅（SSE 解析修复） |
| sail | ✅（tool_calls 序列化） |
| helm | ✅（EventAsk、RunLoop 去参数） |
| brig | ✅（getRawParam 迁移、formatReason 精简） |
| knot | ✅（ToolCallDetail、AskResult、GetRawParam 等） |
| reef | 🚧 |
| vault | ❌ |
| crew | ❌ |
| session | ❌ |

## 文档

| 文件 | 操作 |
|------|------|
| `docs/decisions.md` | 追加 DEC-030（自研 LogWriter）、DEC-031（EventAsk 事件驱动） |
| `docs/modules/helm/design.md` | 全面更新：RunLoop 签名、EventAsk、tool_calls 协议、ToolUseDelta 时序 |
| `docs/iterations/006-wake-mvp/main.md` | 新增迭代文档，含后续优化方向 |
| `docs/handoff.md` | 已删除（过时的 sail MVP 手交文档） |

## 已知问题 / 下一步

用户判断接下来的重点是 **debug CLI 和工具调用的潜在问题**：

1. **web_search 端点稳定性** — Parallel / Exa 两个免费 MCP 端点可能不稳定或返回异常格式。SSE 解析的边界情况待验证（多事件、空 data、非 JSON data）。
2. **ToolUse 展示逻辑** — `seenTools` map 去重机制目前按 `toolUse.ID` 去重，需验证跨 RunLoop 调用时是否正确清理（当前在 `EventDone` 和 `EventError` 时 `clear`）。
3. **多工具并行调用** — 已验证会出现两条 🔧 的情况（LLM 一次回复多个 tool_call），当前行为正确但未充分测试。
4. **askCLI 被拒绝后的状态** — tool message 注入 `Status=Error`，LLM 能否正确恢复需验证。
5. **续行符边缘情况** — `\\` 转义、空续行、续行中 EOF 等场景未充分测试。
6. **日志文件轮转** — wake 模块的轮转逻辑尚未经过长期运行验证。

## 建议 skills

- 继续调试时参考 `docs/modules/*/design.md` 了解各模块最新设计
- 新增决策记录到 `docs/decisions.md`（下一个 DEC-032）
- 迭代文档 `docs/iterations/006-wake-mvp/main.md` 可在 wake 模块闭环时更新

## 编译与运行

```bash
cd <project_root>
go build -o argo.exe ./src/cli/
./argo.exe
```
