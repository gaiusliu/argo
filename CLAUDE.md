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
