# Mady 开发路线图

> 更新日期：2026-07-14 | 代码基线：`5a75779`
>
> 基于 Manus AI 路线图审阅 + 项目代码深度验证 + 7-9 月执行反馈

---

## 一、产品定位

**Mady = 证据驱动的专利案件工作台**

**北极星**：每个专业工作成果中，经人工确认、可定位到权威来源且无需结构性重做的证据化结论数量 ÷ 专业人员投入时间。

**首要用户旅程**：技术交底书 → 技术特征树 → 现有技术证据包 → 新颖性初评 → 人工复核报告 → 导出

## 二、模块成熟度（代码验证后修正）

| 模块 | 路线图原评分 | 修正评分 | 关键事实 |
|------|:---------:|:------:|------|
| 知识/检索 | 3.5 | **5.0** | FTS5 trigram BM25 + 144K 向量并行 + 三路 RRF + cross-encoder |
| 评估框架 | 2.0 | **5.0** | 6 套专利法条基准 + LLM Judge + RAG 评估 |
| 推理引擎 | — | **5.0** | FactBlackboard + Syllogism + 多假设 + 五阶段引擎 |
| Agent 运行时 | 4.0 | **4.5** | 生命周期/事件/压缩/Handoff/流式/结构化输出 |
| TUI | 3.5 | **4.5** | 8 层 Elm 架构，斜杠命令完整 |
| 工程 CI/CD | 3.0 | **4.0** | golangci-lint v2 + 多平台矩阵 + Codecov + eval 门禁 |
| 工作流 | 4.0 | **4.0** | Pipeline/Parallel/Router + PER 模式 |
| 状态持久化 | 3.0 | **4.0** | SQLiteCheckpoint + SQLiteMemory + SQLiteApproval |
| 护栏安全 | 1.5 | **3.5** | 两层防御 + 熔断器 + fails-closed |
| 人机审批 | 2.0 | **3.0** | ApprovalState 状态机 + SQLite 持久化 + /approve /reject |
| 专业工作流 | 2.5 | **3.0** | disclosure 10 节点 Pregel → LLM 新颖性 + 证据包裹生成 |
| 证据/引用 | 2.0 | **2.5** | EvidenceSpan + ClaimBinding + ConflictDetector（+ Receipt/Ledger） |
| 产品可用性 | 1.5 | **2.0** | DOCX/Markdown 导出 + TUI 审批流程 |

## 三、7-9 月执行成果（2026.07.14 - 2026.09.30）

### P0 止血（第 1-2 周）✅

- CI 修复：`go mod tidy` / `go vet` / `go build` / `go test` 全绿
- lint 清零：修复 11 个 golangci-lint v2.12.2 问题
- 分支保护：开启 PR 审查 + 状态检查 + 禁止直接推送
- 评估基线：41 个 Golden Case，6 套法条基准，静态回放 1.0
- 功能冻结：README 收敛为首要用户旅程，明确不做事项

### P1A 证据底座（第 3-6 周）✅

- **EvidenceSpan**：证据实体（文档版本、页码、原文哈希、引用方向）+ ClaimBinding + ConflictDetector
- **Case 实体**：CaseStage 状态机（7 阶段）+ ProjectRecord 业务字段（CaseType/FilingNumber/Jurisdiction/Confidentiality）
- **Approval 状态机**：drafted → pending → approved/modified/rejected/canceled + 终止态隔离
- **SQLiteApprovalStore**：WAL 模式持久化 + Save/List/ListByCase/Delete
- **统一存储契约**：store.CaseStore 接口（CaseID/RunID/Migrate/Version），3 个 store 实现

### P1B 闭环集成（第 7-9 周）✅

- **新颖性初判节点**：LLM 驱动（JSON Schema 输出）+ 启发式回退 + sync.Once 缓存
- **证据包裹生成器**：特征 → EvidenceSpan 映射（PriorArtStatus → 证据方向）
- **端到端测试**：证据构建 + JSON 提取 + Schema 完整性

### P1C 打磨（第 10-11 周）✅

- **DOCX/Markdown 导出**：6 章节报告 + pandoc 转换 + 免责声明
- **CI 评估门禁**：eval-benchmark job + 回归检测
- **TUI 审批增强**：/approve /reject 接入 Agent 执行 + 审核状态显示

### 代码审查修复

max effort 审查（10 角度 × 10 并行）发现 15 项，全部修复。详见提交 `5a75779`。

## 四、10-12 月展望

| 阶段 | 时间 | 核心目标 |
|------|------|---------|
| P2A | 10 月 | 10-20 道公开专利考试真题 → Golden Set 第一层 |
| P2B | 10-11 月 | 3-5 件代理人合作脱敏案件 → Golden Set 第二层 |
| P3 | 11-12 月 | 盲测 10 个案件，人工采纳率/修改率/拒绝率测量 |

## 五、停止规则

> **main 不绿 → 不加新功能**
>
> **Golden Set 不能说明质量差异 → 不换模型/Prompt**
>
> **首个专利工作流未通过专家盲测 → 不启动第二垂直**
