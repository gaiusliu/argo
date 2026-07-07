# Handoff — Bug 修复 + 输入区改造 + vault 模块启动

## 上回会话（wake + CLI REPL + 集成联调）

详见 git log `8efcdb7`。本次会话在其基础上做了以下工作。

## 本次修复 / 新增

### stripHTML 正则 panic → html-to-markdown（`src/deck/builtin_tool.go`）

Go `regexp` 使用 RE2 引擎，不支持反向引用 `\1`。原来 `(?is)<(script|style|...)[^>]*>.*?</\1>` 在 `regexp.MustCompile` 时直接 panic。

**修复**：引入 `github.com/JohannesKaufmann/html-to-markdown`，11 行替换旧的 stripHTML + isSkipTag（DEC-032，废弃 DEC-014）。

### globHandler 不支持 `**` → doublestar（`src/deck/builtin_tool.go`）

`filepath.Match` 不支持 `**` 递归匹配，且只匹配 basename。LLM 发送 `**/hello.go` 永远返回 `no files found`。

**修复**：引入 `github.com/bmatcuk/doublestar/v4`，改用 `filepath.Rel` 构造相对路径 + `doublestar.Match`（DEC-033）。`GetRawParam` 加防御型调试日志（slog.Error），同时 `toolParamKeys` 补了 `"glob": ParamPattern`。

### write/edit askUser 时展示彩色 diff 预览（`src/cli/main.go`）

`askCLI` 对 write/edit 工具增加变更预览——读取目标文件当前内容，模拟替换，用 `github.com/sergi/go-diff/diffmatchpatch` 生成行级 diff，删除行红色、新增行绿色输出到 stderr。`showDiff` 函数 42 行。

### 用户输入区域背景色（`src/cli/main.go`）

放弃分割线方案（续行时固定位置的下边框错位），改用 ANSI 背景色标记输入区域：
- `\033[48;5;60m`（灰蓝 #5f5f87）→ `\033[49m`（复位）
- 每次 `argo> ` 和续行 `  → ` 前设置背景色，输入结束复位
- `\033[K` 清除行尾以填充全宽背景色
- 回车后 `%s\n` 复位背景色，避免下一行残留

### 文档 / 配置修正

- `docs/decisions.md`：新增 DEC-032（html-to-markdown）+ DEC-033（doublestar），DEC-014 标记废弃
- `handoff.md`：session 模块去掉，归入 vault；crew 明确为 Skill 库
- `CLAUDE.md`：修正 pi 描述（删除"Anthropic 官方"）

## go.mod 新增依赖

| 依赖 | 用途 |
|------|------|
| `github.com/JohannesKaufmann/html-to-markdown` | HTML→Markdown（DEC-032） |
| `github.com/bmatcuk/doublestar/v4` | glob `**` 匹配（DEC-033） |
| `github.com/sergi/go-diff/diffmatchpatch` | write/edit diff 预览 |

## 当前模块状态

| 模块 | 状态 |
|------|------|
| wake | ✅ |
| cli | ✅（加了 diff 预览 + 背景色输入区） |
| deck | ✅（stripHTML 修复、glob doublestar 修复） |
| sail | ✅ |
| helm | ✅ |
| brig | ✅ |
| knot | ✅（toolParamKeys 补 glob 条目、GetRawParam 调试日志） |
| reef | 🚧 |
| vault | ❌ → 下一会话重点（含 session 管理） |
| crew | ❌（Skill 库管理） |

## 下一步：vault 模块设计与开发

### 前置条件（已就绪）

- **Memory 协议**：[`docs/what.md`](docs/what.md) 第 78-84 行定义了 `Store`/`Retrieve` 接口和 `MemoryEntry` 结构
- **MemoryEntry 类型**：[`src/helm/types.go`](src/helm/types.go) 第 38-41 行已有基础定义
- **目录工具**：`helm.ArgoDir()`（`~/.argo/`）和 `helm.ProjectDir()`（`<root>/.argo/`）已实现
- **TurnState**：[`src/helm/types.go`](src/helm/types.go) 第 25-30 行，当前在 cli/main.go 的堆栈上，需要持久化
- **Config**：`~/.argo/config.json` 已有配置加载链路（`knot.LoadConfig`）

### vault 需要实现

1. **存储引擎选型**—SQLite vs JSON 文件。建议 SQLite（单文件、零配置、Go 标准库 `database/sql` + `modernc.org/sqlite` 纯 Go 驱动）
2. **Session 管理**—创建/恢复会话。会话 ID 生成、TurnState.Messages 序列化/反序列化、会话元数据（创建时间、模型名、turn 计数）
3. **Transcript 写入**—每轮对话结束后追加消息到当前会话
4. **Memory 存取**—`Store(key, value, metadata)` + `Retrieve(query, limit)` → `[]MemoryEntry`
5. **helm 集成点**—`RunLoop` 开始前加载记忆，每轮结束后写入 transcript

### 建议技能

- 设计阶段参考 `docs/modules/*/design.md` 了解模块设计文档格式
- 新增决策记录到 `docs/decisions.md`（下一个 DEC-034）
- 新建迭代文档 `docs/iterations/007-vault-mvp/main.md`

### 编译与运行

```bash
cd c:/Users/Gaiusliu/Desktop/code/argo
go build -o argo.exe ./src/cli/
./argo.exe
```
