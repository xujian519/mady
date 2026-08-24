# Mady 对齐 Sati 专利域能力实施计划（对齐实现记录）

> 原计划归档自 Kimi 会话；本文已更新为 Mady-Sati 对齐的**最终实现记录**（2026-08-24）。
> 各阶段按批次 B0→B6 执行，每批次独立提交（commit 见各阶段标注），完整追踪见
> `docs/decisions/ai-changelog/2026-08-24.md`。

## 执行状态总览

| 批次 | 阶段 | 状态 | 关键 commit |
|---|---|---|---|
| B0 | 阶段 2 收尾 | ✅ 完成 | 80e9fb1 之前（阶段0/1/2） |
| B1 | 阶段 3 模板 | ✅ 完成 | c30321c |
| B2 | 阶段 4 质量门 | ✅ 完成 | 80e9fb1 |
| B3 | 阶段 5 溯源 | ✅ 完成 | 72cdf38 |
| B4 | 阶段 6 条款角色 + workflow | ✅ 完成 | 70d060e |
| B5 | 阶段 7 数据引擎 | ✅ 完成 | 10b03ee |
| B6 | 阶段 9 装配终验 + 文档 | ✅ 完成 | 本文 |

> 阶段 8（化学/附图分析）依赖 RDKit/vision，默认不注册，保持可选，不在本次对齐范围。

## 目标

将 Sati（`/Users/xujian/projects/Sati`）中成熟的专利产品化能力引入 Mady，并以 **XiaoNuo Agent**（`/Users/xujian/projects/XiaoNuo Agent`）作为专利/法律知识底座的数据来源。

## 范围边界

- **做**：专利专用工具、交付物模板、知识底座集成、质量门、Worker 契约与溯源、条款级 SOP 角色、工作流 manifest、数据引擎补强。
- **不做**：改造 Mady 核心运行时（agentcore/graph/协议层）已优于 Sati 的部分；不引入 Sati 的 TypeScript 运行时。

## 数据资产来源

XiaoNuo Agent 已完成数据清洗并产出以下资产，Mady **直接复用，不复建数据库**：

| XiaoNuo 资产 | 大小/数量 | Mady 使用方式 |
|---|---|---|
| `data/knowledge.db` | 运行时构建 | `$MADY_HOME/knowledge/knowledge.db` |
| `data/patent_kg.db` | 207 MB / 116k 节点 | `$MADY_HOME/knowledge/patent_kg.db` |
| `data/laws-full.db` | 152 MB / 9,121 部法规 | `$MADY_HOME/knowledge/laws-full.db` |
| `data/wiki/` | 1,574 份 Markdown | `WIKI_PATH` 直接加载 |
| `data/rules/` | 8+ YAML/JSON 规则库 | `domains/rules/data/rules/` |
| `assets/patent-provisions/` | 22 个 provision 角色 | `domains/provisions/data/` |
| `assets/roles/` | 12 个专利角色 | `skills/patent/roles/` |

`patent_kg.db` 中约 83% 的边为弱共现关系（`SIMILAR_TO`/`RELATED_TO`），Mady 在**查询层默认过滤**这类边，仅保留 `CITES`/`APPLIES`/`HAS_PRECEDENT`/`CONTAINS` 等语义边。

## 分阶段实现记录

### 阶段 0：前置准备 — ✅ 已完成

- [x] 确认三个项目均为个人项目，数据合规无阻塞。
- [x] 记录决策：`docs/decisions/ai-changelog/2026-08-24.md`。
- [x] 验证 `~/.mady/knowledge/` 软链状态（ipc-classification、laws-full.db、patent_kg.db、rules、wiki）。
- [x] 确认 Mady 能正常加载 XiaoNuo SQLite 与 wiki（B1 知识底座集成验证）。

### 阶段 1：知识底座集成 — ✅ 已完成（B1 前置）

- [x] 扩展 `bootstrap/init_knowledge.go`：自动检测并打开 `$MADY_HOME/knowledge/patent_kg.db`。
- [x] 扩展 `knowledge/sqlite/graph.go`：`LoadGraph` 优先 `knowledge.db`，空表回退 `patent_kg.db`；增加 `kg_nodes` 表存在性检查。
- [x] 扩展 `knowledge/loader/wiki.go`：支持 wiki 根目录、`Wiki/`、`cards/`、`patent-cards/` 四种来源。
- [x] 将 XiaoNuo `data/rules/` 规则、articles、orchestrations 复制到 `domains/rules/data/`（冲突文件加 `xiaonuo-` 前缀）。
- [x] 将 XiaoNuo `assets/patent-provisions/` 复制到 `domains/provisions/data/`。
- [x] 将 XiaoNuo `assets/roles/` 复制到 `skills/patent/roles/`。
- [x] 在 `knowledge/graph/query.go` 实现默认弱共现边过滤（`QueryOptions{IncludeWeakEdges:true}` 恢复）。
- [x] 验证：`make verify` 全绿。

### 阶段 2：专利专用工具补齐 — ✅ 已完成（B0）

- [x] 工具测试补齐 + QueryPaths 复杂度拆分（`knowledge/patenttools` + `knowledge/graph`）。
- [x] 装配孤儿工具：`patent_plan_task` / `patent_flexible_plan`（plantask）、`claim_chart_build`（claimchart）、`patent_worker_validate`（workercontract）进入 `domains/patent.go` filterNilTools。
- [x] SystemPrompt（五步工作法 + 工具链优先原则）追加新工具引导。
- [x] 完成 `patent_workflow_run`（见阶段 6，声明式工作流入口）。

### 阶段 3：交付物模板体系 — ✅ 已完成（B1）

- [x] `domains/doctmpl` 专利模板：`templates/patent/` 5 个模板（可专利性意见/检索报告/无效意见/OA答复/权利要求与说明书）。
- [x] `renderer_html.go` 新增 `patentHTMLStyleBlock`（A4 打印、仿宋正文、藏蓝标题、verdict/doc-meta/callout、徽章色），经 `meta.Style.Name` 选择，默认渲染不变。
- [x] 样式选择黄金用例 + 5 模板渲染 smoke 测试。

> 说明：本阶段落地为 doctmpl 模板 + A4 样式（Mady-ification），非原计划的 `doc-templates/` + `render_patent_document` 工具；如后续需要独立 `render_patent_document` 工具可再封装。

### 阶段 4：质量门与 HITL 闭环 — ✅ 已完成（B2）

- [x] `guardrails/outputgate.go`：`PatentOutputReport` + `VerifyPatentOutput`（绝对化/风险/审批/免责/引用五维核验）+ `NewPatentOutputGate` LifecycleHook（命中审批词挂起人工复核）。
- [x] `domains/claimdrafting/coverage.go`：`CoverageChecker` 逐权利要求校验特征是否被实施例覆盖（full/partial/none + 缺口 + 编号连续性 + 1000 上限 + 去重）。
- [x] `disclosure/clarity.go` + `graph.go`：`check_clarity` 节点（四维信号正则 + 语义分融合，低于阈值 HITL 中断），接线 merge_extractions→check_clarity→groundedness_filter。
- [x] 测试：outputgate 五维、coverage 全路径、clarity 中断/通过 + FullFlow 回归。

### 阶段 5：Worker 契约与溯源审计 — ✅ 已完成（B3）

- [x] `domains/provenance/`：`ProvenanceEvent`/`ProvenanceLogger`（JSONL 按日 + MADY_ENC_KEY AES-GCM 详情加密 + 禁用静默）+ `DefaultProvenanceLogger`。
- [x] 四注入点接 provenance：plantask（plan_lifecycle）、claimchart（workflow_step）、workercontract（contract_validation）、guardrails/outputgate（outputgate_suspend）。
- [x] `bootstrap/init_reasoning.go` `SetupProvenance`（通道 B）+ `domains/patent.go` patent_workflow_run 溯源。
- [x] `domains/inventiveness/feedback.go`：HITL 反馈回流（rejection/modification 落盘 `$MADY_HOME/cases/<caseId>/inventiveness-feedback.jsonl`）+ 结论节点 `FeedbackPrompt` 注入历史反馈 + caseID 路径净化。

### 阶段 6：条款级 SOP 角色与工作流 Manifest — ✅ 已完成（B4）

- [x] `domains/provisions/types.go` `ProvisionManifestEntry.WikiRoots`（增量并入事实源 manifest，文本级保留注释）+ 删除冗余 `provisions/data/manifest.yaml`。
- [x] `domains/provisions/roles.go`：`LoadRoles` 解析 `skills/patent/roles/*.yaml`（11 个角色）+ `BuildRoleListForSystemPrompt`；nuochat 补 role_id。
- [x] IPC 领域映射收敛：对比两份 `ipc-domain-map.yaml`，provisions 独有 section（A63/B01/B23/C08/F01）并入 rules 事实源（15 section 全覆盖），删除冗余。
- [x] `domains/workflows/patent/manifests/` 8 个工作流 manifest（go:embed）+ `manifest.go`（`PatentWorkflowManifest`/`LoadPatentWorkflowManifests`/`ResolveWorkflowEntryPoint`/`ValidateManifestSchema`）。
- [x] `workflow_run_tool.go`：`patent_workflow_run`（声明式路由到既有入口 + 逐步骤溯源）；装配进 patent.go filterNilTools；SystemPrompt 追加角色目录 + workflow 目录摘要。
- [x] 完成 `patent_workflow_run` 声明式工作流入口（阶段 2 列表项）。

### 阶段 7：数据引擎与 CNIPA 客户端 — ✅ 已完成（B5，方案 A）

- [x] `retrieval/domain/nuopatent/retriever.go`：外部 `nuo-patent` CLI 封装为 `DomainRetriever`（`CommandRunner` 测试注入 + argv 防注入 + ctx 超时 + 非零退出 stderr 结构化 error + 空结果容错）。
- [x] 二进制发现 `discoverBin`：`cfg.Bin > MADY_NUO_PATENT_BIN > exec.LookPath`；缺失返回 nil 不阻塞启动。
- [x] `bootstrap/init_reasoning.go`：`MADY_NUO_PATENT_RETRIEVERS=off` 关闭开关（对齐 `MADY_BROWSER_RETRIEVERS`），权威源置 composite 首位。
- [x] E2E：`MADY_E2E=1` 门控测试（仿 browser/e2e_test）。

### 阶段 8：化学/附图分析（可选，P2）

`recognize_chemical_structure`（RDKit）/`analyze_patent_figure`/`search_patent_figure`（vision/OCR）——默认不注册，保持可选扩展，不在本次对齐范围。

### 阶段 9：统一装配与文档 — ✅ 已完成（B6，本文档）

- [x] 装配核对：`bootstrap/init_reasoning.go` 清单（outputgate hook、provenance、nuo-patent 链、provisions handoffs、plantask store）+ patent.go SystemPrompt 终校（新工具 + workflow 目录 + 角色列表；免责声明与 outputgate 分属 prompt 指导层与输出校验层，不重复叠加）。
- [x] 全量验证：`go build ./...` + `go vet ./...` + `go test ./...`（110 包全绿）+ `make check-tone`（存量 0 命中，含 Unicode 转义规避）。
- [x] `AI_CHANGELOG` 全批次条目补全（`docs/decisions/ai-changelog/2026-08-24.md`）。
- [x] 本文档更新为最终实现记录。

## 验证矩阵

| 层级 | 命令 | 状态 |
|---|---|---|
| 单元/模块 | `go test -race ./<改动包>/...` | 每批 ✅ |
| 模块构建 | `go build ./...` | ✅ |
| 全量门禁 | `make verify` | ✅ |
| lint newcode | `golangci-lint --config .golangci-newcode.yml --new-from-rev=HEAD` | 每批 ✅ |
| 架构边界 | `go-arch-lint check` | 每批 ✅ |
| tone 词表 | `make check-tone` | ✅ |
| 知识库集成 | `MADY_E2E=1 go test ./retrieval/...` | 门控（默认 skip） |
| 文档一致性 | `make doc-check` | ✅ |
