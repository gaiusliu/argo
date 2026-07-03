# Argo 项目指南

## 调用 code-review skill 时的规则

`.agents/skills/code-review` 的 Step 4 会向两个子代理发送完整 diff。为保持 Standards/Spec 上下文隔离：

1. Step 1 的 `git diff` 按文件类型拆分——代码文件（`*.go`）和文档文件分开发送
2. 两个子代理只收到 `*.go` 文件的 diff
3. Spec 文档（`docs/modules/*/design.md`、`docs/iterations/*/main.md`）单独作为规格依据传给 Spec 子代理，不以 diff 形式出现

这样避免：
- Standards 子代理对 .md 文档报告无意义的代码坏味
- Spec 子代理用自己的文档变更去"匹配"规格（循环审查）

## commit message 规范

- 不使用 `Co-Authored-By` 署名
- message 使用中文描述

## Go 代码风格

项目代码必须遵守 `go-styleguide/`（Google Go 编程规范），核心要点：

- **格式化**：所有代码必须通过 `gofmt` 格式化，提交前执行 `gofmt -w`
- **命名**：MixedCaps 驼峰命名，禁止下划线；包名全小写；避免包名与导出符号重复（如 `tool.ToolRegistry` → `tool.Registry`）
- **注释**：字段注释放在字段上方独立一行，避免行尾注释；类型/函数的 doc 注释以名称开头
- **导入分组**：标准库 → 空行 → 项目及第三方包
- **接口归属**：接口由消费者（helm）定义，而非实现者
- **错误处理**：error 为最后一个返回值；错误字符串小写开头，不以标点结尾；先处理错误再走正常路径（减少 else 嵌套）
- **goroutine**：启动 goroutine 时必须清楚它何时退出

## 命令白名单

- 项目 `.claude/settings.json` 维护了无需审批的 Bash 命令白名单
- 如遇到反复审批的常用命令，主动询问用户是否加入白名单
