# Argo 术语表

## barrel 触发

Go 编译器行为：`import _ "pkg"` 只执行 `pkg` 的 `init()`，不引用任何导出符号。

利用这一点，用一个 barrel 文件匿名导入所有工具子包，启动时编译器沿依赖链自动执行所有 `init()`，完成自注册。

```go
// deck/tools/tools.go —— barrel 文件
package tools

import (
    _ "argo/deck/tools/builtin/bash"   // 仅触发 init() 中的 registry.Register()
    _ "argo/deck/tools/builtin/read"
)
```

```go
// cmd/argo/main.go —— 启动时一行导入
import _ "argo/deck/tools"  // 触发 barrel → 触发所有子包 init()
```

名称来自 JS/TS 社区 "barrel export" 概念（index.ts 只做 re-export），Go 中用匿名导入达到相同"聚合触发"效果。

---

## choices

OpenAI Chat Completions API 响应中的多候选结果数组。通过 `n` 参数可让模型一次生成多个不同版本的回答，每个版本是一个 choice。

```json
{"choices": [
  {"index": 0, "message": {"content": "回答版本A..."}},
  {"index": 1, "message": {"content": "回答版本B..."}}
]}
```

实际中几乎无人使用 `n > 1`——多倍开销换多版本回答不划算。流式模式下 `choices` 数组永远只有一个元素，取 `choices[0]` 即可。

---

## 无限压缩循环

上下文长度超阈值 → 触发压缩 → 压缩后 token 数仍超阈值 → 再次触发压缩 → 仍超阈值 → …… 如此反复，每次压缩消耗一次 LLM 调用（生成摘要），但节省的 token 还抵不上这次调用的开销。

类比：用水舀舀船舱里的水，但水舀本身漏得更多——每次舀出去 100ml，水舀的破洞又漏回来 120ml。

防止手段——反抖动/断路器：
- Claude-Code：连续 3 次压缩后放弃（`MAX_CONSECUTIVE_AUTOCOMPACT_FAILURES = 3`）
- Hermes Agent：连续 2 次压缩节省率 < 10% 则跳过后续压缩
- OpenClaw：最大溢出压缩尝试次数上限

---

## SSE（Server-Sent Events）

LLM API 流式响应的传输协议。客户端发普通 HTTP POST，服务端以 `text/event-stream` 格式持续推送数据块，直至结束。

```
HTTP 响应头: Content-Type: text/event-stream

data: {"choices":[{"delta":{"content":"你"}}]}

data: {"choices":[{"delta":{"content":"好"}}]}

data: [DONE]
```

规则：
- 每行以 `data: ` 开头，后跟 JSON
- 两个事件之间用空行分隔
- 流结束发 `data: [DONE]`

OpenAI、DeepSeek、Kimi、MiniMax、豆包均使用同一套 SSE 格式（兼容 OpenAI Chat Completions API），因此 sail 的 openaiAdapter 和 compatibleAdapter 共用一套 SSE 解析逻辑，仅 baseURL 不同。