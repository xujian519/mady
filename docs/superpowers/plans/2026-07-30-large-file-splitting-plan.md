# 超大文件拆分实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 70 个 ≥500 行的 Go 源码文件拆分为职责单一的小文件，提升代码可维护性和可读性。

**Architecture:** 按 Phase 逐步推进。每个拆分保持 package 不变，仅移动代码位置，不改变行为。验证采用现有测试套件（非 TDD 新写测试）。Phase 1 快速收割有明显边界的文件，Phase 2 深入模块分析，Phase 3 收尾。

**Tech Stack:** Go 1.26, go build, go test -race, go vet

**设计文档:** [2026-07-30-large-file-splitting-design.md](../specs/2026-07-30-large-file-splitting-design.md)

---

## 全局约束

1. **每个文件拆分后立即验证**：`go build ./...` + `go test -race ./<pkg>/...` + `go vet ./<pkg>/...`
2. **不改变行为**：只移动代码，不"顺便优化"逻辑、命名、错误处理
3. **保持 package 不变**：拆分后的文件声明同一个 `package`
4. **导出符号位置不变**：原来在哪个文件导出，拆分后仍然导出（通过新文件 re-export 或直接保留）
5. **commit 粒度**：一个文件拆分 = 一个 commit

---

## Phase 1：快速收割（15 文件）

### Task 1: 拆分 `domains/workflows/patent/rule_engine.go`（1203 行）

**目标:** 按专利领域拆分规则构造函数，每个领域一个文件。

**文件:**
- 修改: `domains/workflows/patent/rule_engine.go`（保留 RuleEngine struct + 核心逻辑）
- 创建: `domains/workflows/patent/rule_inventiveness.go`
- 创建: `domains/workflows/patent/rule_novelty.go`
- 创建: `domains/workflows/patent/rule_infringement.go`
- 创建: `domains/workflows/patent/rule_reexamination.go`
- 创建: `domains/workflows/patent/rule_invalidation.go`
- 创建: `domains/workflows/patent/rule_design.go`
- 创建: `domains/workflows/patent/rule_reasoning.go`
- 创建: `domains/workflows/patent/rule_subject_matter.go`

- [ ] **Step 1: 读取文件并识别所有独立函数**

使用 codegraph_node 或 Read 查看 `domains/workflows/patent/rule_engine.go`，列出所有 `*Rules()` 构造函数及其行号范围。

- [ ] **Step 2: 逐个提取规则函数到新文件**

按以下映射逐文件创建（每创建一个验证编译一次）：
- `InventivenessRules()` → `rule_inventiveness.go`
- `NoveltyRules()` → `rule_novelty.go`
- `InfringementRules()` → `rule_infringement.go`
- `ReexaminationRules()` → `rule_reexamination.go`
- `InvalidationRules()` → `rule_invalidation.go`
- `DesignRules()` → `rule_design.go`
- `ReasoningPatternRules()` → `rule_reasoning.go`
- `SubjectMatterRules()` → `rule_subject_matter.go`
- `DefaultPatentRules()` → 保留在 `rule_engine.go`

每个新文件包含：`package patent` + 必要的 import（从原文件复制对应部分）。

- [ ] **Step 3: 验证编译**

```bash
go build ./domains/workflows/patent/...
```

- [ ] **Step 4: 运行测试**

```bash
go test -race ./domains/workflows/patent/...
```

- [ ] **Step 5: 静态检查**

```bash
go vet ./domains/workflows/patent/...
```

- [ ] **Step 6: 提交**

```bash
git add domains/workflows/patent/rule_*.go
git commit -m "refactor(patent): 拆分 rule_engine.go 按专利领域分离规则构造函数"
```

---

### Task 2: 拆分 `bootstrap/setup.go`（1195 行）

**目标:** 类型定义独立 + 各领域初始化函数分离。

**文件:**
- 修改: `bootstrap/setup.go`（保留 Setup + Options + Context 类型）
- 创建: `bootstrap/init_knowledge.go`（LoadWikiStore, ResolveWikiRoot）
- 创建: `bootstrap/init_skills.go`（DiscoverSkills, LoadManifests, DiscoverMCP）
- 创建: `bootstrap/init_memory.go`（InitMemorySystem）
- 创建: `bootstrap/init_reasoning.go`（InitReasoningAndTemplates, BuildReasoningRetriever, loadWorkflowManifests）
- 创建: `bootstrap/init_workspace.go`（InitWorkspace, BuildBaseTools）
- 创建: `bootstrap/init_plugins.go`（InitPlugins）
- 创建: `bootstrap/init_search.go`（SearchGuidelines, SearchSimilarCases, SearchLegalProvisions）
- 创建: `bootstrap/fallback.go`（LoadFallbackConfig, BuildCitationSource）

- [ ] **Step 1: 读取文件结构**

使用 Read 查看 `bootstrap/setup.go`，确认每个函数的行号范围及其依赖关系。

- [ ] **Step 2: 逐文件创建，每创建一个验证编译**

按上述映射，逐个创建新文件。每个新文件：
- `package bootstrap`
- 从原文件复制对应的 import
- 复制对应函数体

- [ ] **Step 3: 验证编译**

```bash
go build ./bootstrap/...
```

- [ ] **Step 4: 全量测试**

bootstrap 被广泛引用，需要跑全量测试：

```bash
go test -race ./bootstrap/...
go test ./... 2>&1 | tail -20  # 快速检查无破坏
```

- [ ] **Step 5: 提交**

```bash
git add bootstrap/
git commit -m "refactor(bootstrap): 拆分 setup.go 按初始化领域分离函数"
```

---

### Task 3: 拆分 `cmd/mady/tui_session.go`（1194 行）

**目标:** 类型+访问器 / Agent 管理 / Slash handlers / 存储初始化 分离。

**文件:**
- 修改: `cmd/mady/tui_session.go`（保留 tuiSession struct + 核心访问器）
- 创建: `cmd/mady/tui_session_agent.go`（Agent 生命周期管理方法）
- 创建: `cmd/mady/tui_session_handlers.go`（所有 slash command handlers）
- 创建: `cmd/mady/tui_session_storage.go`（数据库/存储初始化方法）
- 创建: `cmd/mady/tui_session_detect.go`（detectAgentID, detectProjectID, 项目检测）

- [ ] **Step 1: 读取并标记方法分组**

使用 codegraph_node 查看 `tuiSession` 结构体的所有方法，按功能分组。

- [ ] **Step 2: 逐组提取到新文件**

- Agent 管理：`rebuildAgent`, `initAgent`, `shutdownAgent` 等 → `tui_session_agent.go`
- Handlers：`handleSubmit`, `handleThinkingCommand`, `handleThemeCommand`, `handleClearCommand` 等 → `tui_session_handlers.go`
- 存储：`dbPath`, `openApprovalStore`, `openPendingStore`, `openEventStore`, `startEventLogger` 等 → `tui_session_storage.go`
- 检测：`detectAgentID`, `detectProjectID` 等 → `tui_session_detect.go`

- [ ] **Step 3: 验证**

```bash
go build ./cmd/mady/...
go test -race ./cmd/mady/...
go vet ./cmd/mady/...
```

- [ ] **Step 4: 提交**

```bash
git add cmd/mady/tui_session*.go
git commit -m "refactor(cmd): 拆分 tui_session.go 按职责分离 handler/agent/storage"
```

---

### Task 4: 拆分 `domains/inventiveness/nodes.go`（1041 行）

**目标:** 创造性判断四步法各节点独立文件 + 辅助函数独立。

**文件:**
- 修改: `domains/inventiveness/nodes.go`（保留 BuildInventivenessGraph + 注册函数）
- 创建: `domains/inventiveness/node_step1.go`
- 创建: `domains/inventiveness/node_step2.go`
- 创建: `domains/inventiveness/node_step3.go`
- 创建: `domains/inventiveness/node_step4.go`
- 创建: `domains/inventiveness/node_conclusion.go`
- 创建: `domains/inventiveness/node_combined.go`
- 创建: `domains/inventiveness/node_experimental.go`
- 创建: `domains/inventiveness/guidance.go`（personSkilledDefinition, inventionTypeGuidance, techDomainGuidance 等辅助函数）

- [ ] **Step 1: 读取结构**

使用 codegraph_explore 查看 `domains/inventiveness/nodes.go` 中所有节点函数。

- [ ] **Step 2: 按节点逐个提取**

每个步骤节点函数（step1ClosestPriorArtNode, step2DistinguishingFeaturesNode 等）→ 独立文件。

- [ ] **Step 3: 验证**

```bash
go build ./domains/inventiveness/...
go test -race ./domains/inventiveness/...
go vet ./domains/inventiveness/...
```

- [ ] **Step 4: 提交**

```bash
git add domains/inventiveness/
git commit -m "refactor(inventiveness): 拆分 nodes.go 创造性四步法节点各一文件"
```

---

### Task 5: 拆分 `a2a/ws.go`（883 行）

**目标:** WebSocket 服务端和客户端代码分离。

**文件:**
- 修改: `a2a/ws.go`（保留共享类型和常量）
- 创建: `a2a/ws_server.go`（handleWebSocket, wsReadLoop, wsPingLoop, 所有 handleWS* 函数）
- 创建: `a2a/ws_client.go`（WSClient, WSConnection, NewWSClient, Connect, readLoop, SendRequest, Recv）

- [ ] **Step 1: 读取结构**

```bash
# 识别服务端和客户端代码的分界
```

- [ ] **Step 2: 分离服务端和客户端**

- `a2a/ws_server.go`：handleWebSocket, wsReadLoop, wsPingLoop, handleWSSendTask, handleWSGetTask, handleWSCancelTask, handleWSQueryTasks, handleWSSubscribe, handleWSResubscribe, wsForwardEvents, subscribeToTask, unsubscribeFromTask, checkWSOrigin, checkWSAuth
- `a2a/ws_client.go`：WSClient, WSConnection, NewWSClient, Connect, readLoop, SendRequest, Recv, tryReconnect

- [ ] **Step 3: 验证**

```bash
go build ./a2a/...
go test -race ./a2a/...
go vet ./a2a/...
```

- [ ] **Step 4: 提交**

```bash
git add a2a/ws*.go
git commit -m "refactor(a2a): 拆分 ws.go 为 ws_server.go + ws_client.go"
```

---

### Task 6: 拆分 `tui/component/markdown.go`（881 行）

**目标:** 组件包装 / 渲染引擎 / 主题 / 缓存分离。

**文件:**
- 修改: `tui/component/markdown.go`（保留 Markdown 组件 struct + Render 方法）
- 创建: `tui/component/markdown_renderer.go`（内联渲染引擎）
- 创建: `tui/component/markdown_theme.go`（主题相关代码）
- 创建: `tui/component/markdown_cache.go`（块缓存管理）

- [ ] **Step 1: 分析 markdown 组件结构**

使用 Read 查看 `tui/component/markdown.go`，识别渲染器内部结构、主题逻辑、缓存逻辑的边界。

- [ ] **Step 2: 逐块提取**

先提取渲染引擎（最大的内部块），验证编译 → 提取主题 → 提取缓存。

- [ ] **Step 3: 验证**

```bash
go build ./tui/component/...
go test -race ./tui/component/...
go vet ./tui/component/...
```

- [ ] **Step 4: 提交**

```bash
git add tui/component/markdown*.go
git commit -m "refactor(tui): 拆分 markdown.go 渲染引擎/主题/缓存分离"
```

---

### Task 7: 拆分 `domains/evidence/engine.go`（815 行）

**目标:** 三性判断（关联性/合法性/真实性）/ 类型识别 / 公开使用评估各一文件。

**文件:**
- 修改: `domains/evidence/engine.go`（保留 DefaultEngine + Judge + BatchJudge）
- 创建: `domains/evidence/triple_attributes.go`（evaluateTripleAttributes + 相关）
- 创建: `domains/evidence/type_specific.go`（evaluateTypeSpecific + inferEvidenceType）
- 创建: `domains/evidence/public_use.go`（公开使用评估相关函数）

- [ ] **Step 1-4: 同上模式**（分析 → 提取 → 验证 → 提交）

- [ ] **Step 5: 提交**

```bash
git add domains/evidence/
git commit -m "refactor(evidence): 拆分 engine.go 三性/类型/公开使用分离"
```

---

### Task 8: 拆分 `knowledge/sqlite/store.go`（802 行）

**目标:** 按功能域分离：向量搜索 / FTS 搜索 / 法律搜索 / 图加载。

**文件:**
- 修改: `knowledge/sqlite/store.go`（保留 SQLiteStore struct + CRUD 基础方法 + 类型定义）
- 创建: `knowledge/sqlite/vector.go`（PreloadVectors, VectorSearch 等）
- 创建: `knowledge/sqlite/fts.go`（FTSSearch, GetChunksByDocID）
- 创建: `knowledge/sqlite/laws.go`（SearchLaws, searchLawsFTS, searchLawsLike）
- 创建: `knowledge/sqlite/graph.go`（LoadGraph）

- [ ] **Step 1-4: 同上模式**

- [ ] **Step 5: 提交**

```bash
git add knowledge/sqlite/
git commit -m "refactor(knowledge): 拆分 store.go 向量/FTS/法律/图加载分离"
```

---

### Task 9: 拆分 `tui/terminal/detect.go`（795 行）

**目标:** 各终端能力检测函数按类别分组。

**文件:**
- 修改: `tui/terminal/detect.go`（保留核心类型 + DetectTerminalContext + CurrentTerminalContext）
- 创建: `tui/terminal/detect_color.go`（compute256Color, computeTrueColor）
- 创建: `tui/terminal/detect_input.go`（computeKittyKeyboard, computeShiftEnterCtrlDot）
- 创建: `tui/terminal/detect_hyperlink.go`（computeOSC8Hyperlinks）
- 创建: `tui/terminal/detect_brand.go`（detectTerminalBrandFromEnv 等品牌检测）
- 创建: `tui/terminal/detect_env.go`（buildTerminalContextFromEnv 等环境检测）

- [ ] **Step 1-4: 同上模式**

- [ ] **Step 5: 提交**

```bash
git add tui/terminal/detect*.go
git commit -m "refactor(tui): 拆分 detect.go 终端能力检测按类别分组"
```

---

### Task 10: 拆分 `domains/enablement/nodes.go`（779 行）

**目标:** 充分公开分析各步骤节点独立。

**文件:**
- 修改: `domains/enablement/nodes.go`（保留 BuildEnablementGraph + 注册函数）
- 创建: `domains/enablement/node_completeness.go`
- 创建: `domains/enablement/node_clarity.go`
- 创建: `domains/enablement/node_enablement.go`
- 创建: `domains/enablement/node_conclusion.go`
- 创建: `domains/enablement/helpers.go`（extractInput, buildInput, buildResult, parse* 函数）

- [ ] **Step 1-4: 同 Task 4 模式**（Pregel 图节点拆分，结构一致）

- [ ] **Step 5: 提交**

```bash
git add domains/enablement/
git commit -m "refactor(enablement): 拆分 nodes.go 充分公开各步骤节点独立"
```

---

### Task 11: 拆分 `tools/browser_session.go`（768 行）

**目标:** 按浏览器后端分离 session 创建策略。

**文件:**
- 修改: `tools/browser_session.go`（保留 BrowserManager struct + 公共调度逻辑 + BrowserSession struct）
- 创建: `tools/browser_session_cdp.go`（createCDPSession）
- 创建: `tools/browser_session_camofox.go`（createCamofoxSession）
- 创建: `tools/browser_session_lightpanda.go`（createLightpandaSession）
- 创建: `tools/browser_session_cloud.go`（createCloudSession + createAgentBrowserSession）
- 创建: `tools/browser_session_local.go`（createLocalSession + URL/IP 验证）

- [ ] **Step 1-4: 同上模式**

- [ ] **Step 5: 提交**

```bash
git add tools/browser_session*.go
git commit -m "refactor(tools): 拆分 browser_session.go 按浏览器后端分离"
```

---

### Task 12: 拆分 `cmd/mady/slash_registry.go`（765 行）

**目标:** Registry 框架 / 各命令组定义分离。

**文件:**
- 修改: `cmd/mady/slash_registry.go`（保留 Registry struct + SlashCommand 类型 + Lookup/Suggest 方法）
- 创建: `cmd/mady/slash_commands_chat.go`（chat 相关命令注册）
- 创建: `cmd/mady/slash_commands_settings.go`（设置相关命令注册）
- 创建: `cmd/mady/slash_commands_session.go`（会话相关命令注册）
- 创建: `cmd/mady/slash_commands_project.go`（项目相关命令注册）
- 创建: `cmd/mady/slash_commands_patent.go`（专利相关命令注册）

- [ ] **Step 1: 分析 buildSlashRegistry 中的命令分组**

读取 `cmd/mady/slash_registry.go` 中 `buildSlashRegistry()` 函数的命令列表，按功能分组。

- [ ] **Step 2-4: 逐组提取 → 验证 → 提交**

- [ ] **Step 5: 提交**

```bash
git add cmd/mady/slash_*.go
git commit -m "refactor(cmd): 拆分 slash_registry.go 命令按功能组分离"
```

---

### Task 13: 拆分 `domains/case_index.go`（756 行）

**目标:** 按实体类型分组 CRUD 操作。

**文件:**
- 修改: `domains/case_index.go`（保留 CaseIndex struct + initSchema + syncFTS）
- 创建: `domains/case_index_crud.go`（CreateCase, GetCase, UpdateCase, DeleteCase, ListAll, FindByDraftingIdentity, FindByFilingNumber）
- 创建: `domains/case_index_path.go`（AddPath, GetPaths, FindByPath）
- 创建: `domains/case_index_document.go`（RecordDocument, GetDocuments, GetDocument）
- 创建: `domains/case_index_event.go`（AddEvent, GetEvents）
- 创建: `domains/case_index_search.go`（SearchCases, searchByFTS）
- 创建: `domains/case_index_lifecycle.go`（UpgradeToFiled, UpgradeToPublished）
- 创建: `domains/case_index_types.go`（CaseRecord, CasePath, CaseDocument, CaseEvent, CaseSearchQuery）

- [ ] **Step 1-4: 按实体逐组提取 → 验证 → 提交**

- [ ] **Step 5: 提交**

```bash
git add domains/case_index*.go
git commit -m "refactor(domains): 拆分 case_index.go 按实体分组 CRUD"
```

---

### Task 14: 拆分 `tui/chat/chat_app_layout.go`（753 行）

**目标:** 按布局区域分离 Header / Footer / Sidebar / Main。

**文件:**
- 修改: `tui/chat/chat_app_layout.go`（保留 Layout 主调度方法）
- 创建: `tui/chat/layout_header.go`
- 创建: `tui/chat/layout_footer.go`
- 创建: `tui/chat/layout_sidebar.go`
- 创建: `tui/chat/layout_main.go`

- [ ] **Step 1-4: 同上模式**

- [ ] **Step 5: 提交**

```bash
git add tui/chat/layout_*.go tui/chat/chat_app_layout.go
git commit -m "refactor(tui): 拆分 chat_app_layout.go 按布局区域分离"
```

---

### Task 15: 拆分 `domains/case_extension.go`（720 行）

**目标:** 各 tool handler 独立文件。

**文件:**
- 修改: `domains/case_extension.go`（保留 Extension struct + Init + 公共方法）
- 创建: `domains/case_extension_list.go`（handleListCases）
- 创建: `domains/case_extension_search.go`（handleSearchCases）
- 创建: `domains/case_extension_sync.go`（handleSyncCase + incrementalScan + applyDocUpdates）
- 创建: `domains/case_extension_focus.go`（handleFocusCase）
- 创建: `domains/case_extension_register.go`（handleRegisterCase + applyIdentityUpgrade）
- 创建: `domains/case_extension_upgrade.go`（handleUpgradeCase）

- [ ] **Step 1-4: 每个 handler → 独立文件 → 验证 → 提交**

- [ ] **Step 5: 提交**

```bash
git add domains/case_extension*.go
git commit -m "refactor(domains): 拆分 case_extension.go 各 tool handler 独立文件"
```

---

### Phase 1 收尾验证

- [ ] **全量编译**

```bash
go build ./...
```

- [ ] **全量测试**

```bash
go test ./... 2>&1 | tail -30
```

预期：所有测试通过，无 regression。

---

## Phase 2：模块深耕（41 文件）

Phase 2 文件需要深入分析模块架构后才能确定具体拆分方式。以下为模块级执行顺序和每模块完成后验证命令。

### Task 16-27: domains/ 模块（12 文件）

执行顺序：按行数降序

| 序号 | 文件 | 预估拆分方向 |
|------|------|-------------|
| 16 | `domains/workflows/patent/reexamination.go` (809) | 图节点分离 + 状态类型独立 |
| 17 | `domains/workflows/patent/invalidation.go` (796) | 同复审模式 |
| 18 | `domains/workflows/patent/oa_response.go` (746) | OA 解析/答复生成/证据引用分离 |
| 19 | `domains/claimdrafting/builder.go` (667) | 权利要求构建器按撰写阶段分离 |
| 20 | `domains/infringement/nodes.go` (644) | Pregel 节点独立（同 inventiveness 模式） |
| 21 | `domains/workflows/patent/reasoning_patterns.go` (634) | 推理模式按类别分组 |
| 22 | `domains/workflows/patent/analysis.go` (539) | 分析工作流步骤分离 |
| 23 | `domains/workflows/legal/comparison.go` (543) | 法律比较维度分离 |
| 24 | `domains/patent.go` (531) 🔴 | 敏感路径，需保守处理 |
| 25 | `domains/rules/slop_engine.go` (530) | 反套话规则分组 |
| 26 | `domains/novelty/prompts.go` (520) | 提示词按新颖性维度分组 |
| 27 | `domains/evidence/date.go` (500) | 日期判断规则分类 |

每完成一个文件：
```bash
go build ./domains/... && go test -race ./domains/<subpkg>/... && go vet ./domains/<subpkg>/...
```

### Task 28-38: tui/ 模块（11 文件）

| 序号 | 文件 | 预估拆分方向 |
|------|------|-------------|
| 28 | `tui/chat/chat_app.go` (1184) | ChatApp 方法按功能组分离 |
| 29 | `tui/component/input.go` (675) | 输入模式/渲染分离 |
| 30 | `tui/chat/chat_history.go` (680) | 历史数据管理/查询分离 |
| 31 | `tui/chat/chat_history_render.go` (585) | 渲染模式分离 |
| 32 | `tui/component/editor_edit.go` (610) | 编辑操作分组 |
| 33 | `tui/component/session_selector.go` (558) | 待分析 |
| 34 | `tui/component/review_gate.go` (541) | 待分析 |
| 35 | `tui/overlay.go` (672) | 按 Overlay 类型分离 |
| 36 | `tui/terminal/stdin_buffer.go` (618) | 待分析 |
| 37 | `tui/terminal/keys.go` (594) | 可能豁免（常量文件） |
| 38 | `tui/terminal/terminal.go` (512) | 待分析 |

### Task 39-43: tools/ 模块（5 文件）

| 序号 | 文件 | 预估拆分方向 |
|------|------|-------------|
| 39 | `tools/browser_supervisor.go` (662) | 监督策略/任务调度分离 |
| 40 | `tools/edit.go` (544) | 编辑操作类型分离 |
| 41 | `tools/process.go` (520) | 进程类型分离 |
| 42 | `tools/desktop/computer_use.go` (512) | 平台适配分离 |
| 43 | `tools/vision.go` (508) | 图片处理/模型调用分离 |

### Task 44-46: agentcore/ 模块（3 文件）🔴

| 序号 | 文件 | 预估拆分方向 |
|------|------|-------------|
| 44 | `agentcore/compaction.go` (644) 🔴 | 压缩策略分离（需保守） |
| 45 | `agentcore/agent.go` (536) 🔴 | 生命周期阶段分离（需保守） |
| 46 | `agentcore/event_types.go` (531) | **建议豁免**（纯类型定义） |

### Task 47-48: knowledge/ 模块（2 文件）

| 序号 | 文件 | 预估拆分方向 |
|------|------|-------------|
| 47 | `knowledge/extension.go` (698) | Extension 方法按功能组分离 |
| 48 | `knowledge/fileindex/store.go` (688) | 索引操作/查询分离 |

### Task 49-50: a2a/ 模块（2 文件）

| 序号 | 文件 | 预估拆分方向 |
|------|------|-------------|
| 49 | `a2a/client.go` (722) | 客户端方法分组 |
| 50 | `a2a/server_jsonrpc.go` (544) | JSON-RPC handler 分组 |

### Task 51-56: 其他模块（6 文件）

| 序号 | 文件 | 预估拆分方向 |
|------|------|-------------|
| 51 | `desktop/app.go` (1268) | Wails bindings 按功能组分离 |
| 52 | `acp/server.go` (1013) | 协议层/传输层分离 |
| 53 | `mcp/discovery.go` (796) | 发现策略/扩展创建分离 |
| 54 | `acp/session.go` (594) | 会话方法分组 |
| 55 | `session/session_store.go` (585) | 存储操作分组 |
| 56 | `session/agent_store.go` (529) | AgentStore 方法分组 |

Phase 2 每模块完成后：
```bash
go test -race ./<module>/...
```

Phase 2 全部完成后：
```bash
go test ./...
```

---

## Phase 3：收尾（14 文件）

Phase 3 处理分散模块和豁免文件。大部分是需要独立分析的边界文件。

| 序号 | 文件 | 行数 | 处理方式 |
|------|------|------|----------|
| 57 | `example/cli-chat/main.go` | 919 | **豁免**（入口文件） |
| 58 | `disclosure/novelty.go` | 728 | 待分析拆分 |
| 59 | `agui/converter.go` | 720 | 事件类型分离 |
| 60 | `provider/chatcompat/chat.go` | 707 | Chat API 分组 |
| 61 | `memory/sqlite_store.go` | 663 | 操作分组 |
| 62 | `memory/store.go` | 645 | 接口 + 实现分离 |
| 63 | `doomloop/doomloop.go` | 625 | 🔴 **建议豁免**（结构清晰，敏感路径） |
| 64 | `evaluate/benchmark/patent_exam_real_a22.go` | 619 | 评估用例分组 |
| 65 | `server/disclosure.go` | 562 | 待分析 |
| 66 | `evaluate/metrics.go` | 557 | 指标计算分组 |
| 67 | `agentcore/tool_gen.go` | 512 | **豁免**（代码生成） |
| 68 | `session/session.go` | 511 | 待分析 |
| 69 | `provider/chatcompat/responses.go` | 501 | 待分析 |
| 70 | `tui/layout/flex.go` | 539 | 布局原语分组 |

---

## 全局收尾验证

全部 70 文件处理完成后：

- [ ] **最终全量验证**

```bash
go build ./...
go test -race ./...
go vet ./...
```

- [ ] **敏感路径额外验证**

```bash
bash scripts/check-sensitive-paths.sh
```

- [ ] **统计拆分结果**

```bash
# 统计所有新创建的文件数
git diff --stat HEAD | grep -c "create mode"
```
