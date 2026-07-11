# Mady Chat 与助理智能体架构决策记录

## 动机

Mady v0 中 `ChatAgentConfig` 将聊天和助理功能合并为单一 "chat-assistant" Agent。用户分析文档建议拆分为独立 Agent，这符合以下原则：

1. **关注点分离** — 聊天（轻护栏、心理引擎、无工具）和助理（Standard 护栏、工具集）的配置完全不同
2. **Context Protection** — Anthropic 研究表明，子 Agent 隔离上下文可防止污染，提升推理质量
3. **行业对齐** — OpenAI Agents SDK、LangGraph、A2A 等都采用 Router + Specialist 模式

## 架构决策

### ADR-1: Chat 与 Assistant 分离为两个独立 Agent

**决策：** 拆分 `ChatAgentConfig` 为 `ChatAgentConfig`（纯聊天）和 `AssistantAgentConfig`（工具执行）。

**理由：**
- 护栏等级不同（Chat=LevelLight, Assistant=LevelStandard）
- 工具需求不同（Chat=无工具, Assistant=20+ 工具中选 13 个）
- 心理引擎介入程度不同（Chat=情绪感知, Assistant=最小化）
- MaxTurns 不同（Chat=8, Assistant=20）

**替代方案：** 单 Agent 内 if-else 模式切换。被否决原因：护栏/工具/心理配置是静态的，不易动态切换。

### ADR-2: Router 统一管理 Handoff，Agent 配置不互相引用

**决策：** 所有 Handoff 由 `RouterConfig` 集中配置。Agent 配置函数（`ChatAgentConfig`、`AssistantAgentConfig` 等）不包含跨领域 Handoff。

**理由：** 避免循环引用导致栈溢出（`ChatAgentConfig` → `AssistantAgentConfig` → `ChatAgentConfig` → ...）。

**替代方案：** Lazy initialization。被否决原因：增加复杂度，不符合 Go 的显式风格。

### ADR-3: 关键词分类优先级 patent > legal > assistant > chat

**决策：** 专业领域优先匹配。

**理由：** 用户说"帮我查一下这个专利"时，虽然包含 assistant 关键词"查一下"，但核心意图是专利检索。

### ADR-4: Server 不修改，由调用方组装 Config

**决策：** `server.New()` 保持不变。调用方（`cmd/mady serve`）负责用 `domains.RouterConfig()` 包装 Config。

**理由：** Server 不应该知道领域路由细节。Router Agent 对 Server 透明。

### ADR-5: 工具限制用 DisableTools 反向控制

**决策：** 在 `tools.ExtensionConfig` 添加 `DisableTools []string`，默认空=全部启用。

**理由：** 不影响现有行为。Assistant Agent 禁用 bash/git/browser/execute_code/computer_use/process。

## 领域配置矩阵

| 领域 | Agent 名称 | 护栏等级 | 心理引擎 | 工具 | MaxTurns | 审批关卡 |
|------|-----------|---------|---------|------|----------|---------|
| chat | chat-agent | LevelLight | VAD/OCC (轻量) | 无 | 8 | 无 |
| assistant | assistant-agent | LevelStandard | SDT (最小) | web_search, web_fetch, read, write_file 等 13 个 | 20 | 无 |
| patent | patent-agent | LevelStrict | SDT (标准) | 知识库检索 | — | ✓ |
| legal | legal-advisor | LevelStrict | SDT (标准) | 知识库检索 | — | ✓ |

## 分类准确率设计

v0 使用关键词分类 + LLMClassifier 回退的「分层级联」策略：

1. 规则匹配（<1ms）→ patent/legal 关键词优先
2. Assistant 关键词其次
3. 其余 → chat
4. 如果未来关键词准确率不足，升级为 LLM 分类（接口不变，只换 `classify` 内部实现）

## 文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `domains/router.go` | 修改 | 添加 DomainAssistant/DomainTrademark, 更新 RouterConfig/ClassifyIntent |
| `domains/chat.go` | 修改 | 拆分为纯 Chat Agent |
| `domains/assistant.go` | 新建 | 工具型 Assistant Agent |
| `domains/psychological_config.go` | 新建 | 按领域提供 psychological.Config |
| `domains/classifier.go` | 修改 | 添加 assistant 到分类 schema |
| `domains/graph.go` | 修改 | 添加 assistant 到 BuildDomainGraph |
| `guardrails/disclaimer.go` | 修改 | 添加 assistant Disclaimer/RiskKeywords/ApprovalKeywords |
| `tools/tools.go` | 修改 | 添加 DisableTools 字段 |
| `cmd/mady/main.go` | 修改 | 添加 mady serve 命令 |
| `cmd/mady/web_test.html` | 新建 | Web 测试页面 |
| `domains/integration_test.go` | 新建 | 21 个集成测试 |
| `domains/domain_test.go` | 修改 | 适配新 RouterStep 签名 |
| `domains/classifier_test.go` | 修改 | 添加 assistant 关键词测试 |
| `domains/doc.go` | 修改 | 更新架构图 |

## 后续迭代（v0.2 已完成项）

以下为本文档撰写时的计划，已在 v0.3.0 中落地：

1. ✅ **Manifest 注册表**（`agentcore/manifest.go`）— JSON 声明式 Agent 注册，替代纯硬编码
2. ✅ **UserIntent v2** — `summarizeUserIntent()` LLM 摘要，5 分钟缓存，Provider 不可用时回退 v1
3. ✅ **案件感知 Agent** — `BuildProjectAgent(rec, base)` + `AgentPool` 生命周期管理
4. ✅ **ProjectRegistry** 入口集成 — `runServer` / `runTui` 自动初始化
5. ✅ **动态 Handoff** — `RouterConfigWithRegistry()` 自动注册 `project-{projectID}` 案件目标
6. ✅ **Disclosure 管线** — 10 节点 Pregel 图 + 异步 API + Agent Tool 封装
7. ✅ **专利/法律工作流 Tool 封装** — `analyze_patent_novelty` / `compare_legal_cases`
8. ✅ **错误类型体系** — RetryableError / FatalError / HandoffError / GuardrailError
9. ✅ **措辞规范** — `docs/tone-style-guide.md`，禁用绝对化表述
10. ✅ **Invisible Handoff**（v0.3.0）— Chat Agent 作为统一对话界面（`IntegratedChatConfig`），根据意图自动无缝委派给专业 Agent，用户无需感知路由切换
11. ✅ **Embed Manifest**（v0.3.0）— 4 个领域 JSON 通过 `go:embed` 编进二进制，`MadyHome()` 统一路径解析，任意目录开箱即用
12. ✅ **Reasonix 扩展包**（v0.3.0）— Evidence Ledger / File Checkpoint / Permission / PlanMode / Guardian AI / Evaluate / Tracing / Memory Compiler / 四级渐进式压缩

## 下季度候选

1. 向量召回上线（当前仅结构化过滤）
2. 记忆编译器策略学习增强
3. 评估框架与 CI 集成
4. 添加 DomainTrademark 领域
5. Checkpoint 暂停点：在 assistant 涉及"生成文档草稿"时加人工确认
6. Manifest 文件监听热加载（fsnotify）
