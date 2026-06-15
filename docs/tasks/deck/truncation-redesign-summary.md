# deck 截断逻辑重构摘要（pi 模式）

> **用途**：本文件是跨会话的执行参考。新会话开篇应先读此文件以理解背景、目标、已认可的设计与执行顺序，再开始任何修改。
>
> **生成时间**：用户决定用 pi/opencode 模式重写 deck 截断逻辑
> **执行顺序**：C → B → A（先规约，再参数化，最后完整重构）

---

## 1. 目标

将 `argo/deck/postprocess.go` 从"头 40% + 尾 60% 组合（几乎不节省）"重构为"工具自己选 head/tail + truncation 目录存完整输出 + metadata 字段让 LLM 查尾"，参考 **pi** (`packages/agent/src/harness/utils/truncate.ts` + `shell-output.ts`) 和 **opencode** (`packages/opencode/src/tool/truncate.ts`) 的设计。

---

## 2. 背景与现状问题

### 当前 postprocess.go 截断公式

```go
// deck/postprocess.go:10
if outputLen <= MaxOutputChars+TruncationMinSavings {
    return result
}
head := result.Output[:20000]              // 前 40% = 20000
tail := result.Output[outputLen-30000:]    // 后 60% = 30000
result.Output = head + marker + tail       // 总长 ≈ 50078
```

**悖论**：head 40% (20000) + tail 60% (30000) = **50000 = MaxOutputChars**，加 marker 后约 50078 字符。**截断前后几乎不节省（< 1%）**。

### 触发条件

`outputLen > MaxOutputChars + TruncationMinSavings` = 50200 字符才截断。

### 6 个开源项目对比

| 项目 | 截断方式 | 实际节省 |
|------|---------|---------|
| opencode `truncate.ts:86-142` | head 或 tail（**二选一**），完整内容存盘 | **真的截断** |
| pi `truncate.ts:125 / 215` | `truncateHead` 或 `truncateTail`（**二选一**），完整内容存盘 | **真的截断** |
| Claude-Code `toolLimits.ts:13` | 简单按字符数截断 | **真的截断** |
| openclaw `format.ts:60` | `truncateText` 简单字符截断 | **真的截断** |
| hermes-agent `tools.py:205` | `_truncate_text` 简单字符截断 | **真的截断** |
| **argo（当前）** | head+tail 组合（100%）| **< 1%** ❌ |

**所有其他项目都"真的截断"——把内容压到限制内。argo 是唯一做组合且几乎不节省的**。

---

## 3. 核心设计方向（已与用户确认）

### 3.1 工具决定 head/tail，框架不组合

- **不**做"head 40% + tail 60%"这种组合
- 每个 Tool 在自己内部决定用 `head` 还是 `tail`
- 调用依据：**工具类型/使用场景**（bash 输出关键信息在尾部，file read 在头部）

### 3.2 完整输出存到 truncation 目录

参考 opencode `truncate.ts:128` + pi `shell-output.ts:68-82`：
- 截断时把完整内容写到 `~/.local/share/opencode/truncation/`（或类似目录）
- metadata 字段记录文件路径
- LLM 通过 `read` 工具按需查尾

### 3.3 metadata 驱动 LLM 决策

返回结果时同时携带：
- `truncated: true/false`
- `original_length: N`
- `kept_length: M`（截断后长度）
- `truncation_strategy: "head" | "tail"`
- `full_output_path: "/path/to/truncated/tool-xxx.log"`（如适用）

LLM 看到 metadata 可决定：
- 任务只需要截断内容 → 直接用
- 任务需要尾部信息 → 用 `read` 工具读 `full_output_path`

### 3.4 触发条件简化

**新触发条件**：`outputLen > MaxOutputChars`（去掉 +TruncationMinSavings）。
**理由**：新设计截断后输出远小于 MaxOutputChars，永远节省。

---

## 4. 6 个 builtin 工具的截断策略（已认可）

| 工具 | 策略 | 理由 |
|------|------|------|
| `bash` | **tail** | 错误/退出码/最终结果在尾部（pi `shell-output.ts:112` 同样用 tail）|
| `read` | **head** | 头是文件元信息/开头（pi `truncate.ts:120` 注释："Suitable for file reads where you want to see the beginning"）|
| `grep` | **head** | 头是匹配项（前 N 行）|
| `glob` | **head**（实际几乎不截断）| glob 输出小 |
| `write` | 不截断 | 写成功无输出 |
| `edit` | 不截断 | 写成功无输出 |

---

## 5. 执行顺序 C → B → A

### 阶段 C：先更新 decisions.md 记录设计意图（不写代码）

**目标**：在动手前先在 `argo/docs/deck/decisions.md` 记录设计决策，避免"代码先行，文档滞后"。

**任务清单**：
1. 读 `argo/docs/deck/decisions.md` 现有所有 DEC-XXX 条目
2. 起草新条目 `DEC-XXX：postProcess 截断策略重设计` 内容包括：
   - 决策：每个 Tool 自己选 head/tail，postProcess 不组合
   - 决策：完整输出存到 truncation 目录
   - 决策：metadata 加 `truncated` / `original_length` / `kept_length` / `truncation_strategy` / `full_output_path` 字段
   - 决策：触发条件简化为 `outputLen > MaxOutputChars`
   - 决策：6 个工具各自的截断策略
   - 拒绝的替代方案：保留 head+tail 组合（节省 < 1%）/ 框架统一决定 head/tail（缺乏场景感知）
3. 在 `argo/docs/deck/how.md` 第 17-19 行（`postProcess` 说明）更新为新设计
4. 在 `argo/docs/tasks/deck/builtinTool.md` 测试场景表中标注"待 gen-code 阶段后新增 truncated 元数据测试"

**验证**：`go test ./deck/...` 仍 RED（不写代码，状态不变）

### 阶段 B：postProcess 参数化

**目标**：让 postProcess 接受 TruncationStrategy 参数，**不**改 Execute、不引入 truncation 目录。

**任务清单**：
1. 在 `argo/deck/types.go` 添加：
   ```go
   type TruncationStrategy string
   const (
       TruncationHead TruncationStrategy = "head"
       TruncationTail TruncationStrategy = "tail"
       TruncationNone TruncationStrategy = "none"  // 不截断
   )
   ```
2. 改 `argo/deck/postprocess.go`：
   - 函数签名改为 `func postProcess(result ToolResult, strategy TruncationStrategy) ToolResult`
   - 移除 head+tail 组合逻辑
   - 改用对应 `truncateHead` 或 `truncateTail` 行为
   - 触发条件简化为 `outputLen > MaxOutputChars`
   - metadata 自动加 `truncated` / `original_length` / `kept_length` / `truncation_strategy`
3. **不改** `builtinTool.go` 的 Execute（让 builtinTool 默认传 TruncationTail，与原设计兼容）
4. 更新 `argo/deck/postprocess_test.go`：
   - 把所有 `expectedTruncatedOutput` 测试拆为 `TestPostProcess_Head` 和 `TestPostProcess_Tail` 两组
   - 验证 metadata 字段
   - 验证新触发条件（> MaxOutputChars 触发，不再需要 +200）
5. 更新 `argo/deck/builtinTool_test.go`：
   - `TestBuiltinTool_Execute_PostProcess` 改名或拆分（`Truncated` 改用新逻辑）
   - `TestBuiltinTool_Execute_HandlerError_OutputTruncated` 验证 metadata 字段

**验证**：
- `go test ./deck/...` 仍 RED
- 新 metadata 断言触发
- head 模式返回 head N 字符，tail 模式返回 tail N 字符

### 阶段 A：完整重构（Tool 选择 + truncation 目录）

**目标**：每个 Tool 内部传自己的 TruncationStrategy；完整输出存到 truncation 目录。

**任务清单**：
1. 在 `argo/deck` 添加 truncation 目录管理：
   - 目录路径：`~/.local/share/argo/truncation/`（参考 opencode `TRUNCATION_DIR`）
   - 工具：`writeTruncation(content) (path, error)`、`cleanupOldTruncations(retention)` 等
2. 改 `builtinTool.go` 的 Execute：
   - 接受可选的 truncation 策略参数（或读 Tool 配置）
   - 调用 handler 后，如果 Output 超过 MaxOutputChars，先把完整内容写入 truncation 目录
   - metadata 加 `full_output_path: "/path/to/tool-xxx.log"`
   - 再调 postProcess
3. 6 个 builtin 工具实现各自的 TruncationStrategy：
   - `bash` → TruncationTail
   - `read` / `grep` / `glob` → TruncationHead
   - `write` / `edit` → TruncationNone
4. 更新 `argo/docs/deck/how.md`：6 个工具的截断策略表
5. 更新 `argo/docs/tasks/deck/builtinTool.md`：测试场景新增 metadata 断言
6. 更新 `argo/deck/postprocess_test.go` 和 `builtinTool_test.go`：
   - 新增 truncation 目录写盘测试
   - 验证 metadata 包含 `full_output_path`
   - 验证 cleanup 机制

**验证**：
- `go test ./deck/...` 仍 RED（桩代码不实现 truncation 目录）
- 完整流水线 ⑤ gen-code 阶段实现 Execute 后转 GREEN

---

## 6. 需要修改的文件清单

| 文件 | 阶段 | 改动 |
|------|------|------|
| `argo/docs/deck/decisions.md` | C | 新增 DEC 条目（截断策略重设计）|
| `argo/docs/deck/how.md` | C + A | 更新 postProcess 说明 + 6 工具策略表 |
| `argo/docs/deck/what.md` | A | 更新契约（truncation 目录行为）|
| `argo/docs/tasks/deck/builtinTool.md` | C + A | 测试场景表更新 + metadata 字段说明 |
| `argo/deck/types.go` | B + A | 新增 `TruncationStrategy` 类型 + 常量 |
| `argo/deck/postprocess.go` | B | 函数签名改 + 移除组合逻辑 + 新 metadata |
| `argo/deck/builtinTool.go` | A | Execute 实现 truncation 目录 + metadata |
| `argo/deck/tools/builtin/*.go`（bash/read/grep/glob/write/edit）| A | 每个工具声明自己的 TruncationStrategy |
| `argo/deck/postprocess_test.go` | B + A | 拆 head/tail 测试组 + metadata 断言 + truncation 目录测试 |
| `argo/deck/builtinTool_test.go` | B + A | PostProcess 测试改 + metadata 字段断言 |
| `argo/deck/builtinTool_*.go`（6 个工具的测试）| A | 各自加 truncation 行为测试 |
| `argo/docs/tasks/deck/builtinTool.test-review.md` | B + A | 记录重构决策（与 R1/R2/R3 并列）|

---

## 7. 关键技术参考

### pi 的实现

| 文件 | 行号 | 关键代码 |
|------|------|---------|
| `pi/packages/agent/src/harness/utils/truncate.ts:125-207` | truncateHead 函数 | 保留前 N 行/字节，不切行 |
| `pi/packages/agent/src/harness/utils/truncate.ts:215-289` | truncateTail 函数 | 保留后 N 行/字节，最后一行可部分切 |
| `pi/packages/agent/src/harness/utils/truncate.ts:11-12` | 默认限制 | `DEFAULT_MAX_LINES = 2000`, `DEFAULT_MAX_BYTES = 50 * 1024` |
| `pi/packages/agent/src/harness/utils/shell-output.ts:50` | maxOutputBytes | `DEFAULT_MAX_BYTES * 2`（运行时滚动窗口）|
| `pi/packages/agent/src/harness/utils/shell-output.ts:68-82` | ensureFullOutputFile | 创建临时文件存完整输出 |
| `pi/packages/agent/src/harness/utils/shell-output.ts:112` | truncateTail 调用 | bash 默认用 tail |
| `pi/packages/agent/src/harness/utils/shell-output.ts:133-138` | 返回 fullOutputPath | 结果携带完整路径 |

### opencode 的实现

| 文件 | 行号 | 关键代码 |
|------|------|---------|
| `opencode/packages/opencode/src/tool/truncate.ts:16-17` | 默认限制 | `MAX_LINES = 2000`, `MAX_BYTES = 50 * 1024` |
| `opencode/packages/opencode/src/tool/truncate.ts:86-142` | output 函数 | truncate 主逻辑 + 完整内容存盘（write）|
| `opencode/packages/opencode/src/tool/truncate.ts:128` | write 工具调用 | `yield* write(text)` |
| `opencode/packages/opencode/src/tool/truncate.ts:130-132` | hint 消息 | "The tool call succeeded but the output was truncated" |

---

## 8. 验证清单（每个阶段结束后）

| 检查项 | 方法 |
|--------|------|
| 编译通过 | `go build ./...` |
| 桩代码下 RED 状态保持 | `go test ./deck/... -count=1` 应 FAIL |
| 新 metadata 字段断言触发 | 测试日志中应见 metadata 相关 t.Error |
| 无意外 PASS | 没有"实现已就位但测试未更新"导致 PASS |
| 文档与代码同步 | decisions.md / how.md / what.md / task 文档反映新设计 |
| 与 pi/opencode 设计对齐 | head/tail 二选一（不组合）、truncation 目录存盘、metadata 字段 |

---

## 9. 已知风险点

| 风险 | 缓解 |
|------|------|
| 完整输出写盘涉及文件系统（并发、清理、空间）| 参考 opencode `truncate.ts:55-67` 的 cleanup 机制，retention 7 天 |
| LLM 不读 `full_output_path` 导致信息丢失 | marker 里明确提示"如需尾部信息请调用 read 工具读取完整输出文件，路径见 metadata.full_output_path" |
| 6 个 builtin 工具的测试改动量大 | 阶段 B 先不动工具实现，阶段 A 再分工具加测试 |
| 现有 R1/R2/R3 测试报告已固化 | 阶段 A 完成后追加 R4 报告记录重构决策 |

---

## 10. 当前会话已确定的前置事项

✅ 用户已认可：
- 设计方向（pi/opencode 模式）
- 6 个工具的截断策略
- 执行顺序 C → B → A
- 触发条件简化为 `outputLen > MaxOutputChars`
- metadata 字段列表
- 头 20% 提议的延伸——**最终改为 pi 模式的"工具自己选 head/tail"**，不再固定 20% 比例

❌ 用户在会话结束前提到 token 消耗大——所以把后续执行转移到新会话，新会话应：
1. 优先读本摘要文件
2. 按 C → B → A 顺序执行
3. 任何阶段的代码改动前都应回到摘要核对范围
