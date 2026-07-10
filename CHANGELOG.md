# Changelog

本文件遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 格式，
版本号遵循 [语义化版本 2.0.0](https://semver.org/lang/zh-CN/)。

## [0.1.0] - 2026-07-10

### Added

- **核心 Agent 运行时**：LLM-工具循环、流式响应、多 Agent 交接（委托/转移/转发）、自动上下文压缩、指数退避重试、检查点机制
- **事件系统**：类型安全的事件总线，支持实时可观测性（消息增量、工具调用、状态变更等）
- **生命周期钩子**：Guardrail 钩子、审计钩子、限流钩子，可拦截 Agent 执行的每个阶段
- **工具系统**：JSON Schema 校验、单工具钩子、全局中间件
- **内置工具扩展**（tools/）：文件系统（read/edit/write/delete/move/patch）、Shell（bash/process）、搜索（ls/grep/find/glob）、浏览器自动化（多提供商）、Web 搜索、代码执行、Git 操作、MCP 桥接、macOS 桌面控制
- **三层 Provider**：统一 OpenAI Chat Completions 兼容协议，支持 DeepSeek / 智谱 GLM / Kimi / 通义千问 / 通用兼容 API
- **7 层 TUI 架构**：Elm 风格消息传递、差异渲染、Kitty/Sixel 图像支持、模糊搜索自动补全、Markdown 渲染、主题系统
- **HTTP/SSE 服务器**：`/api/chat` 端点、线程 CRUD、技能管理、状态快照、AG-UI 事件流
- **A2A 协议**：完整的服务器/客户端实现，符合 Google Agent2Agent 规范，支持 Agent Card 发现、任务生命周期、WebSocket 传输
- **A2UI 协议**：声明式 UI 流式传输 v0.9.1，数据绑定、验证、A2A/AG-UI 传输绑定
- **ACP 协议**：基于 JSON-RPC 的 Agent 间通信，会话级 Agent 生命周期管理
- **AG-UI 协议**：SSE 事件流，运行生命周期/步骤进度/文本增量/推理块/工具调用/状态快照
- **MCP 客户端**：stdio 和 HTTP/SSE 传输，工具热刷新
- **领域路由**：Router Agent + Chat/Patent/Legal 子 Agent，基于关键词的意图分类器
- **护栏系统**：三级（Light/Standard/Strict）、领域免责声明、审批门控
- **心理引擎**：VAD 三维情绪空间、OCC 情绪模型、EMA 认知评价、Beck 认知扭曲、SDT 自我决定理论
- **知识管理**：Wiki/Patent/Legal 文档加载器、分块器、关键词搜索、BM25 重排序
- **图引擎**：静态 DAG 并行执行 + 循环 Pregel 图超步迭代
- **工作流原语**：Pipeline、Parallel、Router、Map 步骤
- **会话管理**：JSONL 追加写入树结构，支持分支、压缩、标签和版本迁移
- **SKILL 系统**：Markdown 格式的 AI 技能定义，YAML 前置元数据，热重载
- **示例应用**：cli-chat（TUI 聊天）、tui-demo、a2a-client/server、wiki-import、provider-compat
- **GitHub Actions CI**：go vet + build + test

[0.1.0]: https://github.com/xujian519/mady/releases/tag/v0.1.0
