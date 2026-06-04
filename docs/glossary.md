# Argo 术语表

## barrel 触发

Go 编译器行为：`import _ "pkg"` 只执行 `pkg` 的 `init()`，不引用任何导出符号。

利用这一点，用一个 barrel 文件匿名导入所有工具子包，启动时编译器沿依赖链自动执行所有 `init()`，完成自注册。

```go
// deck/tools/tools.go —— barrel 文件
package tools

import (
    _ "argo/deck/tools/bash"   // 仅触发 init() 中的 registry.Register()
    _ "argo/deck/tools/read"
)
```

```go
// cmd/argo/main.go —— 启动时一行导入
import _ "argo/deck/tools"  // 触发 barrel → 触发所有子包 init()
```

名称来自 JS/TS 社区 "barrel export" 概念（index.ts 只做 re-export），Go 中用匿名导入达到相同"聚合触发"效果。

---