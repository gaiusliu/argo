# Argo

极简 AI Coding Agent 框架。Go 实现，状态机驱动 "模型调用 → 工具执行 → 循环" 的 ReAct 链路。

## 解决什么问题

为 AI coding 场景提供最小可用的 Agent 运行基座——工具注册、权限门控、流式 LLM 调用、对话持久化、会话管理。模块各自独立、通过接口协作，任一模块可替换。

## 架构

```
CLI（HTTP 客户端）──▶ argo-server（HTTP + SSE）──▶ helm（状态机）
                                                    │
           ┌────────────────────────┬───────────────┴───────────────┬────────────────┐
           ▼                        ▼                               ▼                ▼
          sail（模型）            deck（工具）                    brig（权限）     crew（技能）
                                    │
          vault（存储）──▶ voyage（会话聚合）
```

## 部署

### 环境要求

- Go 1.25+
- 至少一个 LLM 提供商的 API Key（OpenAI / DeepSeek）

### 构建

```bash
# 方式一：构建脚本（推荐）
bash build.sh

# 方式二：手动构建
go build -o argo ./src/cli/
go build -o argo-server ./src/argo-server/
```

产物：

- `argo` / `argo.exe` — CLI 客户端（Bubble Tea TUI）
- `argo-server` / `argo-server.exe` — HTTP 后端（端口 4090）

### 启动

```bash
# 设置 API Key（至少一个）
export OPENAI_API_KEY=sk-xxx           # OpenAI
export DEEPSEEK_API_KEY=sk-xxx         # DeepSeek

# 启动服务端
./argo-server

# 另一个终端启动 CLI
./argo --addr localhost:4090
```

CLI 启动时会自动检测 `argo-server` 是否已在 `localhost:4090` 运行；未运行时自动作为子进程拉起。

## 配置

Argo 使用 `~/.argo/config.json` 作为全局配置文件。首次运行时自动生成默认配置；你也可以用 `$ARGO_HOME` 环境变量自定义 Argo 目录。

### 配置示例

```json
{
  "sail": {
    "model": "deepseek-v4-pro",
    "provider": {
      "deepseek": {
        "api": "openai-completions",
        "name": "deepseek",
        "env": ["DEEPSEEK_API_KEY"],
        "models": {
          "deepseek-v4-pro": {
            "name": "deepseek-v4-pro",
            "reasoning": false,
            "limit": {
              "context": 128000,
              "output": 8192
            }
          }
        }
      }
    }
  },
  "log": {
    "dir": "./logs/",
    "level": "info",
    "maxSize": 100,
    "maxAge": 7
  },
  "context": {
    "compact": "llm-summary",
    "compactThreshold": 80,
    "truncation": "head-tail"
}
```

### 配置项说明

| 区块 | 字段 | 说明 |
|------|------|------|
| **sail** | `model` | 默认模型名，必须在某个 provider 的 `models` 中定义 |
| **sail.provider** | `<name>` | provider 标识，可配置多个（如 `deepseek`、`openai`） |
| **provider** | `api` | API 协议类型，目前为 `"openai-completions"`（OpenAI 兼容 Chat Completions） |
| **provider** | `env` | 从哪个环境变量读取 API Key |
| **provider.models** | `<model>` | 模型名 → 模型配置的映射 |
| **model** | `name` | 传递给 API 的模型标识 |
| **model** | `reasoning` | 是否启用推理模式（thinking） |
| **model** | `limit.context` | 上下文窗口上限（token） |
| **model** | `limit.output` | 输出 token 上限 |
| **log** | `level` | 日志级别：`debug` / `info` / `warn` / `error` |
| **log** | `dir` | 日志输出目录 |
| **log** | `maxSize` | 单日志文件上限（MB） |
| **log** | `maxAge` | 日志保留天数 |
| **context** | `compact` | 上下文压缩策略，暂仅支持 `"llm-summary"` |
| **context** | `compactThreshold` | 触发压缩的上下文占用百分比（1-100） |
| **context** | `truncation` | 截断策略，暂仅支持 `"head-tail"` |

### 模型切换

在 CLI 中通过 `/model <模型名>` 指令即可切换模型，无需重启。模型名必须在配置文件的 `sail.provider.<name>.models` 中已定义。

### 环境变量

| 变量 | 用途 |
|------|------|
| `OPENAI_API_KEY` | OpenAI API Key |
| `DEEPSEEK_API_KEY` | DeepSeek API Key |
| `ARGO_HOME` | 自定义 Argo 配置目录（默认 `~/.argo/`） |

## 特点
