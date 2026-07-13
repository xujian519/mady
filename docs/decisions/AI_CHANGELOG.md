# AI 决策变更日志

## 格式

```
## YYYY-MM-DD: 标题

- **变更**: 做了什么
- **原因**: 为什么做
- **影响范围**: 涉及哪些包/文件
- **风险等级**: 低/中/高
- **审查要求**: L1-L4
```

## 2026-07-13: 向量检索落地阶段1 — SQLite backend 接线（FTS + Vector RRF 融合）

- **变更**:
  1. **新建 `knowledge/backend_hook.go`**(~130行)：`BackendRetrievalHook` 类型，嵌入 `BaseLifecycleHook`，实现 `BeforeModelCall` 调用 `KnowledgeExtension.search()` 走 `backendSearch`（FTS + Embed+VectorSearch → RRF 融合）；自实现 `buildContextBlock` + `injectContext`（复刻 `retrieval/agent.go` 的上下文格式化和注入逻辑）
  2. **修改 `knowledge/extension.go`**：新增 `BackendHook(cfg) agentcore.LifecycleHook` 方法，`backend==nil` 时返回 nil，否则返回 `NewBackendRetrievalHook`
  3. **修改 `cmd/mady/main.go`**：新增 `buildEmbedder()`（读 OMLX_BASE_URL/OMLX_API_KEY/OMLX_EMBED_MODEL 构建 `APIEmbedder`）、`loadKnowledgeBackend(madyHome)`（读 KNOWLEDGE_DB_DIR → `sqlite.NewSQLiteStore` 只读打开 knowledge.db）；改造 `loadWikiStore` 为优先 SQLite backend（buildEmbedder → loadKnowledgeBackend → NewExtension(nil,...) → WithBackend → BackendHook），回退 WIKI_PATH 内存库
  4. **新建 `knowledge/backend_hook_test.go`**：7 个测试覆盖 nil guard / context 注入 / 空查询跳过 / 无结果跳过 / nil mcc 安全 / FTS+Vector RRF 双通道融合
- **原因**: 向量检索算法层（APIEmbedder/SQLiteStore/RRFFuser/backendSearch）已实现但生产链路完全未接线，`WithBackend` 全项目零 caller，知识检索生产关闭。此改动完成阶段1接线，让 Agent 运行时自动从 81K 文档/144K chunks 的 knowledge.db 执行混合检索
- **影响范围**: knowledge/backend_hook.go(新), knowledge/extension.go, cmd/mady/main.go, knowledge/backend_hook_test.go(新)
- **环境变量**: OMLX_BASE_URL(默认 http://127.0.0.1:8000/v1) / OMLX_API_KEY / OMLX_EMBED_MODEL(默认 bge-m3-mlx-8bit) / KNOWLEDGE_DB_DIR(默认 ~/.mady/knowledge)
- **降级策略**: OMLX_API_KEY 未设置 → embedder=nil → SQLite backend 不可用 → 回退 WIKI_PATH 内存搜索 → 无 wiki 则知识检索关闭
- **风险等级**: 低（新建文件 + 非破坏性修改；SQLiteStore 只读模式；embedder/backend 均为可选注入，未设置时不改变原有行为）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ 60+ 包全绿 | 端到端 `mady serve` 确认 `knowledge: SQLite backend active` ✅

## 2026-07-13: 向量检索落地阶段2 — 暴力查询优化 + Cross-encoder 重排

- **变更**:
  1. **新建 `knowledge/sqlite/vector_index.go`**(~150行)：`VectorIndex` 类型，启动时一次性 `SELECT chunk_id, document_id, vector FROM embeddings` 全量加载 144K 向量到连续 `[]float32`（`unsafe.Slice` 零拷贝 BLOB→float32）；`Search(queryVec, topK)` 并行 goroutine 分片计算点积（利用归一化跳过除法），合并排序取 Top-K
  2. **修改 `knowledge/sqlite/store.go`**：新增 `vecIndex *VectorIndex` 字段 + `PreloadVectors() error` + `HasVectorIndex() bool`；`VectorSearch` 开头检查 `vecIndex != nil` 走 `vectorSearchInMemory` 快速路径，否则回退 SQL 批量读取
  3. **新建 `retrieval/model_rerank.go`**(~200行)：`QueryReranker` 接口（扩展 `Reranker`，新增 `RerankWithQuery(ctx, query, results)`）；`ModelReranker` 类型调 Cohere 兼容 `/v1/rerank` 端点（oMLX Qwen3-Reranker-4B），支持 `MaxDocuments` 截断 + `TopN` 限制 + 降级（API 错误返回原结果）
  4. **修改 `knowledge/extension.go`**：`KnowledgeExtension` 新增 `queryReranker` 字段 + `WithReranker()` 方法；`backendSearch` 在 RRF 融合后检查 reranker：融合 candidateK 个候选 → rerank → 截取 topK
  5. **修改 `cmd/mady/main.go`**：`loadKnowledgeBackend` 中调用 `store.PreloadVectors()`；新增 `buildReranker()`（读 KNOWLEDGE_RERANK/OMLX_RERANK_MODEL）；`loadWikiStore` 中 `ext.WithReranker(reranker)` 接入
  6. **新建 `retrieval/model_rerank_test.go`**：8 个测试覆盖 no-op / 空输入 / 重排序 / API 错误降级 / MaxDocuments 截断 / TopN 限制 / 接口实现
  7. **修改 `knowledge/backend_hook_test.go`**：新增 `TestBackendHook_RerankerApplied` 验证 reranker 在 BeforeModelCall 中被正确调用且重排序生效
- **原因**: 阶段1接线后 VectorSearch 走 SQL 批量读取（144K 向量 ~3.7s），无法满足 <50ms 性能预算；同时启发式 reranker 无 query 语义信息，Top-5 精度不足
- **影响范围**: knowledge/sqlite/vector_index.go(新), knowledge/sqlite/store.go, retrieval/model_rerank.go(新), knowledge/extension.go, cmd/mady/main.go, retrieval/model_rerank_test.go(新), knowledge/backend_hook_test.go
- **环境变量**: 新增 OMLX_RERANK_MODEL(默认 Qwen3-Reranker-4B-4bit-MLX) / KNOWLEDGE_RERANK(默认 off，设为 on 启用)
- **降级策略**: PreloadVectors 失败 → 回退 SQL 批量 VectorSearch；KNOWLEDGE_RERANK=off → 跳过 reranker，直接 RRF topK；rerank API 错误 → 返回原 RRF 结果
- **风险等级**: 中（向量全量加载 ~560MB 内存；reranker 增加 ~200ms 延迟但可关闭）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ knowledge+retrieval 全绿

## 2026-07-13: 向量检索落地阶段2 T2.5 — Benchmark 基线

- **变更**:
  1. **新建 `knowledge/sqlite/bench_test.go`**：底层性能 benchmark（6 项）— PreloadVectorIndex(251ms) / FTSSearch(10.3ms) / VectorIndexSearch(14.5ms 纯计算) / VectorSearchInMemory(15.2ms 含IO) / VectorSearchSQL(1,328ms 对比基线) / GetChunk(5.2μs)
  2. **新建 `knowledge/bench_test.go`**（package knowledge_test）：端到端 benchmark — BackendSearch(29.8ms, FTS+Embed+Vector+RRF) / RRFFusion(4.6μs)；`benchEmbedder` 类型（预计算向量，不依赖 oMLX）
  3. **修改 `knowledge/sqlite/store.go`**：新增 `SampleVector()` 导出方法（从 embeddings 表取一条向量供 benchmark 使用）
  4. **修改 `knowledge/extension.go`**：新增 `Search()` 导出方法（委托 `search()`，供 external test 包调用）
  5. **新建 `docs/specs/vector-retrieval/benchmark-baseline.md`**：完整基线文档，含性能预算对比（全部达标）、耗时分解、并行效率分析、后续优化方向
- **原因**: 需要量化各检索路径性能，验证性能预算（VectorSearch<50ms / 端到端<500ms），建立优化前后的对比基线
- **关键数据**: 内存版 vs SQL 版 87x 加速；预加载 251ms 在 17 次查询后摊销；端到端 29.8ms 远低于 500ms 预算；M4 Pro 14核并行效率 ~14x
- **影响范围**: knowledge/sqlite/store.go, knowledge/sqlite/bench_test.go(新), knowledge/extension.go, knowledge/bench_test.go(新), docs/specs/vector-retrieval/benchmark-baseline.md(新)
- **风险等级**: 低（benchmark 测试文件 + 2 个导出方法，不改变运行时行为）
- **审查要求**: L1
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ 全绿 | 8 项 benchmark 全部产出数据

## 2026-07-13: 向量检索落地阶段3 — WritableStore + 三路 RRF + add_document 工具

- **变更**:
  1. **新建 `knowledge/sqlite/writable.go`**(~310行)：`WritableStore` 类型，读写模式打开 user.db（WAL）；`OpenWritable(path, embedder, knowledgeDBPath)` 建表（documents/chunks/embeddings/docs_fts，同 knowledge.db schema）+ 路径冲突检测（拒绝指向 knowledge.db）；`AddDocument(ctx, docID, title, content)` 分块（`retrieval.ChunkDocument`）→ 批量 Embed(batch=32) → 事务写入（delete 旧 + insert 新）；`Search(ctx, query, topK)` FTS+Vector RRF 融合；`float32ToBytes`/`vecNorm`/`hashString` 辅助函数
  2. **新建 `knowledge/sqlite/writable_test.go`**：11 个测试覆盖创建/FTS命中/无匹配/替换/路径冲突/nil embedder/空docID/并发写/schema幂等/hash/BLOB往返
  3. **修改 `knowledge/extension.go`**：新增 `WritableBackend` 接口（`Search` + `AddDocument`，领域层不 import sqlite）；`KnowledgeExtension` 新增 `writable` 字段 + `WithWritableStore()` 方法；`backendSearch` 新增第三路（user.db Search）参与 RRF 融合；`Tools()` 条件性暴露 `add_document` 工具（writable!=nil 时）；新增 `handleAddDocument` 方法
  4. **修改 `cmd/mady/main.go`**：`loadKnowledgeBackend` 改为返回 `(KnowledgeBackend, string)` 附带 knowledgeDBPath；新增 `openWritableStore(madyHome, embedder, knowledgeDBPath)`（读 USER_DB_PATH → `sqlite.OpenWritable` → 路径冲突检测 → 自动建目录）；`loadWikiStore` 中注入 `ext.WithWritableStore(ws)`
  5. **新建 `knowledge/ext_writable_test.go`**（package knowledge_test）：4 个集成测试 — add_document 工具暴露条件 / add_document→search 端到端命中 / 三路 RRF 融合（mockBackend + realWritable）/ 参数校验
- **原因**: 阶段1-2 完成了 knowledge.db 的只读检索（FTS+Vector RRF+Rerank），但用户无法向知识库添加自有文档。阶段3 新增独立 user.db（同构 schema，WAL 模式），通过 `add_document` 工具写入用户文档，检索时三路 RRF 融合（knowledge FTS + knowledge Vector + user Search），实现用户文档与权威知识库的混合检索
- **影响范围**: knowledge/sqlite/writable.go(新), knowledge/sqlite/writable_test.go(新), knowledge/extension.go, cmd/mady/main.go, knowledge/ext_writable_test.go(新)
- **环境变量**: 新增 USER_DB_PATH(默认 $MADY_HOME/knowledge/user.db)
- **安全**: user.db 路径冲突检测（拒绝指向 knowledge.db）；WAL 模式 + sync.Mutex 单写者；参数化查询防注入；embedder=nil 时 WritableStore 不初始化
- **降级策略**: embedder=nil → WritableStore 不初始化（无 add_document 工具，三路退化为两路）；OpenWritable 失败 → 打印警告继续（不影响 knowledge.db 检索）；user Search 失败 → 跳过该路，用 knowledge FTS+Vector 两路继续 RRF
- **风险等级**: 中（新增写入路径 + 新增工具；user.db 与 knowledge.db 物理隔离 + 路径冲突检测缓解污染风险）
- **审查要求**: L3（安全敏感：writable.go 新增写入沙箱边界）
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ 全部包全绿（含 15 个新测试）
  1. 删除 `agentcore/manifests/chat.json`（embed 源）和 `manifests/chat.json`（根目录用户参考示例）
  2. 更新 `agentcore/manifest_test.go`：5 个测试的硬编码 manifest 数量从 4→3 / 5→4；ExternalOverride 测试从覆盖 chat-agent 改为覆盖 assistant-agent
- **原因**: 提交 `6837337`（Chat Agent 与意图识别深度融合）后，chat-agent 由 `IntegratedChatConfig` 统一动态构建（`domains/chat.go:71`），`ProfessionalHandoffConfigs` 已明确排除 chat（`domains/router.go:80`）。chat.json 作为独立 manifest 已多余，导致启动日志显示不必要的路由项
- **影响范围**: agentcore/manifests/、manifests/、agentcore/manifest_test.go（不影响代码层面的 ChatAgentConfig/IntegratedChatConfig/DomainChat 常量/分类器枚举）
- **风险等级**: 低（集成模式不依赖 chat manifest；Router 模式的 chatHandoff 在代码中硬编码，不依赖 manifest）
- **审查要求**: L1

## 2026-07-13: TUI 案件上下文接入（/case + /deadline 命令族）

- **变更**:
  1. **`cmd/mady/main.go`**: 新增 `currentProject`/`currentProjectMeta` 变量；buildCfg 的 applyPersistence 扩展为注入案件 WorkspaceDir + SystemPrompt 上下文段；新增 /case 命令族（list/info/off/<关键词>切换），按 ProjectID 或 Alias 模糊匹配；新增 /deadline 命令显示当前案件期限；新增 formatProjectContext/formatProjectInfo 辅助函数；slashSuggestions 添加 /case 和 /deadline
- **原因**: 评审文档阶段2核心——让 TUI 用户能选择/切换案件，Agent 运行时感知案件上下文（工作目录、领域、期限）。ProjectRegistry 已就绪，只需 TUI 层接入
- **影响范围**: cmd/mady/main.go（1 个文件，约 130 行新增）
- **风险等级**: 低（复用已测试的 ProjectRegistry API，不涉及安全敏感路径；WorkspaceDir 注入使用 RootPath 字段，已有 sandbox 保护）
- **审查要求**: L1

## 2026-07-13: TUI /export 对话导出

- **变更**: **`cmd/mady/main.go`**: 新增 `/export` 命令（默认导出到 $MADY_HOME/exports/，支持自定义路径）；新增 `formatExportMarkdown` 辅助函数，将 ChatHistory 格式化为 Markdown（含案件信息、角色标签、时间戳）；slashSuggestions 新增 /export
- **原因**: 律师需要导出对话记录作为工作文档，评审文档 3.3 建议
- **影响范围**: cmd/mady/main.go
- **风险等级**: 低（只读导出，不涉及安全敏感路径）
- **审查要求**: L1

## 2026-07-13: TUI /review 审核关卡 + /export 对话导出

- **变更**:
  1. **`cmd/mady/main.go`**: 新增 `reviewMode` 变量；applyPersistence 中当 reviewMode=true 时注入 `domains.NewApprovalGate(domains.DefaultApprovalConfig())` 到 LifecycleChain；新增 `/review` 命令切换审核关卡开关（重建 Agent + 更新状态栏）；slashSuggestions 新增 /review
  2. **`cmd/mady/main.go`**: 新增 `/export` 命令（默认导出到 $MADY_HOME/exports/，支持自定义路径）；新增 `formatExportMarkdown` 辅助函数（Markdown 格式含案件信息+角色标签+时间戳）
- **原因**: 评审文档 3.2（/review 审批）和 3.3（/export 导出）。ApprovalGate 是"提醒式"审批（通过 Agent.Steer 注入审批提示，非同步阻塞），适合作为 TUI 开关命令
- **影响范围**: cmd/mady/main.go
- **风险等级**: 低（ApprovalGate 是已有已测试的 LifecycleHook；/export 是只读文件写入）
- **审查要求**: L1

## 2026-07-13: TUI reasoning 五阶段推理工具接入（阶段3.1）

- **变更**:
  1. **`cmd/mady/main.go`**: 新增 `domains/reasoning` 包导入；applyPersistence 中当 currentProject 不为 nil 时，调用 `reasoning.NewWorkflowRunner()` 创建 FiveStepRunner 并通过 `reasoning.AsWorkflowTool()` 注入为 agentcore.Tool（retriever=nil/llm=nil 的 MVP 模式：有默认模板+L1校验，无知识库检索+L2/L3 LLM校验）；新增 `mapMatterTypeToCaseType()` 辅助函数（8种事项类型模糊匹配→CaseType 枚举）；/case 切换成功后提示推理工具已启用
- **原因**: 评审文档 /sources 建议建立在虚构的 ExecutionResult 上，但 reasoning 包的 Plan/CheckReport/UsedFacts/UsedRules 真实存在。之前 FiveStepRunner 零生产 caller，完整五阶段编排未接入 Agent 运行时。此改动让 TUI Agent 能在案件上下文中调用深度可验证推理
- **影响范围**: cmd/mady/main.go（1 个文件，约 35 行新增）
- **关键复用**: `reasoning.AsWorkflowTool()`（handoff_integration.go:41，已有完整 Tool 适配器）+ `reasoning.NewWorkflowRunner()`（handoff_integration.go:91，已有预配置工厂）
- **风险等级**: 低（复用已有适配器，不修改 reasoning 包源码；Tool 注入是 append 不是覆盖）
- **审查要求**: L1

## 2026-07-13: TUI 会话持久化（JSONL 自动保存 + 分支）

- **变更**:
  1. **`cmd/mady/main.go`**: buildCfg 前创建 FileStore + AgentStore + MemoryCheckpointSaver（优先级：$SESSION_DIR > $MADY_HOME/sessions > ./sessions）；buildCfg 闭包内新增 applyPersistence 辅助函数，每个模式分支（集成/路由/单Agent）统一注入 Store + Checkpoint；OnSubmit goroutine 中 Agent.Run 完成后自动调用 SaveState（用 context.Background() 确保中断后仍可保存）；/new 和 /clear 创建新 ThreadID（tui-{timestamp}）；/branch 实现真正的分支功能（BranchThread + UI 消息恢复）；/save 显示会话保存路径和线程数；slashSuggestions 中 /branch 和 /save 描述更新
- **原因**: 评审文档 P1 阻断项——TUI 之前纯内存模式，重启丢失对话，/save /branch 均提示不支持。复用 serve 模式的 session.FileStore + AgentStore 持久化方案
- **影响范围**: cmd/mady/main.go（1 个文件，约 80 行新增）
- **风险等级**: 低（复用已测试的 session 包，不涉及安全敏感路径；CheckpointSaver 为内存态不持久化，Store 为磁盘 JSONL）
- **审查要求**: L1

## 2026-07-13: TUI 状态栏常驻 + Handoff 文案中文化

- **变更**:
  1. **`cmd/mady/main.go`**: 新增 `statusBarModeLabel()` 辅助函数，生成中文友好的状态栏模式标签（集成/多域路由/🧠 计划 + 推理级别）；初始化时设置状态栏（之前完全缺失）；/thinking 命令后更新状态栏（之前不更新）；/plan 命令统一使用 statusBarModeLabel
  2. **`tui/chat/chat_app.go`**: UpdateStatusBar 格式从 `provider=X model=X mode=X` 简化为 `X/X · 模式标签`；onHandoffStart/onHandoffEnd 文案中文化（"handoff"→"已切换至"、"done"→"已完成"、"handoff failed"→"交接失败"）
- **原因**: 评审文档建议 1.2（/thinking/mode 状态栏常驻）和 1.3（Handoff 显示简化）。状态栏之前初始化时为空，/thinking 不更新；Handoff 英文文案对律师不友好
- **影响范围**: cmd/mady/main.go, tui/chat/chat_app.go
- **风险等级**: 低（UI 文案+状态栏显示逻辑，不涉及安全敏感路径）
- **审查要求**: L1

## 2026-07-12: 文档全面同步 — 552 文件/134K 行/新增 domains/rules + knowledge/sqlite + retrieval/domain

- **变更**:
  1. **CLAUDE.md**: 代码统计（517→552 文件，352→376 非测试，165→176 测试，~126K→~134K 行）；目录结构新增 domains/rules/、knowledge/sqlite/、retrieval/domain/、tools/browser_providers/、pkg/agentconfig/、benchmark/、integration/；agentcore 文件数修正（88+27→75+40，含子包拆分）；依赖列表更新（+modernc.org/sqlite +gopkg.in/yaml.v3）；架构图基础设施层补 knowledge/retrieval/benchmark/integration
  2. **README.md**: 发展路线新增 SQLite 知识库 + RRF 混合检索、YAML 规则引擎 + OA 解析 + 反套话引擎、五步工作法；知识管理段落补充 SQLite 只读取层和 RRF；推理引擎段落补充 domains/rules（OA解析/反套话/法律意图）；扩展表格新增规则引擎行
  3. **CHANGELOG.md**: [0.3.0] 新增 10 项 Added（SQLite 读取层、RRF 混合检索、YAML 规则引擎、OA 解析、反套话引擎、法律意图检测、五步工作法、pkg/agentconfig、browser_providers）
  4. **CONTRIBUTING.md**: 目录结构新增 domains/rules、knowledge/sqlite、retrieval/domain、tools/browser_providers、benchmark、integration、pkg/agentconfig；架构图基础设施层补 benchmark/integration
  5. **docs/knowledge.md**: 架构图补充 KnowledgeBackend + RRF Fuser；新增 SQLite 只读取层段落（3 个数据库表 + RRF 公式）
  6. **docs/adr/0001**: 基础设施层补充 knowledge/retrieval/benchmark/integration；依赖说明补充 modernc.org/sqlite
  7. **docs/chat-assistant-architecture.md**: 新增「v0.3.0 后续迭代（已完成）」10 项
  8. **AGENTS.md**: 核心分层描述更新（+domains/rules +memory +disclosure +ACP）；新增文件数/行数统计
- **原因**: 文档再次滞后于代码进度（代码已 552 文件/~134K 行，文档仍记 517 文件/~126K 行；v0.3.0 新增的 domains/rules + knowledge/sqlite + RRF 混合检索 + OA 解析 + 反套话引擎 + 五步工作法在多份文档中缺失）
- **影响范围**: CLAUDE.md, README.md, CHANGELOG.md, CONTRIBUTING.md, AGENTS.md, docs/knowledge.md, docs/adr/0001, docs/chat-assistant-architecture.md, docs/decisions/AI_CHANGELOG.md
- **风险等级**: 低（纯文档变更，不涉及代码逻辑）
- **审查要求**: L1

## 2026-07-12: XiaoNuno专利能力移植 — OA解析/反套话引擎/法律意图检测

- **变更**:
  1. **新增 `domains/rules/oa_parser.go`**: 审查意见解析器（从XiaoNuo legal-bus/src/rules/oa-parser.ts移植）。纯规则零LLM，3个提取函数：`DetectOaRejectionType`(7组关键词匹配novelty/inventiveness/clarity/support/disclosure/scope/formal)、`ExtractCitations`(正则提取CN/US/WO/EP/JP/KR专利文献号)、`ExtractAffectedClaims`(正则提取权利要求号+范围展开)；入口`ParseOfficeAction`+`FormatOaSummary`
  2. **新增 `domains/rules/slop_engine.go`**: 反AI套话引擎（从XiaoNuo slop-engine.ts 452行完整移植）。三层架构：Layer1短语级(42条正则替换规则，7个分组filler/qualifier/meta/intimacy/subjectless/search/advisory)、Layer2结构级(6种缺陷检测empty_three_step/fake_comparison/binary_turn/reason_pile/passive_voice/oa_formula)、Layer3评分级(50分制5维directness/evidence/rhythm/practicality/concision+8项快检)；入口`AnalyzeSlop`+`FormatSlopAnalysis`
  3. **新增 `domains/legal_intent.go`**: 法律意图细分检测器（从XiaoNuo LegalIntentDetector.ts 270行移植）。`@legal`显式触发+15组关键词→CaseType映射(复用reasoning.CaseType 12种)、专利语境门控(14个信号词)、子串去重(utf8.RuneCountInString)；入口`DetectLegalIntent`+`SelectRunMode`。独立函数，不修改现有ClassifyIntent路由
  4. **修改 `domains/rules/engine.go`**: RulesExtension.Tools()新增2个ReadOnly工具：`parse_office_action`(审查意见解析)、`analyze_slop`(反套话分析)
- **原因**: Mady基础框架完整但缺专利文书规则解析层。XiaoNuo的纯规则解析器从BCIP codex-patent-domain(Rust)移植，天然适合Go重写，零LLM开销
- **影响范围**: domains/rules/oa_parser.go(新), domains/rules/oa_parser_test.go(新), domains/rules/slop_engine.go(新), domains/rules/slop_engine_test.go(新), domains/legal_intent.go(新), domains/legal_intent_test.go(新), domains/rules/engine.go(修改)
- **风险等级**: 低（6个新文件+1个文件追加工具，不修改现有路由/classifier/安全路径）
- **审查要求**: L2

## 2026-07-12: ACP 知识系统集成修复

- **变更**:
  1. **`acp/server_app.go`**: `RunOptions` 新增 `Lifecycle agentcore.LifecycleHook` 字段；`buildAgentConfig` 将其注入 `agentcore.Config.Lifecycle`，使 ACP 创建/重建的 Agent 能携带知识检索等生命周期钩子
  2. **`cmd/mady/main.go`**: `runAcp()` 改为调用 `setupFrameworkContext()`（与 `runTui`/`runServer` 对齐），将 `fc.WikiHook` 通过 `RunOptions.Lifecycle` 传入 ACP 服务器
- **原因**: ACP 入口（`mady acp`）此前完全跳过了 `setupFrameworkContext()`，不加载 Wiki 知识库、不注入 RAG 检索钩子，导致 ACP 用户（如 Zed 编辑器）无法使用知识系统；TUI 和 Serve 已正确集成
- **影响范围**: acp/server_app.go, cmd/mady/main.go
- **风险等级**: 低（新增可选字段，nil 时不改变原有行为；已有测试全部通过）
- **审查要求**: L2

## 2026-07-12: 阶段4 — YAML规则引擎 (domains/rules/)

- **变更**:
  1. **新增 `domains/rules/types.go`**: Go类型系统，覆盖4种YAML格式 — Rule/Check（规则文件）、ArticleFramework/ArticleStep（法条框架）、Orchestration/DiscoveryStage/ExecutionTemplate（事务编排）、ReflectionDomain（反思指示词）；Check使用自定义`UnmarshalYAML`两遍解码：已知字段填充结构体，未知字段保存在`Extra map[string]any`供消费者解释
  2. **新增 `domains/rules/loader.go`**: `LoadFromDir(dir)` 从目录加载全部YAML文件，自动分类（顶层规则文件/articles/*/orchestrations/*/reflection-indicators.yaml），构建索引（rulesByDomain/rulesBySeverity/ruleIndex）
  3. **新增 `domains/rules/engine.go`**: `Engine`查询引擎（AllRules/RuleByID/RulesByDomain/RulesBySeverity/Article/Orchestration/ReflectionIndicators/SearchRules/ToRuleConstraints）+ `RulesExtension`实现agentcore.Extension（ToolProvider+SystemPromptProvider+TransformContextProvider）；暴露3个工具：search_rules、get_article_framework、get_orchestration；ToRuleConstraints将规则转换为reasoning.RuleConstraint供推理框架使用
  4. **新增 `domains/rules/engine_test.go`**: 10个测试覆盖全部功能（加载/Extra字段/域查询/严重度查询/ID查询/搜索/法条框架/编排/反思指示词/RuleConstraint转换）
  5. **依赖**: 添加 `gopkg.in/yaml.v3` v3.0.1（已在go.sum中间接存在，现提升为直接依赖）
- **原因**: XiaoNuo的规则数据（novelty/inventiveness/disclosure/claims/amendment/response 6个顶层规则文件 + 8个法条框架 + 2个事务编排 + 反思指示词）是专利法律推理的核心知识资产，需要在Mady中以Extension机制集成，供Agent通过工具查询规则、法条判断框架和事务编排方案
- **影响范围**: go.mod, go.sum, domains/rules/types.go, domains/rules/loader.go, domains/rules/engine.go, domains/rules/engine_test.go
- **风险等级**: 低（纯新增包，不修改任何现有文件）
- **审查要求**: L2

## 2026-07-12: 代码审查修复 — Context传播/错误处理/FTS5转义/LIKE转义

- **变更**:
  1. **Context传播** (`knowledge/extension.go`): `search`/`backendSearch`/`memorySearch` 方法签名增加 `context.Context` 参数；`handleSearch`/`Provide` 传递调用者ctx；`backendSearch` 中 `e.embedder.Embed` 从 `context.Background()` 改为 `ctx`，支持用户中断时取消嵌入API调用
  2. **NewSQLiteStore错误处理** (`knowledge/sqlite/store.go`): 添加 `db.Ping()` 验证连通性；维度检测查询失败时返回error而非静默回退到dim=1024
  3. **VectorSearch rows.Err()** (`knowledge/sqlite/store.go`): 内层 `rows.Next()` 循环后添加 `rows.Err()` 检查，避免DB错误导致静默返回部分结果
  4. **FTS5引号转义** (`knowledge/sqlite/store.go`): `strconv.Quote(query)` 替换为 FTS5 兼容的双引号包裹+内部双引号加倍（`"` → `""`），避免反斜杠转义导致查询异常
  5. **SearchLaws LIKE转义** (`knowledge/sqlite/store.go`): 转义 `%`→`\%`、`_`→`\_`、`\`→`\\`，添加 `ESCAPE '\'` 子句，确保关键词字面匹配
  6. **backendSearch错误日志** (`knowledge/extension.go`): FTS/Vector/Embed 错误从静默吞没改为 `log.Printf` 记录，便于诊断持续性故障
- **原因**: 代码审查发现6个问题（2中等+4低），涉及context传播缺失、错误静默吞没、SQL注入风险（非安全注入但语义错误）
- **影响范围**: knowledge/extension.go, knowledge/sqlite/store.go
- **风险等级**: 低（修复内部实现细节，不改变公开API）
- **审查要求**: L2

## 2026-07-12: 引入 XiaoNuo 知识系统数据资产 + SQLite 读取层 + RRF 混合检索

- **变更**:
  1. **数据资产引入**: 在 `~/.mady/knowledge/` 下创建符号链接，引入 XiaoNuo Agent 的知识数据（knowledge.db 6.5GB 含81K文档/144K分块/215K图谱节点/144K嵌入向量；laws-full.db 152MB 含9121条法律；patent_kg.db 207MB；ipc-classification/ 6.8MB；wiki/ 17MB；rules/ 76KB）
  2. **SQLite 依赖**: 添加 `modernc.org/sqlite` v1.53.0（纯Go无CGO），更新 go.mod
  3. **SQLite 读取层** (`knowledge/sqlite/store.go`, 419行): `SQLiteStore` 支持只读打开 knowledge.db/laws-full.db/patent_kg.db；`FTSSearch` 利用 FTS5 trigram + BM25 评分；`VectorSearch` 批量读取 BLOB float32 嵌入向量计算余弦相似度；`LoadGraph` 从 kg_nodes/kg_edges 批量加载到内存 GraphStore；`SearchLaws` LIKE 搜索法律库
  4. **RRF 融合检索器** (`retrieval/hybrid.go`): `RRFFuser` 实现 Reciprocal Rank Fusion 算法（k=60），融合 FTS 和向量搜索结果，score-agnostic 只看排名位置
  5. **Extension 集成 SQLite 后端** (`knowledge/extension.go`): 新增 `KnowledgeBackend` 接口（`FTSSearch`/`VectorSearch`）；`WithBackend()` setter 注入 SQLiteStore + Embedder；`search()` 方法优先走 SQLite 后端（FTS+Vector RRF 融合），降级到内存关键词搜索；`handleSearch`/`Provide` 统一调用 `search()` 分发
  6. **测试**: `knowledge/sqlite/store_test.go`（FTS/Graph/Laws 3测试全过）；`retrieval/hybrid_test.go`（RRF 4测试全过）
- **原因**: Mady 原有知识库仅2篇种子文档，无法支撑专利/法律专业领域智能体；XiaoNuo Agent 的数据模型（GraphNode/GraphEdge/节点类型/关系类型/权威度权重）与 Mady 完全对齐，嵌入向量格式兼容（BGE-M3 1024维 float32 LE），可直接复用
- **影响范围**: go.mod, go.sum, knowledge/sqlite/store.go, knowledge/sqlite/store_test.go, retrieval/hybrid.go, retrieval/hybrid_test.go, knowledge/extension.go
- **风险等级**: 低（新增文件+非破坏性修改，现有功能通过 WithBackend 可选注入，不影响默认行为）
- **审查要求**: L2（新增 SQLite 依赖和数据访问层，需确认只读模式和路径安全）

## 2026-07-11: 文档全面同步实际开发进度

- **变更**:
  1. **CLAUDE.md**: 代码统计（419→517 文件，283→352 非测试，136→165 测试，~108K→~126K 行）；目录结构新增 disclosure/memory/agentcore 子包/guardrails/guardian/；架构概要扩展层 10+→35+；新增 Invisible Handoff + IntegratedChatConfig 描述
  2. **CHANGELOG.md**: 版本顺序修正（0.3.0→0.2.0→0.1.0）；补充 0.3.0 缺失特性（Embed Manifest、MADY_HOME、Invisible Handoff、Reasonix 9 包、四级压缩、Permission/Guardian/PlanMode/Evidence/FileCheckpoint/MemoryCompiler/Tracing/Evaluate）；添加 [0.3.0] 链接
  3. **README.md**: 发展路线更新（下季度项中已实现的标记为当前）；架构图补充 memory/；manifest 说明改为 embed + ~/.mady/manifests/；扩展表格新增 8 个 opt-in 扩展包（Evidence/FileCheckpoint/Permission/PlanMode/Guardian/Evaluate/Tracing/Memory）；工具数 40+→35
  4. **SECURITY.md**: 护栏描述修正为实际行为（关键词屏蔽+免责声明+审批门，非"仅免责声明"）；新增 Guardian AI 熔断器 + Permission 权限门控描述；新增安全敏感路径表（12 条路径）；版本表 0.1.x→0.x.x
  5. **docs/chat-assistant-architecture.md**: 后续迭代补充 Invisible Handoff / Embed Manifest / Reasonix 包；下季度候选项更新
  6. **docs/manifest-guide.md**: 文件位置改为 embed + $MADY_HOME/manifests/；启动方式更新
  7. **docs/adr/0001**: TUI 7 层→8 层；基础设施层补充 disclosure/memory/filequeue/fuzzy
  8. **CONTRIBUTING.md**: 目录结构新增 disclosure/memory/filequeue/fuzzy；架构图工具层 10+→35，基础设施层补充新模块
- **原因**: 文档全面滞后于代码实际进度（代码已 517 文件/~126K 行，文档仍记 419 文件/~108K 行；v0.3.0 新增的 12 项特性在多份文档中缺失或描述不足）
- **影响范围**: CLAUDE.md, CHANGELOG.md, README.md, SECURITY.md, docs/chat-assistant-architecture.md, docs/manifest-guide.md, docs/adr/0001-use-layered-architecture.md, CONTRIBUTING.md
- **风险等级**: 低（纯文档变更，不涉及代码逻辑）
- **审查要求**: L1

## 2026-07-11: Chat Agent 与意图识别模块深度融合（Invisible Handoff + IntegratedChatConfig）

- **变更**:
  1. `agentcore/handoff.go`：`HandoffConfig` 新增 `Invisible bool` 字段；`executeDelegate` 中 `Invisible=true` 时不再将子 Agent 事件总线转发到父 Agent
  2. `agentcore/event.go`：`HandoffStartEvent` / `HandoffEndEvent` 新增 `Invisible bool` 字段
  3. `domains/router.go`：提取 `ProfessionalHandoffConfigs()` 共享函数；`AllowedSources` 白名单增加 `"chat-agent"`
  4. `domains/chat.go`：新增 `IntegratedChatConfig(base)` 工厂函数，注册 `ProfessionalHandoffConfigs` 为 Invisible Handoff，SystemPrompt 融合路由指令与对话能力；`ChatAgentConfig` 保持纯聊天向后兼容
  5. `tui/chat/events.go`：`HandoffStartChatEvent` / `HandoffEndChatEvent` 新增 `Invisible bool` 字段
  6. `tui/agentadapter/adapter.go`：透传 `Invisible` 标志
  7. `tui/chat/chat_app.go`：`onToolStart`/`onToolEnd` 跳过 `transfer_to_*` 工具显示；`onHandoffStart`/`onHandoffEnd` 跳过 `Invisible` 交接公告
  8. `cmd/mady/main.go`：新增 `useIntegratedMode`（`MADY_ROUTER_MODE=1` 回退到传统 Router 模式，`MADY_SINGLE_AGENT=1` 回退到单 Agent 模式）；集成模式使用 `IntegratedChatConfig` 作为默认 Agent

- **原因**: Chat Agent 功能单一且意图识别交接过程在 TUI 中可见（`transfer_to_*` 工具调用 + handoff 系统消息 + 子 Agent 实时输出流），影响用户体验。深度融合后 Chat Agent 成为统一对话界面，内部通过 Invisible Handoff 无缝委派专业任务。

- **影响范围**: agentcore/handoff.go, agentcore/event.go, domains/router.go, domains/chat.go, tui/chat/events.go, tui/agentadapter/adapter.go, tui/chat/chat_app.go, cmd/mady/main.go

- **风险等级**: 中（触及 `agentcore/handoff.go` 的安全敏感路径 — HandoffConfig 结构体和 executeDelegate 事件总线逻辑，但 AllowedSources 白名单校验不变，仅新增 Invisible 控制字段）

- **审查要求**: L3（handoff 白名单扩展 + 入口模式切换逻辑需审阅）

## 2026-07-11: 让 mady 在任意工作目录开箱即用（embed manifest + MADY_HOME 统一路径层）

- **变更**:
  1. `pkg/util/paths.go`（新增）：统一路径解析层 `MadyHome()` / `EnsureDir()` / `ResolveDataDir()`，优先级 `$MADY_HOME` > `~/.mady`
  2. `agentcore/embedded_manifests.go`（新增）+ `agentcore/manifests/*.json`（从仓库根 `manifests/` 迁入）：4 个领域 manifest 通过 `go:embed` 编进二进制，任意目录可用
  3. `agentcore/manifest_loader.go`：重构出 `ScanManifestsFromFS(fs.FS)`，新增 `LoadManifests(userDir)` 实现「内置 embed + 外部目录覆盖/新增」合并语义
  4. `cmd/mady/main.go`：`setupFrameworkContext()` 统一走 `util.MadyHome()`，消除 5 处 cwd 相对路径依赖（manifest/workspace/session/AgentStore cwd）；修掉 `main.go:581` 硬编码 `./workspace` 绕过 `WORKSPACE_DIR` 的隐蔽 bug
  5. `agentcore/agent.go` Config 新增 `WorkspaceDir` 字段；`domains/assistant.go` 读取 `base.WorkspaceDir` 替代硬编码 `./workspace`，经 Router 工厂链透传
  6. `Makefile` 新增 `install` target（默认 `PREFIX=~/.local`）
  7. 文档同步：`.env.example` 清理死变量（`KNOWLEDGE_DIR`/`SKILL_DIR`单数）、新增 `MADY_HOME` 说明；`AGENTS.md` 补「资源定位」gotcha
- **原因**: 修复"从非项目根目录启动 `mady tui` 静默降级为裸 LLM 对话"的根因——manifest 扫描依赖相对路径 `./manifests`，目录不存在时 `ScanManifests` 返回 nil 导致 `useMultiDomain=false`，全部领域 agent 能力丢失
- **影响范围**: pkg/util, agentcore(manifest_loader/agent/embedded_manifests), cmd/mady, domains/assistant, Makefile, .env.example, AGENTS.md
- **风险等级**: 中（触及安全敏感路径 `agentcore/manifest_loader.go` 的 Manifest 校验规则，但未改校验逻辑，仅重构加载入口 + 加 embed；`domains/assistant.go` WorkingDir 透传影响工具沙箱边界）
- **审查要求**: L3

## 2026-07-11: 引入 Reasonix 高价值特性 — Phase 0-2 实施

- **变更**: 基于 Reasonix 分析报告，为 Mady 引入 9 个新特性包，全部以 opt-in Extension 模式接入，零侵入现有代码路径：
  1. **Phase 0.1 Tool ReadOnly** (`agentcore/tool.go`): Tool 结构新增 `ReadOnly` 字段 + `DynamicReadOnly` 回调 + `ToolReadOnly()` 辅助函数；`tools/tools.go` 标记 12 个只读工具
  2. **Phase 0.2 Evidence Ledger** (`agentcore/evidence/`): Receipt/Ledger/查询方法/context 注入/Extension 自动注册，追踪每个 turn 的工具调用证据
  3. **Phase 0.3 File Checkpoint** (`agentcore/filecheckpoint/`): Store/Snapshot/Restore + BeforeHook 自动快照写入工具，支持按 turn 回退文件状态
  4. **Phase 1.1 Guardian AI** (`guardrails/guardian/`): AI 安全审查子 Agent，熔断器，三档审查级别，Middleware 集成，fail-closed
  5. **Phase 1.2 Permission System** (`agentcore/permission/`): Allow/Ask/Deny 三态决策 + 规则解析（glob/command prefix）+ Approver 接口 + Middleware
  6. **Phase 1.3 Plan Mode** (`agentcore/planmode/`): 计划模式工具门控，bash 命令安全分类器（read-only/write），LifecycleHook 集成
  7. **Phase 2.1 Tiered Compaction** (`agentcore/context_engine_tiered.go`): 四级渐进式压缩管线（snip→prune→force-fold），注册为 "tiered" ContextEngine
  8. **Phase 2.2 Memory Compiler** (`memory/compiler/`): 策略学习型记忆扩展，ε-greedy 探索，执行轨迹追踪，质量分级 + 置信度衰减，5 个预置专利/法律策略
- **原因**: 系统性提升 Agent 安全性、上下文管理效率、和学习能力，借鉴 Reasonix 工程实践
- **影响范围**: agentcore/{tool.go, evidence/, filecheckpoint/, permission/, planmode/, context_engine_tiered.go, context_engine.go, context_engine_test.go}, tools/tools.go, guardrails/guardian/, memory/compiler/
- **安全敏感**: 是（涉及 Permission 门控、Guardian 审查、Plan Mode 工具门控、文件系统操作）
- **验证**: go build ✅ | go test -race ✅ 全部通过
- **风险等级**: 中（新功能均为 opt-in，不影响现有代码路径）
- **审查要求**: L3

## 2026-07-11: 修复三个 CRITICAL 并发安全问题

- **变更**:
  1. `domains/agent_pool.go` GetOrCreate 消除 defer+手动 Unlock 混合模式导致的 double-unlock panic，改为显式 Lock/Unlock + 锁外批量 Close
  2. `domains/reasoning/fact_blackboard.go` 为 FactBlackboard 添加 sync.RWMutex 保护所有字段，写方法检查 Locked 并 panic，MarshalJSON/UnmarshalJSON 加锁
  3. `domains/project.go` 提取 StatusActive/StatusArchived/StatusUnreachable 常量替换硬编码字符串
- **原因**: 消除运行时 panic 风险和并发数据竞争
- **影响范围**: domains/agent_pool.go, domains/reasoning/fact_blackboard.go, domains/project.go
- **风险等级**: 中（涉及安全敏感路径 agent_pool 和并发同步）
- **审查要求**: L3

## 2026-07-11: 全面代码质量审查修复 — 16 CRITICAL + 45 MAJOR + lint清零

- **变更**:
  1. **CRITICAL 安全修复**: tools/ delete.go/move.go/patch.go 改用 resolvePathSandboxed 堵住沙箱绕过；tools.go BuildTools 传播 Sandbox 配置；bash.go 添加 Setpgid 进程组隔离 + 临时文件延迟清理 + Write 错误检查
  2. **CRITICAL 并发/泄漏修复**: agentcore/stream.go Map/Merge 添加 out.Done() 监听取消 goroutine 泄漏；session/session.go 锁缓存改 LRU 淘汰替代全量清空；knowledge/store.go ReindexVectors 锁外批量 Embed；server/server.go handleSkillEvents defer unregister；tui/tui.go PanicMsg 处理 + terminal.go readLoop 错误日志 + 写错误记录
  3. **MAJOR agentcore 修复**: 删除死代码(`_ = tc`/tmpState)；compaction 失败时清空 previousSummary；runStreaming 添加 recover；提取 buildRequestMessages 辅助函数；handoff_context 全局 goroutine 简化 + 移除 intentCacheStopCh；handoff.go fmt.Printf → slog；新增 messagesNoClone 内部方法；agent.go map 直接访问改为 Create 调用
  4. **MAJOR tools 修复**: process.go handleKill/handleList 从 stub 改为 Registry 实现；handleStatus/handleWait 从 registry 查真实 entry；browser.go Stealth JS 改用 AddScriptToEvaluateOnNewDocument；find.go WalkDir 深度限制 5 层；grep.go Kill 后立即 Wait
  5. **MAJOR 网络层修复**: a2a PublishTaskUpdate/ReadLoop 事件丢弃添加 slog；SSEKeepAlive 添加 mu 参数；disclosure SSE 添加写锁；mcp/client.go tryReconnect 递归深度限制 3
  6. **MAJOR 基础设施修复**: store/file.go + psychological/store.go 原子写入(tmp+rename)；filequeue RWMutex 替代 Mutex；session persistEntry O(1) hasAssistant 标志；session readInfo 加锁；knowledge/graph 手写 intToStr/floatToStr → 标准库
  7. **MAJOR 其他修复**: guardrails 免责声明完整文本匹配；psychological SDT 权重归一化；disclosure 重试时删除三个提取 key；cmd/mady log.Fatalf → return；example a2a-client/a2a-server signal handling
  8. **Lint 清零**: 18 个 golangci-lint issues 全部修复（dupArg/appendCombine/exitAfterDefer/gofmt/ineffassign/QF1008/QF1012/S1005/SA9003/unconvert/unused）
  9. **代码重复消除**: 4 处 itoa → strconv.Itoa；3 处 lastUserMessage → agentcore.LastUserMessage；2 处 validateKey → util.ValidateKey
- **原因**: 系统性消除审查报告中的 16 CRITICAL / 45+ MAJOR / golangci-lint 问题
- **影响范围**: 全项目（agentcore/tools/domains/session/knowledge/server/tui/a2a/mcp/disclosure/guardrails/psychological/store/filequeue/workflow/cmd/example）
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ 全部通过 | golangci-lint 0 issues
- **风险等级**: 中（涉及安全敏感路径 tools/path 沙箱 + handoff + guardrails）
- **审查要求**: L3

## 2025-06-11: 初始化代码质量全面审查报告

- **变更**: 完成 Mady 项目首次全面代码质量审查，覆盖 484 个文件的 6 大维度
- **原因**: 系统性识别性能瓶颈、安全漏洞、架构合规性问题，支撑智能体高效调用
- **审查结果**: 审查报告已输出至 `docs/decisions/REVIEW_REPORT_2025-06-11.md`
- **风险等级**: 中（大量安全/性能问题需修复）
- **审查要求**: L2

## 2026-07-13: 向量检索端到端验证修复 — Dimensions 修正 + Extension 注册暴露工具

- **变更**:
  1. **修正 `retrieval/embedding.go` `Dimensions()` 方法**：bge-m3 系列模型未在已知列表中，default case 返回 1536 导致 WritableStore schema 建为 1536 维，与实际 1024 维向量不匹配（`vector dim mismatch: got 1024, want 1536`）。添加 `strings.Contains(strings.ToLower(e.Model), "bge-m3") → return 1024` 判断
  2. **Extension 注册到 `cfg.Extensions` 暴露工具**：`loadWikiStore` 新增第三个返回值 `agentcore.Extension`（KnowledgeExtension），`frameworkContext` 新增 `KnowledgeExt` 字段，`buildCfg` 3 分支（集成/路由/单Agent）+ `runServer` + `runAcp` 均注入 `cfg.Extensions`。此前 Extension 只返回了 BackendHook（LifecycleHook），`Tools()` 方法从未被调用，`search_knowledge` 和 `add_document` 工具未暴露
  3. **`acp/server_app.go`**：`RunOptions` 新增 `Extensions []agentcore.Extension` 字段，`buildAgentConfig` 传递到 `agentcore.Config.Extensions`
  4. **`cmd/mady/main.go`**：新增 `extSlice()` 辅助函数（nil 安全的单 Extension → slice 转换）
- **原因**: 端到端测试发现两个问题 — (1) user.db 向量搜索维度不匹配 (2) add_document 工具未被 agent 识别
- **影响范围**: `retrieval/embedding.go`、`cmd/mady/main.go`、`acp/server_app.go`
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ 全绿 | 端到端：`mady serve` + oMLX → add_document 写入 → search_knowledge 检索命中 → 日志零报错
- **风险等级**: L2（Dimensions 修正影响所有 APIEmbedder 调用方；Extension 注册改变 agent 工具集）
- **审查要求**: L2

## 2026-07-13: 代码审查修复 — 跨数据库 chunk ID 冲突 + buildReranker 空值检查

- **变更**:
  1. **修复 `knowledge/sqlite/writable.go` chunk ID 冲突**：`ftsSearch` 和 `getChunk` 中的 `ID: strconv.Itoa(id)` 改为 `ID: "u:" + strconv.Itoa(id)`。knowledge.db 和 user.db 是独立的 SQLite 数据库，各自的 AUTOINCREMENT 序列都从 1 开始。`RRFFuser.Fuse`（`retrieval/hybrid.go:44`）用 `r.ID` 字符串去重，两个数据库的相同数字 ID 会被误判为同一 chunk，导致 RRF 分数错误累积和结果静默丢失
  2. **修复 `cmd/mady/main.go` `buildReranker` 空值检查**：文档字符串声明"OMLX_API_KEY 未设置返回 nil"，但代码未检查空值。添加 `if apiKey == "" { return nil }` 使实现与文档一致
  3. **新增回归测试 `TestExtension_CrossDBIDNoCollision`**：模拟 knowledge.db 返回数字 ID "1" + user.db 也有 chunk ID 1，验证两者在 RRF 融合后均独立出现（不被错误合并）
- **原因**: 代码审查（task review）发现三路 RRF 融合中的跨数据库 ID 冲突 bug — 当 user.db 配置启用时（`OMLX_API_KEY` 已设置 + `add_document` 被调用），搜索结果会静默损坏
- **影响范围**: `knowledge/sqlite/writable.go`（2处 ID 前缀）、`cmd/mady/main.go`（buildReranker 空值检查）、`knowledge/ext_writable_test.go`（新增回归测试）
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ 全绿（含新测试 `TestExtension_CrossDBIDNoCollision`）
- **风险等级**: L2（chunk ID 格式变更影响 RRF 去重行为，但仅限 user.db 路径；knowledge.db 路径不变）
- **审查要求**: L2
