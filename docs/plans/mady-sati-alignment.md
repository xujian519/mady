# Mady 对齐 Sati 专利域能力实施计划

> 本计划是 `/Users/xujian/.kimi-code/sessions/wd_mady_534a5ad89b3b/session_4d612bac-16fc-4b96-9b0a-4163f5454e50/agents/main/plans/nightcrawler-us-agent-nick-fury.md` 的持久化归档。
> 关联计划：`docs/plans/knowledge-enhancement-plan.md`（审查指南索引、判决文书解析、Skill 蒸馏）。

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

## 分阶段实施

### 阶段 0：前置准备（当前）

- [x] 确认三个项目均为个人项目，数据合规无阻塞。
- [x] 记录决策：`docs/decisions/ai-changelog/2026-08-24.md`。
- [ ] 验证 `~/.mady/knowledge/` 软链状态（ipc-classification、laws-full.db、patent_kg.db、rules、wiki）。
- [ ] 确认 Mady 能正常加载 XiaoNuo SQLite 与 wiki。

### 阶段 1：知识底座集成（P0，1–2 周）— 已完成

- [x] 扩展 `bootstrap/init_knowledge.go`：自动检测并打开 `$MADY_HOME/knowledge/patent_kg.db`。
- [x] 扩展 `knowledge/sqlite/graph.go`：`LoadGraph` 优先从 `knowledge.db` 加载，空表时回退到 `patent_kg.db`；增加 `kg_nodes` 表存在性检查。
- [x] 扩展 `knowledge/loader/wiki.go`：支持 wiki 根目录、`Wiki/`、`cards/`、`patent-cards/` 四种来源。
- [x] 将 XiaoNuo `data/rules/` 规则、articles、orchestrations 复制到 `domains/rules/data/`（冲突文件加 `xiaonuo-` 前缀）。
- [x] 将 XiaoNuo `assets/patent-provisions/` 复制到 `domains/provisions/data/`。
- [x] 将 XiaoNuo `assets/roles/` 复制到 `skills/patent/roles/`。
- [x] 在 `knowledge/graph/query.go` 实现默认弱共现边过滤（`SIMILAR_TO`/`RELATED_TO`），可通过 `QueryOptions{IncludeWeakEdges: true}` 恢复。
- [x] 验证：`make verify` 全绿。

### 阶段 2：专利专用工具补齐（P0，3–4 周）

新增/补强以下工具：

- `patent_workflow_run` —— 声明式工作流入口
- `patent_plan_task` / `patent_flexible_plan` —— HITL 计划状态机
- `patent_wiki_search` —— wiki 卡片检索
- `patent_kg_query` —— 知识图谱查询
- `claim_chart_build` —— 权利要求对照图
- `patent_case_search` —— 判例/无效决定检索
- `knowledge_note_save` —— 项目笔记沉淀
- `patent_worker_validate` —— Worker 输出契约校验

可选（P2）：`analyze_patent_figure`、`search_patent_figure`、`recognize_chemical_structure`。

### 阶段 3：交付物模板体系（P0，2 周）

在 `doc-templates/patent-deliverables/` 下新增 HTML 单文件模板：

- 可专利性分析意见书
- 检索报告
- 审查意见答复
- 无效宣告意见
- 权利要求书/说明书

并新增 `render_patent_document` 工具封装。

### 阶段 4：质量门与 HITL 闭环（P1，2–3 周）

- 新建 `domains/outputgate/`：专利输出门、风险/审批/引用核验、挂起/通过/拒绝/修改回调。
- 新建 `domains/claimdrafting/coverage.go`：Claim Coverage 矩阵。
- 新建 `domains/disclosure/clarity.go`：交底书清晰度评分。
- 新建 `domains/feedback/inventiveness_feedback.go`：HITL 反馈回流。

### 阶段 5：Worker 契约与溯源审计（P1，2 周）

- 新建 `domains/workercontract/`：Worker 注册表、输入/输出契约、校验、监控。
- 新建 `domains/provenance/`：PROV-O-lite 溯源收集、SQLite 落盘、CSV/JSON 导出。

### 阶段 6：条款级 SOP 角色与工作流 Manifest（P1，2 周）

- 将 XiaoNuo `assets/patent-provisions/` 迁移为 `skills/patent/provisions/` 的 SKILL.md。
- 在 `domains/workflows/patent/manifests/` 定义 8 个 Sati 工作流 manifest。
- 实现 `patent_workflow_run` 按 manifest ID 分发。

### 阶段 7：数据引擎与 CNIPA 客户端（P2，2–3 周）

- 方案 A：保留外部 `nuo-patent` CLI，增加批量调用、MANIFEST 恢复、结果缓存。
- 新增 `retrieval/domain/browser/cnipa_client.go`：CNIPA 公布公告独立客户端。

### 阶段 8：化学/附图分析（P2，可选，2 周）

- `recognize_chemical_structure`（依赖 RDKit）
- `analyze_patent_figure` / `search_patent_figure`（依赖 vision/OCR）

默认不注册，作为可选扩展。

### 阶段 9：统一装配与文档（P1，1 周）

- 更新 `domains/patent.go` / `domains/assemble.go` 默认注入新工具/扩展。
- 更新 `README.md` / `CLAUDE.md` / `AGENTS.md` 专利能力说明。
- 每个阶段完成后追加 `docs/decisions/ai-changelog/` 条目。

## 验证矩阵

| 层级 | 命令 |
|---|---|
| 单元测试 | `go test -race ./<改动包>/...` |
| 模块构建 | `go build ./...` |
| 全量门禁 | `make verify` |
| 知识库集成 | `MADY_E2E=1 go test ./retrieval/... ./knowledge/...` |
| 端到端工作流 | `mady patent workflow --manifest patent_novelty_v1` |
| 文档一致性 | `make doc-check` |

## 估算总工期

约 **11–15 周（2.5–3.5 个月）**，数据直接复用 XiaoNuo 已处理资产，无需数据清洗工期。
