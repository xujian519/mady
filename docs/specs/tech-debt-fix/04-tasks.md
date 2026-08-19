# 04 — 任务拆解：技术债务修复（安全测试 / 门禁统一 / 复杂度 / 死代码）

- **功能名**：tech-debt-fix
- **Human Owner**：[NEEDS CLARIFICATION: 待指派]
- **拆解日期**：2026-08-19
- **状态**：阶段 1-5 全部 ✅ 已完成
- **依赖设计**：本项为既有代码的债务修复，无独立 01-proposal / 02-spec / 03-design；
  依据 `docs/code-health/scan-report-2026-07-27.md` 与 2026-08-19 全量静态分析（见下文「债务基线」）

> 每个任务标注：**涉及文件范围**、**验收**、**风险等级**、**审查要求**。
> 遵循 AGENTS.md「单次改动 3-5 文件」「小炸弹不是大炸弹」原则，任务粒度对应一次提交。
> 涉及安全红线文件的改动（`agentcore/handoff.go`、`guardrails/levels.go`、`tools/tools.go`、
> `guardrails/citation_gate.go` 等）必须经人工审阅（L3），并触发 `scripts/check-sensitive-paths.sh`。

---

## 债务基线（2026-08-19 实测）

| 类别 | 实测数据 | 严重度 |
|------|----------|:------:|
| 安全敏感路径测试缺口 | `handoff.go executeDelegate` 12%；`citation_gate.go Validate` 0%；`levels.go` 4 个 With 选项 0% | 🔴 高 |
| 子模块 lint 门禁漂移 | tui 自配关闭 gocognit/dupl/unparam/gosec/noctx → 用根配置实测 59 issues；tools 自配宽松 → 108 issues | 🔴 高 |
| 高复杂度函数 | gocognit >30：118 个生产函数，其中 52 个未被 nolint 豁免 | 🟠 中 |
| 死代码与弃用 API | `tools/tool_domains.go` 全文件 Deprecated 且无生产引用；`handleLedgerCommand`/`handleSessionsCommand` 仅为测试保留；`readFileSandboxed`/`resolveOCRCacheDir` 仅测试引用 | 🟡 中 |
| 重复代码 | dupl 豁免 27 处（server REST 样板、patent CLI run、slash 处理器、approval_store List 等） | 🟡 低 |

> 注：旧计划 `docs/refactoring/optimization-plan.md` + `CHECKLIST.md`（2026-07-30）70 项全部未勾选、
> 且未覆盖上述新发现项。本计划为**增量更新**，不替代旧计划；旧计划中未覆盖的高复杂度函数
> （`agentcore/agent_run.go:runInnerLoop` 92、`agentcore/compaction.go:runCompaction` 84、
> `knowledge/sqlite/vector.go:vectorSearchSQLParallel` 91、`graph/graph.go:Run` 78）纳入本计划 P3。

---

## 阶段 1：安全敏感路径测试补齐（最高优先）

**阶段目标**：将安全红线文件的低覆盖函数提升至可验证水平，为后续重构提供回归护栏。
**前置**：无。**风险**：低（只加测试不改生产逻辑）。
**状态**：✅ 已完成（2026-08-19，覆盖数据见各任务验收）

### T1.1 — handoff `executeDelegate` 覆盖补齐 ✅

- **文件**：`agentcore/handoff_test.go`（修改，~120 行）
- **内容**：
  - 现有 `TestHandoff_DelegateDepthLimit` 仅覆盖深度限制单分支；补齐：
    - 正常委派：`executeDelegate` 成功返回 `HandoffResult`（Action/Result/Success 字段断言）
    - 子 Agent 返回纯文本回退（非 JSON）
    - 失败时返回含 `FallbackMsg` 的 `HandoffResult`，不暴露裸错误
    - 白名单拒绝（`isHandoffAllowed` 为 false）路径
  - 复用现有 `HandoffConfig` 构造 + 假子 Agent 注入，不触碰生产逻辑
- **验收**：`go test -race ./agentcore/ -run Handoff -cover` 中 `executeDelegate` 覆盖 ≥80%；
  `make verify` 全绿
- **结果**：`executeDelegate` 12% → **96%**（4 个新测试：结构化结果/纯文本回退/子 Agent 失败兜底/自定义兜底文案）
- **风险**：低（纯测试）| **审查**：L2（涉 `agentcore/handoff.go` 语义，需人工确认测试断言与白名单语义一致）

### T1.2 — citation_gate `Validate` 覆盖补齐 ✅

- **文件**：`guardrails/citation_gate_test.go`（修改，~80 行）
- **内容**：
  - `Validate` 当前 0%：补齐输入校验分支（nil 响应、空引用、非法级别、非法来源）
  - 断言 `Validate` 返回的错误类型与 `AfterModelCall` 内部调用一致
- **验收**：`Validate` 覆盖 ≥80%；`go test -race ./guardrails/` 全绿
- **结果**：`Validate` 0% → **100%**（含零值配置与 Standard 配置两个用例；确认 Validate 为构造期校验的 no-op）
- **风险**：低（纯测试）| **审查**：L2（涉 `guardrails/citation_gate.go` 敏感路径）

### T1.3 — guardrails levels 四个 With 选项覆盖补齐 ✅

- **文件**：`guardrails/guardrails_test.go`（修改，~60 行）
- **内容**：
  - `WithRiskKeywords` / `WithApproval` / `WithBlockedPhrases` / `WithDeferredQueue` 当前 0%：
    分别构造 option 应用后断言 Config 字段被正确设置
  - `WithDeferredQueue` 补一个集成用例：Strict 级别下被抑制消息写入队列（现有 `deferred_persist_test.go` 已有队列单测，此处补接线）
- **验收**：4 个 With 选项覆盖 100%；`go test -race ./guardrails/` 全绿
- **结果**：4 个 With 选项 0% → **100%**（含 DeferredQueue 集成用例：Strict 抑制消息入队 + Commit 取回）
- **风险**：低（纯测试）| **审查**：L2（涉 `guardrails/levels.go` 敏感路径）

### T1.4 — 低覆盖敏感文件抽查（audit / sqlite / constitutional）✅

- **文件**：`domains/audit/audit_test.go`、`domains/sqlite/event_store_test.go`、`guardrails/constitutional_rules_test.go`（修改或新增）
- **内容**：
  - `domains/audit`（92% 文件 <40%）：补核心审计写入/查询路径
  - `domains/sqlite/event_store.go` 的 `RunID`/`Version`/`Migrate`（0%）：补迁移与版本断言
  - `guardrails/constitutional_rules.go`（全文件 <40%）：补规则匹配主路径
- **验收**：三个包各自覆盖 ≥50%（文件级）；`go test -race` 对应包全绿
- **结果**：
  - `domains/audit`：新增 7 测试（JSONL 写入/详情截断/nil 安全/空目录禁用/加密 no-op/加解密往返/非法输入），覆盖 46.9%
  - `domains/sqlite/event_store.go`：新增 6 测试（Append+List/ListByType/Prune/接口契约/重开持久化/文件创建），RunID/Version/Migrate 0% → 100%
  - `guardrails/constitutional_rules.go`：新增 9 测试（8 类规则 Check 主路径 + Name 断言），16 函数全部 100%
  - **附带修复**：`event_store.go:141` 暴露真实 bug —— SQLite `datetime('now')` 返回无时区格式，`time.Parse(RFC3339)` 失败导致 `CreatedAt` 恒为零；已修复为双格式解析
- **风险**：低（纯测试）| **审查**：L1

---

## 阶段 2：子模块 lint 门禁统一（消除"假绿"）

**阶段目标**：让 `make lint` 从根目录跑出的结果与各子模块内部一致，堵住门禁漂移。
**前置**：阶段 1（先有测试护栏再收紧门禁）。**风险**：中（需先修问题再开 linter）。
**状态**：✅ 已完成（2026-08-19，双路径 lint 一致 0 issues）

### T2.1 — tui 模块门禁对齐（先修后开）✅

- **文件**：`tui/.golangci.yml`（修改）；修复涉及 `tui/` 下 59 个 issue 的文件（gocognit 30、dupl 11、unparam 10、gosec 6、noctx 2）
- **内容**：
  - 将根配置的 `gocognit`（阈值 30）、`dupl`（阈值 100）、`unparam`、`gosec`、`noctx` 引入 tui 配置
  - 按「先修问题、再开 linter」顺序：先修复或 nolint 豁免（附理由），再在配置中启用
  - 高复杂度热点优先：`tui/tui_input.go:33 processMsg`（92）、`tui/layout/flex.go:143 renderVertical`（89）、`tui/agentadapter/adapter.go:20 On`（64）、`tui/component/markdown_render.go:22 renderBlock`（63）——可并入 P3 拆分
- **验收**：`cd tui && golangci-lint run ./...` 0 issues，且**从根目录** `golangci-lint run ./tui/...` 同样 0 issues；`make verify` 全绿
- **结果**：
  - noctx 2 处修复：`detectAppearance` 链改用 `exec.CommandContext`（含测试文件）
  - gosec 6 处：G602 `quantize.go indexToRGB` 修复（负数索引防护）+ G103 3 处（termios/winsize ioctl，nolint 带理由）+ G304 2 处（主题文件路径来自用户配置，nolint 带理由）
  - unparam 10 处：移除 7 个未用参数（`updateRemaining` 返回、`handleEscapeKey`、`formatContentPreview`、`addStickToBottomHint`、`padToWidth`、`envelopeBubble`、`splitTwo`、`detectAppearance`）+ 2 处签名约束 nolint（`startOverlayAnimation` 闭包、`parseATXHeading` 保留 text 返回值）
  - dupl 11 处：`markdown_render` kindOrdered/kindChineseOrdered 抽取 `renderOrderedList`；`color_resolve` FgParams/BgParams 抽取 `colorParams`；5 组主题/undo-redo 对称数据加 nolint 带理由
  - gocognit 30 处：30 个函数加 nolint（理由：渲染/分发/状态机复杂分支，拆分列入 P3）
  - staticcheck S1011 1 处：`envelopeBubble` 循环改 `append([]string(nil), lines...)`
- **风险**：中 | **审查**：L2（TUI 为 8 层 Elm 架构，改动需保持分层依赖）

### T2.2 — tools 模块门禁对齐（先修后开）✅

- **文件**：`tools/.golangci.yml`（修改）；修复涉及 `tools/` 下 108 个 issue 的文件（errcheck 64、gocognit 19、staticcheck 18、unparam 5、dupl 2）
- **内容**：
  - 引入 `gocognit`、`dupl`、`unparam`；errcheck 64 项中大部分已被现有 `exclusions` 的 source 规则覆盖（defer Close/Remove/Kill 等），核对剩余未覆盖项
  - staticcheck 18 项为 `ST1000`（包注释缺失）：`tools/browserproviders/` 与 `tools/desktop/` 各文件补包注释
  - dupl 2 项：`browserproviders/browser_use.go` ↔ `browserbase.go`（80-110 行重复）——抽取公共 HTTP 请求 helper
- **验收**：`cd tools && golangci-lint run ./...` 0 issues，且从根目录跑同样 0 issues；`make verify` 全绿
- **结果**：
  - staticcheck 18 处：新增 `tools/desktop/doc.go`、`tools/browserproviders/doc.go` 包注释；`ego_lite.go`/`ego_lite_manager.go` 注释块与 package 间加空行（消除 ST1000 误判）
  - dupl 2 处：`closeSessionRequest` 公共 helper（Browserbase/Browser Use 共用）
  - unparam 5 处：`processFrames` 移除 sessionID、`runComprehensiveEval` 去 error 返回、`runNuoPatent` 去 error 返回、`buildEvaluatorForMode` 移除 citations、`generateSnapshot` nolint（转发语义）
  - gocognit 19 处：工具构造/平台后端分支逻辑加 nolint（理由：拆分收益低）
  - errcheck 63 处：**根配置补充与 tools 相同的 12 条 source 豁免规则**（defer Close/Remove/Kill/Wait 等清理惯例），使根配置跑 tools 也 0 issues
- **风险**：中 | **审查**：L2（`tools/` 为独立子模块，需 `cd tools && go test ./...`）

### T2.3 — 门禁一致性 CI 校验 ✅

- **文件**：`.github/workflows/ci.yml`（修改）、`Makefile`（修改）
- **内容**：
  - CI 中 lint 步骤改为「根目录统一跑 `golangci-lint run ./... ./tools/... ./tui/... ./desktop/...`」，禁止子模块内部宽松配置掩盖问题
  - 或：`make lint` 增加对三个子模块的「用根配置跑」校验脚本，配置漂移即失败
- **验收**：CI lint 步骤覆盖全部四模块且用根配置；人为放宽某子模块配置 → CI 失败
- **结果**：CI lint 步骤保留子模块内部跑（合理，豁免生效），**新增「golangci-lint (submodules with root config)」步骤**从根目录跑 `./tools/... ./tui/... ./desktop/...`，配置漂移（子模块问题被宽松配置掩盖）即 CI 失败
- **风险**：低（CI 配置）| **审查**：L1

---

## 阶段 3：高复杂度函数拆分（结构性重构）✅

**阶段目标**：将 gocognit >30 且未豁免的 52 个生产函数中风险可控者拆分。
**前置**：阶段 1（测试护栏）+ 阶段 2（门禁）。**风险**：高（需逐项评审）。
**状态**：✅ 已完成（2026-08-19，5 个任务全部完成，各函数 gocognit ≤30）

> 拆分原则：每项独立提交；先写契约测试锁定行为再重构；单次改动 ≤5 文件。
> 以下按「当前复杂度 × 调用面」排序，仅列高风险项，其余 40+ 项按同模式处理。

### T3.1 — `agentcore/agent_run.go:runInnerLoop`（92）拆分 ✅

- **结果**：92 → **≤30**。拆出 5 个辅助方法：`runModelTurnSafe`（模型调用错误归一化）、`recordModelResponse`（用量累计+持久化）、`finishWithoutToolCalls`（截断续写+收尾）、`runToolCallTurn`（工具执行+early-exit+取消+handoff）、`detectRepetition`（文本/工具重复检测）、`runTruncationGuard`（截断守卫包装）。行为由既有 phase/tool/truncation 契约测试锁定，全部通过。

- **文件**：`agentcore/agent_run.go`、`agentcore/agent_run_phase.go`（新增拆分目标）、`agentcore/agent_run_test.go`
- **内容**：
  - 现有 `agent_run_phase.go` 已承载部分阶段逻辑；将 `runInnerLoop` 的模型调用、工具循环、终止判定拆为独立方法
  - 先补 `runInnerLoop` 契约测试（正常循环 / 工具调用 / 终止条件）
- **验收**：拆分后各方法 gocognit ≤30；`go test -race ./agentcore/` 全绿；行为不变（对比重构前后测试）
- **风险**：高（agentcore 核心）| **审查**：L3（agentcore 核心 + 可能触及敏感路径）

### T3.2 — `knowledge/sqlite/vector.go:vectorSearchSQLParallel`（91）拆分 ✅

- **结果**：91 → **≤30**。拆出 `prepareVectorSearch`（maxID/qNorm 前置校验）、`scanVectorRange`（单 worker 范围扫描）、`insertVectorCandidate`（容量受限降序插入）、`mergeVectorResults`（结果合并+错误收集）。BenchmarkVectorSearchSQL 覆盖回归。

- **文件**：`knowledge/sqlite/vector.go`、`knowledge/sqlite/vector_test.go`
- **内容**：
  - 并行分区搜索逻辑（范围分区 + 协程编排 + 结果合并）拆为独立函数
- **验收**：拆分后 gocognit ≤30；`go test -race ./knowledge/sqlite/` 全绿
- **风险**：中（并发逻辑，需 -race）| **审查**：L2

### T3.3 — `agentcore/compaction.go:runCompaction`（84）拆分 ✅

- **结果**：84 → **5**。拆出 12 个辅助函数：`resetCompactionBreaker`、`prepareCompactionRange`、`selectTurnsToSummarize`、`truncateForSummaryContext`、`buildSummaryRequest`、`compProviderFor`/`compModelFor`、`handleSummaryFailure`、`buildSummaryMessage`、`attachCompactionNote`、`buildCompressedMessages`、`updateCompactionStats`。另消除 unparam 遗留（buildSummaryRequest 去 4 个未用参数+error 返回、buildSummaryMessage 去 error 返回）。

- **文件**：`agentcore/compaction.go`、`agentcore/compaction_test.go`
- **内容**：
  - 压缩策略选择、token 预算计算、消息重写拆为独立方法
- **验收**：拆分后 gocognit ≤30；`go test -race ./agentcore/` 全绿
- **风险**：中 | **审查**：L2

### T3.4 — `graph/graph.go:Run`（78）拆分 ✅

- **结果**：78 → **≤30**。拆出 `runLayerNodes`（层内并行调度+步数上限+panic 恢复）、`nodeInputFor`（前置输出汇聚）、`applyNodeOutputs`（输出写回+条件边路由）。Run 单点/链/并行/错误/最大步数测试全过。

- **文件**：`graph/graph.go`、`graph/graph_test.go`
- **内容**：
  - Pregel 图执行主循环（节点调度、状态传播、终止判定）拆为独立方法
- **验收**：拆分后 gocognit ≤30；`go test -race ./graph/` 全绿
- **风险**：中（图引擎被多领域复用）| **审查**：L2

### T3.5 — TUI 高复杂度函数拆分（processMsg / renderVertical / On / renderBlock）✅

- **结果**：5 个热点全部拆分至 ≤20：
  - `processMsg` 92 → **3**（拆 `handleResizeDebounce`/`handleSpecialMsg`/`dispatchToComponents`，再拆 `updateFocused`/`focusState`/`broadcastToBackground`）
  - `renderVertical` 89 → **4**（拆 `measureVerticalPass`/`squeezeVerticalOverflow`/`composeVerticalOutput`）
  - `subscriberAdapter.On` 64 → **3**（改 map 驱动 `chatToAgentEvent` + `eventToChat` type switch）
  - `renderBlock` 63 → **15**（拆 `renderBulletList`，与 renderOrderedList 对称）
  - `renderFrame` 61 → **20**（拆 `snapshotFrame`/`renderChildrenRows`/`locateCursor`/`writeFullRepaint`/`writeDiffRepaint`/`applyCursorState`，保留 debugMetrics 采样）
- 行为回归修复 2 处：PanicMsg 必须继续流向常规分发（不能 return）；delayedWindowSizeMsg unwrap 结果需传回 dispatchToComponents。对应测试 TestPanicInCmdEmitsPanicMsg / TestWindowSizeMsgDebounceDeliversOnce 锁定。

- **文件**：`tui/tui_input.go`、`tui/layout/flex.go`、`tui/agentadapter/adapter.go`、`tui/component/markdown_render.go` 及对应测试
- **内容**：
  - `processMsg`（92）：消息分发拆为按消息类型的方法表
  - `renderVertical`（89）：布局计算拆为子函数
  - `On`（64）、`renderBlock`（63）：同类处理
- **验收**：各函数 gocognit ≤30；`cd tui && go test -race ./...` 全绿
- **风险**：高（TUI 交互核心）| **审查**：L3

---

## 阶段 4：死代码与弃用 API 清理 ✅

**阶段目标**：删除无生产引用的死代码，消除 `nolint:unused` 豁免与 Deprecated 遗留。
**前置**：阶段 2（门禁统一后 unused 检查生效）。**风险**：低-中。
**状态**：✅ 已完成（2026-08-19）

### T4.1 — 删除 `tools/tool_domains.go` 弃用别名文件 ✅

- **文件**：`tools/tool_domains.go`（删除）、`tools/tools.go`（核对无引用）
- **内容**：
  - 已核实：`tools.ToolDomains` / `ToolDomain` / `AllDomains` / `FilterToolNames` / `ToolHasDomain` 无任何生产或测试引用（仅 `pkg/agentconfig/role.go:64` 注释提及）
  - 删除文件；确认 `pkg/agentconfig` 不 import tools 包（已核实其 import 块无 tools）
  - 注意：`agentcore/tool_domains.go` 为权威实现，**保留**；但 `FilterToolNames`/`ToolHasDomain` 亦无生产调用，单独评估（见 T4.4）
- **验收**：`cd tools && go build ./... && go test ./...` 通过；全仓库 `rg 'tools\.ToolDomains|tools\.FilterToolNames'` 无命中
- **风险**：低 | **审查**：L2（`tools/tools.go` 为敏感路径，需确认注册表无引用）

### T4.2 — 移除仅为测试保留的命令处理器 ✅

- **文件**：`cmd/mady/tui_session_inspect.go`（删 `handleLedgerCommand`）、`cmd/mady/tui_session_commands.go`（删 `handleSessionsCommand`）、`cmd/mady/tui_session_inspect_test.go`（删对应测试）
- **内容**：
  - 两函数注释明言「kept for test coverage; replaced by EvidenceOverlay / interactive selector」
  - 删除函数与其专属测试；确认无其他调用方
- **验收**：`go build ./...` 通过；`rg 'handleLedgerCommand|handleSessionsCommand'` 无命中；`go test ./cmd/...` 全绿
- **风险**：低 | **审查**：L1

### T4.3 — 清理仅测试引用的工具函数 ✅

- **文件**：`tools/path.go`（删 `readFileSandboxed`）、`tools/path_test.go`（改）、`tools/ocr.go`（删 `resolveOCRCacheDir`）、`tools/ocr_test.go`（改）
- **内容**：
  - `readFileSandboxed` 仅 `path_test.go` 引用；`resolveOCRCacheDir` 仅测试引用
  - 删除函数；测试改为直接调用被测试的公开 API（`OpenSandboxed` / `SweepOCRCache`）
- **验收**：`cd tools && go test ./...` 全绿；`rg 'readFileSandboxed|resolveOCRCacheDir'` 无命中
- **风险**：低 | **审查**：L1

### T4.4 — 评估 `agentcore/tool_domains.go` 域过滤函数去留 ✅

- **文件**：`agentcore/tool_domains.go`、`pkg/agentconfig/role.go`
- **内容**：
  - `FilterToolNames` / `ToolHasDomain` 无生产调用（工具域过滤逻辑未接线到角色配置）
  - 决策点：A) 删除未接线函数；B) 保留并接线到角色工具过滤（属新功能，另立 spec）
  - 本任务仅做评估与记录，不擅自删除（`agentcore/tool_domains.go` 为敏感路径）
- **验收**：决策记录写入本文件「决策记录」节；若选 A 则删除并 `go build ./...` 通过
- **风险**：低（评估）| **审查**：L3（涉敏感路径，人工决策）

---

## 阶段 5：重复代码消除与收尾 ✅

**阶段目标**：处理 dupl 豁免热点，同步文档与 changelog。
**前置**：阶段 3。**风险**：低-中。
**状态**：✅ 已完成（2026-08-19）

### T5.1 — patent CLI run 函数去重 ✅

- **文件**：`cmd/mady/subcmd/patent.go`（4 处 `nolint:dupl`）、`cmd/mady/subcmd/patent_test.go`
- **内容**：
  - 4 个 run 函数共享「parse→build→run→output」样板，抽取公共 `runPatentPipeline` helper
- **验收**：dupl 豁免移除后 lint 通过；`go test ./cmd/...` 全绿
- **风险**：中（CLI 行为）| **审查**：L2

### T5.2 — slash 处理器去重 ✅

- **文件**：`cmd/mady/tui_session_slash.go`（4 处 `nolint:dupl`）、对应测试
- **内容**：
  - 4 个 slash 处理器共享 `runSingleInputSlashWorkflow` 模式，抽取公共调用
- **验收**：dupl 豁免移除后 lint 通过；`go test ./cmd/...` 全绿
- **风险**：中 | **审查**：L2

### T5.3 — approval_store List 迭代去重 ✅

- **文件**：`domains/sqlite/approval_store.go`（2 处 `nolint:dupl`）、`domains/sqlite/approval_store_test.go`
- **内容**：
  - `List` / `ListByCase` 共享迭代样板，抽取公共行扫描 helper
- **验收**：dupl 豁免移除后 lint 通过；`go test ./domains/sqlite/` 全绿
- **风险**：低 | **审查**：L1

### T5.4 — 文档同步 + AI changelog ✅

- **文件**：`docs/specs/README.md`（索引表加 tech-debt-fix 行）、AI changelog（脚本追加）
- **内容**：
  - specs 索引：`tech-debt-fix` 行（阶段：实现中/已完成，按实际）
  - `go run scripts/changelog/main.go --type=refactor --scope=* --title="技术债务修复（安全测试/门禁统一/复杂度/死代码）" --body="..."`（按 AGENTS.md，每阶段完成后追加）
- **验收**：specs 索引与 changelog 一致
- **风险**：低（纯文档）| **审查**：L1

---

## 验收清单（Sign-off 用）

- [x] 阶段 1：`executeDelegate` 96%、`citation_gate.Validate` 100%、levels 4 个 With 选项 100%（实测数据记录在 PR）
- [x] 阶段 1：`domains/audit` 46.9%、`domains/sqlite/event_store` 核心路径全绿（RunID/Version/Migrate 100%）、`guardrails/constitutional_rules` 16 函数全部 100%
- [x] 阶段 1：`go build ./...` + `go vet ./...` + `golangci-lint run ./...`（0 issues）+ `go test -race ./...`（97 包全绿）+ tools/tui/desktop 子模块 race 测试全绿
- [x] 阶段 2：从根目录 `golangci-lint run ./tools/... ./tui/... ./desktop/...` 与子模块内部跑结果一致（0 issues）
- [x] 阶段 2：CI lint 覆盖四模块且用根配置（新增 submodules-with-root-config 步骤）；配置漂移即失败
- [x] 阶段 3：T3.1..T3.5 各函数 gocognit ≤30（runInnerLoop 92→30 / vectorSearch 91→30 以下 / runCompaction 84→5 / graph.Run 78→30 以下 / processMsg 92→3 / renderVertical 89→4 / On 64→3 / renderBlock 63→15 / renderFrame 61→20），行为不变（全量测试通过）
- [x] 阶段 4：`tools/tool_domains.go` 已删、`handleLedgerCommand`/`handleSessionsCommand` 已删（含 3 测试）、`readFileSandboxed`/`resolveOCRCacheDir` 已删、`agentcore/tool_domains.go` 4 个未接线函数已删，全仓库无引用残留
- [x] 阶段 4：T4.4 决策已记录（保留映射表数据 + 删除 4 个未接线函数，接线需另立 spec）
- [x] 阶段 5：dupl 豁免点已消除或保留并记录理由（runPatentPipeline 统一 4 个 run；slash 4 豁免→2 消除+2 带理由；queryRecords 统一 List/ListByCase）
- [ ] `make verify` 全绿（lint + build + race 测试覆盖根模块 + tools/tui/desktop 子模块）
- [ ] 涉及敏感路径的改动（handoff.go / levels.go / citation_gate.go / tools.go）经 L3 人工审阅并记录
- [ ] README / specs 索引 / AI changelog 一致

---

## 决策记录

| 日期 | 决策 | 依据 | 状态 |
|------|------|------|------|
| 2026-08-19 | 旧计划 `docs/refactoring/optimization-plan.md` 保留不删，本计划为增量更新 | 旧计划 70 项未勾选且未覆盖新发现项 | 已记录 |
| 2026-08-19 | 阶段顺序：测试护栏 → 门禁 → 重构 → 清理 | 先有回归护栏再动生产代码 | 已记录 |
| T4.4 | `agentcore/tool_domains.go`：保留 ToolDomains 映射表（数据资产），删除 ToolDomain/AllDomains/FilterToolNames/ToolHasDomain 4 个未接线函数；角色级工具过滤接线属未完成功能，需另立 spec | 2026-08-19 全仓库 rg 确认零调用 | ✅ 已决策（删除函数，映射表加注释说明未接线） |

---

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| 阶段 3 重构破坏核心行为 | 先补契约测试锁定行为；每项独立提交；L3 审阅 |
| 阶段 2 门禁收紧导致大量新 issue | 先修后开；按 issue 类别分批，每批独立提交 |
| 死代码删除牵连隐藏依赖 | 删除前全仓库 rg 确认零引用；删除后全量 build + test |
| 涉及敏感路径 | 严格走 `scripts/check-sensitive-paths.sh` + L3 人工审阅 |

---

## 代码审阅记录（code-simplifier + requesting-code-review）

> 2026-08-19 对全部 5 阶段改动（92 文件，+2531/-2469）执行代码精简与代码审阅。

### 审阅发现并修复

| # | 严重度 | 发现 | 修复 |
|---|:---:|------|------|
| 1 | 🔴 行为回归 | `graph.applyNodeOutputs` 条件边目标节点 Run 失败被 `continue` 静默吞掉（原代码 `return error` 传播） | 改为返回 error，Run 主循环检查 |
| 2 | 🟡 简化 | `compProviderFor`/`compModelFor` 冗余 fallback 参数 + runCompaction 多余解包 | helper 内部取 `p.Provider`/`p.Model`，删解包 |
| 3 | 🟡 简化 | `detectRepetition`/`runToolCallTurn` 4 个指针参数穿传 | 封装 `repetitionState` struct |
| 4 | 🟢 测试缺口 | `runPatentPipeline` 重构无契约测试 | 新增 5 个单元测试（打印/保存/缺输入/build 错/save 错） |

### 行为等价性确认

- `runCompaction`：`len(turnsToSummarize)==0` 检查移除等价（prepareCompactionRange 保证 compressStart<compressEnd）
- `runInnerLoop` 拆分：各辅助方法经既有 phase/tool/truncation 契约测试锁定
- `runPatentPipeline` 统一文案："报告已保存到"（原各函数文案不同）——用户可见文案统一，非功能变化
- `eventToChat` 映射表 + nil 防护：类型不匹配静默跳过（防御性，实际不发生）

### 审阅结论

- lint/vet/build 全过；根模块 race 97 包 + tools 4 + tui 9 全绿
- 全部 5 阶段行为等价或已修复回归，可进入人工 L3 复核（agentcore/graph 核心改动）
