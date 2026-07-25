# Phase 4 审阅：R16 LLM 出站 PII 路径 + R17 跨租户/Workspace 隔离回归 — 2026-07-25

> Phase 4 子审阅｜依据：`Mady 全面审阅计划 v1.0` ｜执行者：AI（Grok）｜Human Owner：[NEEDS CLARIFICATION]
> 审阅窗口：数据隐私相关模块（memory/session/filecheckpoint/knowledge/retrieval/orchestration/tasklist）

## 摘要

本次专项聚焦两条数据隐私主线：

**R16 LLM 出站 PII：** 共定位到 **22 条**用户/案件数据送外站 LLM Provider 的出站路径。核心结论是：**除 `memory/extractor_llm.go` 对凭证做了有限正则过滤外，其余路径均无 PII 脱敏**。最严重的是 Agent 主循环、上下文压缩、会话摘要、记忆去重、Embedding 全链路——当事人姓名、案件号、专利交底书、地址等 PII 会原样进入外部 API。

**R17 跨租户/Workspace 隔离：** 发现 **2 个 H 级**隔离缺口：
- `memory.db` 是全局单库，且 `MemoryExtension` 检索只按 `UserID` 过滤，**同一用户在不同项目间的记忆会互相注入**。
- `tasklist` 的任务文件写入全局目录 `~/.mady/sessions/tasks`，使用顺序数字 ID，**任何 workspace 可枚举/读写其他 workspace 的任务**。

另有若干 M/L 级问题（serve 模式 session 未分区、filecheckpoint 根目录固定等）和大量合规观察（fileindex/approval/workflow 分库良好）。

## 1. R16 发现清单：LLM 出站 PII 路径

| ID | 文件:行 | 路径 | 发送数据 | 脱敏现状 | 风险 | 建议 |
|----|--------|------|---------|---------|------|------|
| **R16-01** | `memory/extractor_llm.go:42-58` | Memory LLM 提取 | 完整 `conversation`（用户输入 + 助手回复） | 仅 `sensitiveDataFilter` 替换 password/api_key/secret/token/JWT；姓名、案件号、交底书内容不脱敏 | **H** | 扩展 redactor 覆盖 PII；或前置独立 PII redactor |
| **R16-02** | `memory/extractor_llm.go:75-86` | Memory LLM 提取 fallback | 同上 | 同上 | **H** | 复用同一 redactor；fallback 不应绕过策略 |
| **R16-03** | `memory/session_summarizer.go:30-53` | Session 摘要 | `sessionMemories` 拼接后的 `conversationText` | **无**（grep `sensitiveDataFilter` 零命中） | **H** | 摘要前接入统一 redactor |
| **R16-04** | `memory/dedup_llm.go:30-50` | Memory 去重 | `newFact` + 已有记忆 `existing[].Entry.Content` | **无** | **H/M** | 对 fact 与现有记忆均 redact；去重可本地化/哈希化 |
| **R16-05** | `memory/extension.go:294` → `manager.go:127` | AfterModelCall AutoExtract | 最新一轮 `userMsg` + `respContent` | 仅 credential 过滤 | **H** | 拼接前单轮消息 redact；默认关闭 AutoExtract |
| **R16-06** | `retrieval/embedding.go:74-90` | APIEmbedder | `texts` 数组（查询或文档分片）POST `/embeddings` | **无** | **H** | Embedding 前 redact；或提供本地 Embedding 默认方案 |
| **R16-07** | `memory/sqlite_store.go:133/201/251` | Memory Store Embedding | 记忆内容 `content` / `e.Content` / `query` | **无** | **H** | 在 `Remember`/`Search` 层统一 redact |
| **R16-08** | `memory/store.go:266/311/338` | InMemory Store Embedding | 同上 | **无** | **H** | 同上 |
| **R16-09** | `retrieval/vector.go:46` | Vector Search | 检索 query | **无** | **H** | 检索入口 redact query |
| **R16-10** | `knowledge/extension.go:421` | Knowledge Search | 用户检索 query | **无** | **H** | 同上 |
| **R16-11** | `knowledge/sqlite/writable.go:171` | Writable Store Index | 用户文档分片 `chunks[i].Content` | **无** | **H** | 文档入库前 redact；提供显式"可出域"开关 |
| **R16-12** | `knowledge/sqlite/writable.go:279` | Writable Store Search | 用户检索 query | **无** | **H** | 同上 |
| **R16-13** | `knowledge/store.go:288` | Knowledge ReindexVectors | 所有 searchable chunk 的 `chunk.Content` 批量发送 | **无** | **H** | 批量重建强制 redact |
| **R16-14** | `agentcore/agent_provider.go:79/89` | Agent 主循环 | 完整 `req.Messages` + tool definitions + tool results | **无**（汇聚点） | **H** | Agent 层 outbound 前统一 redact `Message.Content`/tool args/tool results |
| **R16-15** | `agentcore/compaction.go:462` | Context Compaction | `turnsToSummarize` 历史消息拼接文本 | 仅 `sanitizeToolPairs` 移除孤立 tool result，不脱敏 | **H** | 压缩前 redact；摘要文本不得保留原始 PII |
| **R16-16** | `agentcore/handoff_context.go:115` | Handoff 意图摘要 | 最近消息拼接 `fullText`（截断 2000 runes） | **无** | **H/M** | 摘要前 redact；`HandoffContext.RecentMessages` 也应脱敏 |
| **R16-17** | `agentcore/orchestration_executor.go:88` | Orchestration 编排 | 工具输入/输出经 Agent 间接进入 LLM | 依赖 Agent 层（当前无） | **H** | orchestration state 注入 redactor；审查 tool I/O |
| **R16-18** | `domains/classifier.go:151` | Domain 分类器 | 用户原始输入 `input` | **无** | **M** | 分类前 redact；可考虑本地轻量分类器 |
| **R16-19** | `disclosure/keywords.go:108` | Disclosure 关键词 | 交底书文本 `sb.String()` 全文 | **无** | **M/H** | 提取关键词前对案件文本 redact |
| **R16-20** | `guardrails/guardian/guardian.go:109` | Guardian AI Review | 工具名 + `args` + `transcript` | **无** | **M** | Guardian 审查文本含 tool args 时 redact；Provider 与业务 Provider 分离 |
| **R16-21** | `tools/vision.go:338` | Vision Tool | 用户 prompt + base64 图像 | **无** | **M** | prompt redaction；图像场景需显式告知用户出域 |
| **R16-22** | `provider/chatcompat/chat.go:631` | HTTP Transport | 上述所有请求序列化后的 JSON body | **无**（最终汇聚点） | **H** | 序列化前完成 redaction；审查 provider error body 是否回显原文 |

## 2. R16 配置开关现状

| 功能 | 配置项 | 默认值 | 说明 |
|------|--------|--------|------|
| LLM 事实提取 | `MADY_MEMORY_AUTO_EXTRACT` | `""`（关闭） | `=1` 启用 Extractor + Summarizer（`cmd/mady/framework.go:610/652`） |
| Embedding | `EMBEDDING_BASE_URL` | `""`（关闭） | 未设置时退化为关键词检索；设置后启用 `APIEmbedder`（`cmd/mady/framework.go:576`） |
| Agent 主循环 LLM | 无独立开关 | 始终启用 | 只要配置 Provider，用户消息即出域 |

**关键观察**：记忆/摘要/Embedding 默认关闭，风险可控；但 **Agent 主循环、上下文压缩、Handoff 摘要、领域分类器等核心路径无独立关闭开关**，用户无法通过环境变量阻止对话内容出域。

## 3. R17 发现清单：跨 Workspace / 租户隔离

| ID | 模块 | 文件:行 | 隔离现状 | 风险 | 证据 | 建议 |
|----|------|--------|---------|------|------|------|
| **R17-01** | memory SQLite 存储 | `cmd/mady/framework.go:571` | 全局单库 `~/.mady/memory.db`，所有 workspace 共享同一 `SQLiteMemoryStore` | **H** | `memoryDB := filepath.Join(fc.MadyHome, "memory.db")` → `memory.NewSQLiteMemoryStore` | 强制所有查询携带 `ProjectID` 过滤；或改为按 project 分库 |
| **R17-02** | MemoryExtension 检索过滤 | `memory/extension.go:202` `memory/extension.go:240-250` `memory/tools.go:152-158` | `Provide`/`TransformContext`/`handleRecall` 只构造 `MemoryFilter{UserID: ...}`，**未加入 ProjectID/SessionID** | **H** | `filter := MemoryFilter{UserID: e.scope.UserID, TopK: e.cfg.TopK}` | 记忆注入与 recall 统一使用 `UserID + ProjectID + SessionID` 全维度过滤 |
| R17-03 | memory schema | `memory/sqlite_store.go:52-107` | schema 已含 `user_id/agent_id/session_id/project_id` 及复合索引 `idx_memories_scope`，具备行级隔离基础 | L | 索引存在 | 保留 schema；确保查询层强制使用这些字段 |
| R17-04 | session TUI 存储 | `cmd/mady/tui_storage.go:77-120` | 按 CWD 分区：`~/.mady/sessions/by-cwd/<sha256(cwd)[:16]>/` | L | 代码按 cwd hash 分区 | 保持；建议 server 模式也按 workspace 分区 |
| R17-05 | session serve 存储 | `cmd/mady/server.go:128-147` | `SESSION_DIR` 或 `~/.mady/sessions` 单目录，所有会话混放 | M | `session.NewFileStore(sessionDir)` | serve 模式按 `fc.WorkspaceDir` 分区存储 |
| R17-06 | session ID / 路径遍历 | `session/session_store.go:569-570` `pkg/util/util.go:45-52` | session ID 为 `<UnixNano>_<counter>`，难以猜测；`ValidateKey` 禁止 `.`/`..` | L | ID 生成与 key 校验代码 | 保持；server 模式建议按 workspace 物理分区 |
| R17-07 | filecheckpoint 路径安全 | `agentcore/filecheckpoint/store.go:228-255` | `isWithinRoot` 用 `filepath.Abs`+`filepath.Rel` 拦截 `..` 与外部绝对路径 | L | `rel, err := filepath.Rel(rootAbs, abs)` | 补充 `filepath.EvalSymlinks` 降低 symlink 绕过可能 |
| R17-08 | filecheckpoint 根目录固定 | `cmd/mady/framework.go:480-491` `cmd/mady/tui_session_agent.go:216-240` | `filecheckpoint.NewExtension(toolWorkingDir)` 只在启动时创建一次；TUI 切换项目后 `rebuildAgent` 未重新创建扩展 | M | `toolWorkingDir := fc.BaseConfig.ProjectDir`；`rebuildAgent` 直接使用 `buildAgentConfig()` | 在 `buildAgentConfig`/`applyPersistence` 中按当前 project 重新创建 FileCheckpointExtension；或让 Store 支持动态 root |
| R17-09 | knowledge SQLite | `cmd/mady/knowledge.go:200-220` | `~/.mady/knowledge/knowledge.db`，全局只读预构建语料库 | L（按设计） | `dbPath := filepath.Join(dbDir, "knowledge.db")`；`mode=ro` | 保持全局只读设计；确保写入接口不会接入 |
| R17-10 | retrieval/domain/sqlite | `retrieval/domain/sqlite/patent_retriever.go:37-46` | 只是 `knowledge.db` 的只读 FTS 包装 | L（按设计） | `NewPatentDomainRetriever(store)` 绑定全局 `SQLiteStore` | 保持 |
| **R17-11** | tasklist FileStore | `cmd/mady/framework.go:504-509` `agentcore/tasklist/filestore.go:27-73` | 全局目录 `~/.mady/sessions/tasks`，顺序数字 ID | **H** | `taskDir = filepath.Join(taskDir, "tasks")`；`tasklist.NewExtension(taskDir)`；路径 `baseDir/<id>.json` | 按 workspace/project 分目录；或在 task ID 中嵌入 workspace 标识并校验 |
| R17-12 | fileindex | `cmd/mady/tui_session.go:358-366` | 按 project 分库：`WorkspaceDir/projects/<projectID>/fileindex.db` | L | `dbPath := filepath.Join(wsDir, "projects", projectID, "fileindex.db")` | 保持 |
| R17-13 | approval store | `cmd/mady/tui_session.go:630-640` | 每个 workspace 一个 `approvals.db`，内部按 `case_id/session_id` 分表 | L | `p, err := s.dbPath("approvals.db")` | `Load(id)` 增加 `case_id` 校验 |
| R17-14 | reasoning workflow checkpoint | `cmd/mady/tui_session_config.go:740-752` | 每个 workspace 一个 `workflow_checkpoints.db`，schema 含 `case_id` 索引 | L | `dbPath := filepath.Join(base, "workflow_checkpoints.db")` | `Load` 增加 `case_id` 校验 |
| R17-15 | graph checkpoint | `cmd/mady/tui_session.go:646-651` | 每个 workspace 一个 `graph_checkpoints.db`，按 `graph_id` 索引 | L | `dbPath` via `s.dbPath(...)` | `Load` 增加 `graph_id` 校验 |
| R17-16 | store/SnapshotStore | `store/file.go:15-24` | 仅测试使用，生产代码未调用 `NewSnapshotStore` | L | grep 仅命中 `store/file_test.go` | 若未来启用，确保按 workspace 分目录 |

## 4. R17 详细说明

### 4.1 memory：全局单库 + 过滤条件缺失 → 跨项目记忆泄漏（R17-01 / R17-02）

- 存储位置：`cmd/mady/framework.go:571` 把 `memory.db` 固定放在 `fc.MadyHome` 根目录，而不是 `workspace/projects/<id>/` 下。
- schema 已有隔离字段：`memory/sqlite_store.go:89-105` 包含 `user_id`、`agent_id`、`session_id`、`project_id` 及复合索引。
- 但查询过滤不完整：`memory/extension.go:202` / `:240-250` / `memory/tools.go:152-158` 均只使用 `UserID` 构造 `MemoryFilter`。
- 影响：同一台机器上同一用户在不同项目间的记忆会被注入到彼此对话上下文中。对专利/法律场景，这属于 workspace 间数据混用，违反最小权限原则。

**建议**：在 memory 注入与 recall 中统一使用 `UserID + ProjectID + SessionID` 全维度过滤；若需保留"跨项目用户偏好"，应显式把 `LayerUser` 与 `LayerSession/LayerLongTerm` 区分策略并文档化。

### 4.2 tasklist：全局任务目录 + 顺序数字 ID（R17-11）

- `cmd/mady/framework.go:504-509` 把 tasklist 扩展初始化在 `~/.mady/sessions/tasks`。
- `agentcore/tasklist/filestore.go` 用 `.nextid` 文件维护顺序 ID，任务文件名为 `<id>.json`。
- 影响：Agent 通过 `task_get`/`task_list`/`task_update` 只要枚举数字 ID，即可读写其他 workspace 的任务。

**建议**：把 tasklist 目录改为按 workspace 分区，例如 `~/.mady/sessions/by-cwd/<hash>/tasks` 或 `WorkspaceDir/projects/<projectID>/tasks`。

### 4.3 session：TUI 分区 vs serve 模式未分区（R17-04 / R17-05）

- TUI 模式已按 CWD hash 分区（`~/.mady/sessions/by-cwd/<hash>/`）。
- serve 模式使用单一目录（`$SESSION_DIR` 或 `~/.mady/sessions`），所有会话混放。
- session ID 依赖 `UnixNano` 难以猜测，`ValidateKey` 阻止路径穿越，直接风险较低，但物理隔离原则不满足。

**建议**：serve 模式按 `fc.WorkspaceDir` 或 projectID 分区。

### 4.4 filecheckpoint：路径安全正确，但实例根目录固定（R17-07 / R17-08）

- `isWithinRoot` 已拦截 `..` 与外部绝对路径，但未解析符号链接。
- `filecheckpoint.NewExtension` 在启动时只创建一次，TUI 切换项目后 `rebuildAgent` 未重新创建扩展；根目录保持为初始 cwd。
- 影响 1（功能退化）：切换项目后新项目的文件编辑会被判定为"逃逸 root"，无法快照。
- 影响 2（跨项目风险）：若启动 cwd 是多个项目的公共父目录，子项目文件状态可能互相污染。

**建议**：在 `buildAgentConfig` 中按当前 project 重新创建 FileCheckpointExtension；或让 Store 支持动态 root。

## 5. 已验证合规项

| 项 | 证据 |
|----|------|
| ✅ memory schema 预留隔离字段 | `memory/sqlite_store.go:89-105` 含四维索引 |
| ✅ session ID 不可猜测 | `session/session_store.go:569` 使用 `time.Now().UnixNano()` |
| ✅ session key 路径穿越防护 | `pkg/util/util.go:45-52` `ValidateKey` 禁止 `.`/`..` |
| ✅ filecheckpoint 显式 root 逃逸检查 | `agentcore/filecheckpoint/store.go:228-255` `isWithinRoot` |
| ✅ fileindex 按 project 分库 | `cmd/mady/tui_session.go:358-366` `WorkspaceDir/projects/<projectID>/fileindex.db` |
| ✅ approval/workflow/graph checkpoint 按 workspace 分库 | `cmd/mady/tui_session.go:630-651` 分别使用 `s.dbPath(...)` |
| ✅ knowledge 全局只读（按设计） | `cmd/mady/knowledge.go:200-220` `mode=ro` |
| ✅ Embedding 默认关闭 | `cmd/mady/framework.go:576` 仅在 `EMBEDDING_BASE_URL` 设置时启用 |
| ✅ memory auto-extract 默认关闭 | `cmd/mady/framework.go:610/652` 仅在 `MADY_MEMORY_AUTO_EXTRACT=1` 时启用 |
| ✅ 凭证过滤已落地 | `memory/extractor_llm.go:96-110` 过滤 password/api_key/secret/token/JWT |

## 6. 建议处置优先级

| 发现 | 建议动作 | 工时估算 |
|------|---------|---------|
| R17-02 memory 查询加 ProjectID/SessionID 过滤 | 修改 `memory/extension.go`、`memory/tools.go` 三处 filter；补测试 | 0.5d |
| R17-11 tasklist 按 workspace 分目录 | 修改 `cmd/mady/framework.go` 初始化位置 + `agentcore/tasklist/filestore.go` ID 空间；补迁移/兼容性说明 | 0.5d |
| R16-03 session summarizer 接入 redactor | 在 `memory/session_summarizer.go` 调用 sensitiveDataFilter 或统一 redactor | 0.3d |
| R16-14 Agent 主循环 outbound redaction | 在 `agentcore/agent_provider.go` 请求 provider 前统一 redact message/tool 内容 | 1d |
| R17-08 filecheckpoint 动态 root | 在 `buildAgentConfig` 中按当前 project 重建 extension | 0.5d |
| R17-05 serve 模式 session 分区 | 修改 `cmd/mady/server.go` 的 `sessionDir` 构造 | 0.3d |
| 其他 R16/M/L | 按统一 PII redaction 规划分批实施 | 2-3d |

## 7. 规范建议

1. 修订 `docs/data-privacy-standards.md`，新增一节"LLM  outbound 数据最小化"：明确默认哪些数据会出域、哪些不会、如何关闭/脱敏。
2. 在 `docs/decisions/AI_CHANGELOG.md` 记录新增统一 redaction 层的设计决策（注意 R15 发现的 5 字段格式问题）。

---

> 本报告高危结论（R16-03、R17-01、R17-02、R17-11）已由主审通过 grep/read_file 抽样复核确认。
