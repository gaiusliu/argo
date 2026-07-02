# sail 开发任务

## 1. 核心类型定义
- [ ] ▶ 1.1 定义 ModelConfig、TokenUsage、APIError、ModelCapability、ChatRequest、ChatResponse、Event

## 2. 配置加载
- [ ] 2.1 实现 LoadConfig — 解析 ~/.argo/models.json，文件不存在时自动生成默认配置
- [ ] 2.2 实现 GenerateDefaultConfig — 扫描环境变量中的已知 API Key，生成默认模型条目（anthropic / openai / deepseek）

## 3. Provider Adapter
- [ ] 3.1 实现 anthropicAdapter（Anthropic Messages API → channel 流式 Event）
- [ ] 3.2 实现 openaiAdapter（OpenAI Chat Completions API → channel 流式 Event）
- [ ] 3.3 实现 openaiCompatibleAdapter（通用 /v1/chat/completions 端点 + base_url 可配置）

## 4. Client 实现
- [ ] 4.1 实现 Client.New + Chat（按 modelName 查找配置 → 选择 adapter → 流式推送 Event → 归一化 TokenUsage）
- [ ] 4.2 实现 Window — 返回当前模型的 context window token 上限
- [ ] 4.3 实现 ChatRequest 参数构造（system prompt + messages + tools → adapter 内部转换为各 API 格式）

## 5. 错误分类
- [ ] 5.1 实现 classifyError — HTTP 状态码 → APIError{Code, Retryable} 分类（auth / quota / rate_limit / server_error / context_overflow / network）

## 6. 模型元数据
- [ ] 6.1 构建内置默认表（claude-sonnet / gpt-4o / deepseek 等主流模型的能力参数）
- [ ] 6.2 实现 lookupCapability（三级查找：用户覆盖 → OpenRouter 缓存 → 内置表 → 保守默认值）
- [ ] 6.3 实现 RefreshCapabilities（从 OpenRouter API 拉取模型元数据，不阻塞启动）

## 7. 测试
- [ ] 7.1 LoadConfig / GenerateDefaultConfig 测试
- [ ] 7.2 anthropicAdapter / openaiAdapter 流式 Event 测试
- [ ] 7.3 openaiCompatibleAdapter 自定义 base_url 测试
- [ ] 7.4 classifyError 各类状态码分类测试
- [ ] 7.5 模型元数据三级查找 + 回退保守默认值测试
