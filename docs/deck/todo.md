# deck 开发任务

## 1. 核心类型定义（无需测试）
- [ ] ▶ 1.1 定义 Tool 接口（Name / Description / Execute / Concurrency）、ToolResult、ToolCall、ConcurrencyMode
- [ ] 1.2 定义 builtinTool、cliTool、mcpTool 三个未导出结构体

## 2. 工具注册（需测试）

**测试角度**：注册、去重、查找、配置加载
**测试方式**：单元测试（Mock Tool 实现）
**测试策略**：每条覆盖以下场景

- [ ] 2.1 实现包级 Register / List / Lookup + unexported map
- [ ] 2.2 实现 LoadConfig（解析 ~/.argo/tools.json 的 cli + mcp 列表，含可选 truncation）
- [ ] 2.3 Register 正常注册 + 同名冲突报错
- [ ] 2.4 List 返回排序稳定的完整列表
- [ ] 2.5 Lookup 命中 / 未命中
- [ ] 2.6 LoadConfig 正常解析 / 文件不存在 / JSON 格式错误

## 3. 三种 Tool 实现（需测试）

**测试角度**：接口实现完整性、正常路径、错误路径
**测试方式**：表驱动单元测试
**测试策略**：每个实现覆盖 Execute 正常返回 + 错误返回 + Context 超时

- [ ] 3.1 实现 builtinTool（handler 函数指针 + Tool 接口方法）
- [ ] 3.2 实现 cliTool（exec.Command + paramsToArgs + Tool 接口方法）
- [ ] 3.3 实现 mcpTool（Name 由 serverName + toolName 推导 + 委托 MCPManager.CallTool）
- [ ] 3.4 builtinTool 表驱动测试：handler 正常 / 错误 / panic
- [ ] 3.5 cliTool 表驱动测试：命令正常退出 / 非零退出码 / 超时 / 二进制不存在
- [ ] 3.6 mcpTool 表驱动测试：CallTool 正常 / 错误 / Name 推导格式验证

## 4. 执行与后处理（需测试）

**测试角度**：截断边界值、分区调度正确性
**测试方式**：表驱动单元测试
**测试策略**：postProcess 覆盖关键边界（恰好等于 / 小于 / 远大于阈值）；ExecuteBatch 模拟多种 Concurrency 组合

- [ ] 4.1 实现内部 postProcess（头 40% + 尾 60% + 截断标记，MaxOutputChars 默认 50,000）
- [ ] 4.2 实现包级 ExecuteBatch（按 Concurrency 分区 → concurrent goroutine 组 + sequential 串行组）
- [ ] 4.3 postProcess 表驱动测试：输出 < 50K / = 50K / = 50K+1 / 远大于 50K
- [ ] 4.4 ExecuteBatch 调度测试：全 concurrent / 全 sequential / 混合 / 空列表

## 5. MCPManager（需测试）

**测试角度**：协议解析、子进程生命周期、断连容错
**测试方式**：单元测试 mock MCP 子进程 + 集成测试（手动脚本，不自动执行）
**测试策略**：单元测试覆盖逻辑路径；手动脚本操作真实 MCP server

- [ ] 5.1 实现 MCPManager（Start / tools/list 获取清单并构造 mcpTool 注册 / IsAlive / CallTool / Shutdown）
- [ ] 5.2 单元测试：tools/list 响应解析 / 空工具列表 / 子进程启动失败 / IsAlive 状态切换 / Shutdown 清理
- [ ] 5.3 手动测试脚本：连接真实 MCP server，验证 tools/list 清单获取 + CallTool 执行 + 手动 kill 子进程后 IsAlive 变为 false

## 6. 内置工具 tools/builtin/（需测试）

**测试角度**：正确输出、异常输入、文件边界
**测试方式**：集成测试（临时目录 + 真实文件系统，不 mock）
**测试策略**：每个工具覆盖输入合法 + 输入非法场景

- [ ] 6.1 实现 bash（exec.Command("bash", "-c", cmd)）
- [ ] 6.2 实现 read（os.ReadFile + 行号输出）
- [ ] 6.3 实现 write（os.WriteFile + 父目录自动创建）
- [ ] 6.4 实现 edit（读文件 → 精确字符串替换 → 写文件）
- [ ] 6.5 实现 grep（exec.Command("grep", "-rn", pattern, path)）
- [ ] 6.6 实现 glob（filepath.Glob）
- [ ] 6.7 bash 集成测试：正常命令 / 命令不存在 / 非零退出 / 超时
- [ ] 6.8 read 集成测试：正常文件 / 文件不存在 / 空文件 / 二进制文件
- [ ] 6.9 write 集成测试：新文件创建 / 覆写已有文件 / 父目录自动创建 / 路径无写权限
- [ ] 6.10 edit 集成测试：单次替换成功 / old_string 不存在 / 多次出现报错 / 文件不存在
- [ ] 6.11 grep 集成测试：有匹配 / 无匹配 / 路径不存在
- [ ] 6.12 glob 集成测试：有匹配 / 无匹配 / 非法 pattern
