# Argo 项目规则

## 仓库结构

- **`argo`**（`gaiusliu/argo`）：主代码仓库，`src/` + `src2/` 等
- **`argo-docs`**（`gaiusliu/argo-docs`）：`docs/` 目录为独立 Git 仓库，迭代文档、设计文档、handoff 等均在此仓库中
- 两个仓库各自独立提交和推送，互不影响

## 代码生成

- 一次只生成/修改一个文件，每步不超过 30 行
- 每步执行完后暂停，等待用户审核通过再继续下一步
- 审核不通过则修改本步代码，不得跳过进入下一步

## 架构规则

- 参见 `docs/rules.md`：ServerCfg（进程级，绑定到 Server 结构体）与 AgentCfg（请求级，handler 内动态加载）的加载边界
