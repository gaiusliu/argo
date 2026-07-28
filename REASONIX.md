# Argo 项目规则

## 常量规范

- 禁止使用 `iota` 从 0 开始指代有意义的值，避免零值与合法状态混淆
- 使用 `iota + 1` 跳过 0 值，或显式赋值从 1 开始
- 示例：`StatusPending = iota + 1` 而非 `StatusPending = iota`
