# 专利能力超越考试大纲 — 落地计划

> 基于《专利代理师资格考试大纲（2025）》对标分析，为 Mady 专利领域能力跃升制订的详细落地计划。
> 核心论点：考试大纲是**执业准入门槛**（知识掌握 + 基础应用），不是能力天花板。
> 智能体要超越的不是"把大纲学得更熟"，而是在大纲**不覆盖、不考察、无法考察**的维度上建立能力。
> 每个任务严格遵循 Mady 现有架构（Extension / Reasoning Graph / Rules Engine / Domain Retriever / Session）。

---

## 架构适配原则

所有新特性复用现有机制，避免另起炉灶：

| 超越维度 | Mady 对应机制 | 说明 |
|---|---|---|
| 审查员角色模拟 | `reasoning` 对抗式子图 + `rules.Engine` | 复用 `BuildMultiHypothesisSubgraph` 的 advocate/judge 模式 |
| 决策回溯链 | `session` + `reasoning.wiring.ConfirmedRuleWriter` | 扩展 confirmed-rules 持久化模式到决策记录 |
| 判例库 | `retrieval/domain/sqlite.PatentDomainRetriever` | 新增判例数据源，复用 FTS + 打分融合 |
| 多方案权衡 | `reasoning/multi_hypothesis.go` + `fact_blackboard.go` | 从二元对抗扩展到 N 候选方案评估 |
| 法规变更检测 | `tools.WebSearchTool` + `agentcore.LifecycleHook` | 定时检索 + 通知 hook |
| 对抗性无效预演 | `reasoning` 多角色子图 + `domains/rules` | 请求方/权利方/合议组三方博弈 |
| PCT/多法域 | `domains/rules` 多 domain + `retrieval` | 扩展规则域和检索源，不改核心 |

依赖链：
```
任务1 审查员模拟器 (独立，复用 rules+reasoning) ─┐
任务2 决策回溯链 (独立，扩展 session+wiring)  ──┤
                                                ├→ 任务6 对抗性无效预演 (依赖 1+2)
任务3 判例库 (独立，扩展 retrieval) ────────────┤
任务4 多方案权衡 (独立，扩展 reasoning) ─────────┤
                                                ├→ 任务7 PCT/多法域 (依赖 3+4)
任务5 法规变更检测 (独立，扩展 tools+hook) ─────┘
```

---

## 总览

| 阶段 | 任务 | 优先级 | 工作量 | 依赖 |
|---|---|---|---|---|
| Phase 1 | 任务1 审查员角色模拟器 | P0 | 6-8 d | 无 |
| Phase 1 | 任务2 案件决策回溯链 | P0 | 5-7 d | 无 |
| Phase 2 | 任务3 判例库接入 | P1 | 10-12 d | 无 |
| Phase 2 | 任务4 多方案权衡矩阵 | P1 | 6-8 d | 无 |
| Phase 3 | 任务5 法规变更检测器 | P2 | 3-4 d | 无 |
| Phase 3 | 任务6 对抗性无效预演 | P2 | 8-10 d | 任务1 + 任务2 |
| Phase 4 | 任务7 PCT/多法域扩展 | P3 | 12-15 d | 任务3 + 任务4 |

**合计：50-64 人天**（约 10-13 周，按 1 人全职）。建议 Phase 1+2 并行推进，Phase 3 在 1+2 完成后启动，Phase 4 视资源安排。

---

## Phase 1: P0 — 质量内控基础（第 1-3 周）

### 任务 1：审查员角色模拟器（Patent Examiner Simulator）

**目标：** 撰写/答复完成后，自动切换到"审查员视角"对申请文件做一次完整审查，输出模拟审查意见（OA），让代理师在提交前发现可被攻击的弱点。这是考试大纲里完全没有的"自我对抗"能力。

**价值：**
- 把"被动答复真实 OA"前置为"主动预审"，减少实审来回次数
- 生成的模拟 OA 可直接用于训练代理师的答复能力（形成闭环训练数据）

**范围边界：**
- ✅ 做：对权利要求书 + 说明书做新颖性/创造性/清楚性/单一性/实用性五维审查，输出结构化模拟 OA
- ✅ 做：调用现有检索工具找对比文件支撑创造性攻击
- ❌ 不做：替代真实审查员做最终授权判断；不生成正式法律意见

**技术方案：**

复用 `BuildMultiHypothesisSubgraph`（`domains/reasoning/multi_hypothesis.go:57`）的 advocate A/B → merge → judge 架构，新增一个 `examiner` 角色。

| 文件 | 改动类型 | 说明 |
|---|---|---|
| `domains/examiner/doc.go` | 新增 | 包文档 |
| `domains/examiner/simulator.go` | 新增 | `ExaminerSimulator` 核心：接收申请文件 + 检索器，输出 `SimulatedOA` |
| `domains/examiner/checks.go` | 新增 | 五维检查器（新颖性/创造性/清楚性/单一性/实用性），每个检查复用对应 `rules.Engine.RulesByDomain` |
| `domains/examiner/graph.go` | 新增 | 构建 examiner 子图，复用 `BuildMultiHypothesisSubgraph` 模式（pro=可授权 / con=应驳回 → judge） |
| `domains/examiner/types.go` | 新增 | `SimulatedOA`、`ExaminerFinding`、`AttackVector` 类型定义 |
| `domains/rules/oa_parser.go` | 扩展 | 复用现有 OA 解析逻辑的逆向：从 finding 生成标准格式 OA 文本 |
| `domains/patent.go:85` | 扩展 | `PatentAgentConfig` 增加 "撰写完成后调用 examiner" 的 LifecycleHook |

**设计细节：**

```go
// domains/examiner/types.go

// SimulatedOA 是审查员模拟器输出的模拟审查意见。
type SimulatedOA struct {
    CaseID      string           `json:"case_id"`
    ExaminedAt  string           `json:"examined_at"`
    OverallVerdict Verdict       `json:"overall_verdict"` // grant / reject / amend-required
    Findings    []ExaminerFinding `json:"findings"`
    AttackVectors []AttackVector `json:"attack_vectors"` // 最可能被真实审查员/无效请求人攻击的点
    SuggestedAmendments []string `json:"suggested_amendments"`
}

// ExaminerFinding 是单一维度的审查结论。
type ExaminerFinding struct {
    Dimension  string  `json:"dimension"`  // novelty/inventiveness/clarity/unity/utility
    RuleID     string  `json:"rule_id"`    // 关联 rules.Engine 的 RuleID
    Severity   string  `json:"severity"`   // critical/major/minor
    Rationale  string  `json:"rationale"`  // 为什么认为有问题（三段论格式）
    Evidence   []string `json:"evidence"`  // 对比文件引用
}
```

```go
// domains/examiner/simulator.go

// ExaminerSimulator 以审查员视角审查专利申请文件。
type ExaminerSimulator struct {
    rules    *rules.Engine     // 规则引擎（新颖性 A22.2、创造性 A22.3 等）
    retriever domain.Retriever // 检索对比文件
    builder   reasoning.NodeBuilder
}

// Simulate 对给定的权利要求书+说明书执行五维审查。
func (s *ExaminerSimulator) Simulate(ctx context.Context, req SimulateRequest) (*SimulatedOA, error) {
    // 1. 逐维度调用对应规则集
    //    novelty   → rules.RulesByDomain("patent_novelty")
    //    inventive → rules.RulesByDomain("patent_inventiveness")
    //    clarity   → rules.RulesByDomain("patent_clarity")
    // 2. 创造性维度走对抗式子图（pro/con/judge）
    // 3. 汇总 findings → 攻击向量分析
    // 4. 生成建议修改
}
```

**验收标准：**
- [ ] 输入一份示例权利要求书，能输出包含至少 2 个 findings 的 `SimulatedOA`
- [ ] 创造性维度的攻击能引用检索到的对比文件（非空 evidence）
- [ ] 生成的模拟 OA 文本能被现有 `oa_parser.go` 正确反向解析（闭环验证）
- [ ] 单次模拟耗时 < 60s（含检索）
- [ ] 新增 `domains/examiner/` 测试覆盖率 > 70%

**风险与对策：**
- 风险：模拟 OA 质量取决于对比文件检索的召回率 → 对策：复用 `search-commander` skill 的多轮检索策略
- 风险：审查员视角与代理师视角混淆 → 对策：用独立的 system prompt 强制角色切换，不复用 patent-agent 的 prompt

---

### 任务 2：案件决策回溯链（Case Decision Provenance）

**目标：** 记录撰写过程中每个关键决策（如"独立权利要求为何这样划界""为何放弃某实施方式""保护范围为何选宽不选窄"）的理由，形成可检索的决策链。后续答复/无效/诉讼阶段可回溯当时的考量，避免禁止反悔陷阱。

**价值：**
- 答复 OA 时能精准定位"当时为什么这么写"，针对性反驳而非泛泛陈述
- 无效/诉讼阶段能快速核查是否有禁止反悔风险（file wrapper estoppel）
- 案件交接时，接手代理师能快速理解决策脉络

**范围边界：**
- ✅ 做：结构化记录撰写阶段的关键决策点（权利要求划界/范围选择/实施方式取舍/修改策略）
- ✅ 做：按 caseID 索引，支持后续阶段检索
- ❌ 不做：记录每一次 LLM 调用（那是 trace 的工作）；不做实时拦截（由代理师主动触发或 hook 自动记录）

**技术方案：**

扩展 `reasoning.wiring.ConfirmedRuleWriter`（`domains/reasoning/wiring/confirmed_rule_writer.go:23`）的持久化模式——它已经把确认的规则集写到 `$MADY_HOME/knowledge/confirmed-rules/`，同理把决策记录写到 `$MADY_HOME/knowledge/case-decisions/`。

| 文件 | 改动类型 | 说明 |
|---|---|---|
| `domains/case/decision_log.go` | 新增 | `DecisionLogger`：结构化记录决策点，JSON 持久化 |
| `domains/case/decision_query.go` | 新增 | `DecisionQuery`：按 caseID/阶段/决策类型检索历史决策 |
| `domains/case/types.go` | 新增 | `DecisionRecord`、`DecisionType` 枚举（claim_scope/embodiment_drop/amendment_strategy/...） |
| `domains/case/doc.go` | 新增 | 包文档 |
| `domains/reasoning/wiring/confirmed_rule_writer.go` | 参考 | 复用其目录管理 + JSON 序列化模式，不改动原文件 |
| `agentcore/hooks.go` | 扩展 | 新增 `AfterDecisionHook`，在关键决策点自动触发记录 |
| `domains/patent.go` | 扩展 | 注册决策记录 hook |

**设计细节：**

```go
// domains/case/types.go

// DecisionType 枚举关键决策类别。
type DecisionType string

const (
    DecisionClaimScope         DecisionType = "claim_scope"          // 权利要求保护范围选择
    DecisionClaimLayering      DecisionType = "claim_layering"       // 独/从属层次设计
    DecisionEmbodimentDrop     DecisionType = "embodiment_drop"      // 放弃某实施方式
    DecisionAmendmentStrategy  DecisionType = "amendment_strategy"   // 修改策略
    DecisionFilingStrategy     DecisionType = "filing_strategy"      // 申请类型/分案/连续案策略
)

// DecisionRecord 记录一个关键决策及其理由。
type DecisionRecord struct {
    ID          string        `json:"id"`
    CaseID      string        `json:"case_id"`
    Stage       string        `json:"stage"`        // drafting/oa_response/invalidation/...
    Type        DecisionType  `json:"type"`
    Decision    string        `json:"decision"`     // 做了什么决策
    Rationale   string        `json:"rationale"`    // 为什么这样决策（关键！）
    Alternatives []string     `json:"alternatives"` // 考虑过但放弃的备选方案
    Risks       []string      `json:"risks"`        // 已识别的风险
    RelatedClaims []string    `json:"related_claims"` // 关联的权利要求编号
    DecidedAt   string        `json:"decided_at"`
    DecidedBy   string        `json:"decided_by"`   // agent name / human
}
```

```go
// domains/case/decision_log.go

// DecisionLogger 持久化案件决策记录。
// 写入 $MADY_HOME/knowledge/case-decisions/{caseID}_{timestamp}.json
type DecisionLogger struct {
    dir string // 由 util.ResolveDataDir("knowledge/case-decisions") 解析
}

// Log 记录一个决策点。
func (l *DecisionLogger) Log(rec DecisionRecord) (string, error) { ... }

// QueryByCase 返回某案件的所有决策记录，按时间排序。
func (l *DecisionLogger) QueryByCase(caseID string) ([]DecisionRecord, error) { ... }
```

**验收标准：**
- [ ] 撰写流程中能自动记录至少 3 类决策（claim_scope / claim_layering / embodiment_drop）
- [ ] 每条记录的 `rationale` 非空且 > 50 字（不能是空洞理由）
- [ ] 后续阶段（如 OA 答复）能通过 caseID 检索到撰写阶段的决策
- [ ] 禁止反悔扫描：给定一个权利要求修改，能标记"此修改是否与历史决策冲突"
- [ ] JSON 文件格式正确，可被 `jq` 解析

**风险与对策：**
- 风险：决策理由质量差（空洞套话）→ 对策：接入 `slop_engine.go` 对 rationale 做套话检测，拒绝记录低质量理由
- 风险：记录过多噪音 → 对策：只记录 `DecisionType` 枚举内的关键决策，非每次 LLM 调用

---

## Phase 2: P1 — 知识与决策深化（第 4-7 周）

### 任务 3：判例库接入（Case Law Knowledge Base）

**目标：** 构建复审决定/无效决定/法院判例的结构化知识库，让智能体从"套用规则条文"升级为"类比真实判例"。考试大纲只考规则记忆，实务需要判例支撑。

**价值：**
- 创造性答复/无效理由能引用同类案件的裁判要旨，说服力远高于纯条文引用
- 预判审查员/合议组倾向（基于历史同类案件的裁决模式）

**范围边界：**
- ✅ 做：数据源接入（国知局复审无效决定 + 最高院知产判例）、结构化索引、按技术领域/法律问题检索
- ✅ 做：裁判要旨提取（从决定书正文抽取核心裁判观点）
- ❌ 不做：全量历史判例入库（先做近 3 年高频领域）；不做判例效力判断（由代理师确认）

**技术方案：**

复用 `PatentDomainRetriever`（`retrieval/domain/sqlite/patent_retriever.go:34`）的 FTS + 打分架构，新增一个 `CaseLawRetriever`。

| 文件 | 改动类型 | 说明 |
|---|---|---|
| `retrieval/domain/sqlite/caselaw_retriever.go` | 新增 | `CaseLawRetriever`：复用 SQLiteStore 的 FTS，新增判例字段索引 |
| `retrieval/domain/sqlite/caselaw_schema.go` | 新增 | 判例表结构（案号/决定号/技术领域/法律问题/裁判要旨/效力层级） |
| `retrieval/domain/sqlite/caselaw_ingest.go` | 新增 | 数据导入器：从决定书原文抽取结构化字段 |
| `scripts/import_caselaw/` | 新增 | 数据抓取脚本（国知局复审无效公告 + 最高院裁判文书网） |
| `domains/rules/data/caselaw/` | 新增 | 高价值判例的 YAML 结构化摘要（人工精选 + 校对） |
| `retrieval/domain/domain.go` | 扩展 | 注册 `CaseLawRetriever` 到多源融合（RRF） |

**数据源优先级：**
1. 国知局复审委员会复审决定（无效请求核心依据）
2. 国知局无效宣告决定（无效答辩核心依据）
3. 最高人民法院知识产权法庭判决（侵权/确权终审）
4. 各地知产法院典型案例

**验收标准：**
- [ ] 能按"技术领域 + 法律问题"组合检索（如"化学领域 + 创造性"）
- [ ] 返回结果包含裁判要旨摘要（非全文）
- [ ] 判例能被 `ExaminerSimulator`（任务1）和后续无效预演（任务6）调用
- [ ] 初始入库 > 500 条精选判例（覆盖机械/化学/电学/软件四大领域）
- [ ] 检索响应 < 500ms

**风险与对策：**
- 风险：数据源抓取被反爬 → 对策：优先用已公开的批量数据包；必要时用 kimi-webbridge skill
- 风险：判例结构化提取准确率 → 对策：裁判要旨先人工校对 100 条作为黄金集，再训练提取规则

---

### 任务 4：多方案权衡矩阵（Multi-Option Tradeoff Matrix）

**目标：** 对每个关键决策点（保护范围宽窄、修改幅度、是否提分案），强制生成 2-3 个候选方案，各自标注多维度评估，输出权衡矩阵。考试题有标准答案，实务只有权衡。

**价值：**
- 避免代理师"只给一个方案"的认知局限
- 委托人能基于完整信息做商业决策（而非被代理师单一视角绑架）

**范围边界：**
- ✅ 做：对指定决策点生成 N 个候选方案 + 多维评分 + 推荐理由
- ✅ 做：标记"需要委托人确认的关键风险"
- ❌ 不做：自动做最终决策（决策权归委托人/代理师）

**技术方案：**

扩展 `multi_hypothesis.go` 的二元对抗（advocate A/B）到 N 候选方案评估。

| 文件 | 改动类型 | 说明 |
|---|---|---|
| `domains/reasoning/tradeoff.go` | 新增 | `TradeoffMatrix` 类型和生成器 |
| `domains/reasoning/tradeoff_graph.go` | 新增 | N 候选方案并行评估子图（每个方案独立走 ReAct → 汇总打分） |
| `domains/reasoning/types.go` | 扩展 | 新增 `CandidateOption`、`EvaluationDimension` 类型 |
| `domains/reasoning/multi_hypothesis.go` | 参考 | 复用其子图构建模式，不改动原文件 |

**设计细节：**

```go
// domains/reasoning/tradeoff.go

// EvaluationDimension 是评估候选方案的维度。
type EvaluationDimension string

const (
    DimGrantProbability    EvaluationDimension = "grant_probability"     // 授权概率
    DimProtectionBreadth   EvaluationDimension = "protection_breadth"    // 保护范围
    DimInfringementDetect  EvaluationDimension = "infringement_detectability" // 侵权可检测性
    DimAmendmentRoom       EvaluationDimension = "amendment_room"        // 后续修改空间
    DimCost                EvaluationDimension = "cost"                  // 费用/周期
)

// CandidateOption 是一个候选方案及其多维度评分。
type CandidateOption struct {
    ID          string                       `json:"id"`
    Title       string                       `json:"title"`
    Description string                       `json:"description"`
    Scores      map[EvaluationDimension]float64 `json:"scores"` // 0-1
    Rationale   string                       `json:"rationale"`
    Risks       []string                     `json:"risks"`
}

// TradeoffMatrix 是某决策点的完整权衡矩阵。
type TradeoffMatrix struct {
    DecisionPoint string            `json:"decision_point"` // 如"独立权利要求保护范围"
    Options       []CandidateOption `json:"options"`
    Recommendation string           `json:"recommendation"`  // 推荐方案 ID + 理由
    NeedsUserConfirm []string       `json:"needs_user_confirm"` // 需委托人拍板的关键风险
}
```

**评估维度权重（可配置）：**
- 撰写阶段：保护范围 > 授权概率 > 侵权可检测性
- 答复阶段：授权概率 > 修改空间 > 保护范围维持
- 无效阶段：存活概率 > 保护范围维持 > 修改空间

**验收标准：**
- [ ] 给定一个决策点（如"权利要求 1 保护范围"），能生成 ≥ 2 个候选方案
- [ ] 每个方案在 5 个维度上都有评分（非空）
- [ ] 推荐方案附理由，且标注至少 1 个"需委托人确认"的风险
- [ ] 矩阵能以 Markdown 表格渲染（供 TUI/Chat 展示）

---

## Phase 3: P2 — 动态与对抗（第 8-11 周，依赖 Phase 1）

### 任务 5：法规变更检测器（Regulation Change Detector）

**目标：** 定期检测专利法/实施细则/审查指南/司法解释的变更，自动更新知识库并标记受影响的在办案件。考试大纲是静态快照，实务需要追踪法规演进。

**价值：**
- 避免用过时规则办案（如 2021 专利法大修、2023 审查指南修订）
- 在办案件受新法规影响时主动预警

**范围边界：**
- ✅ 做：监控国知局/最高院官方公告，diff 检测条款变更，通知受影响案件
- ❌ 不做：自动修改规则 YAML（变更需人工确认后入库）

**技术方案：**

| 文件 | 改动类型 | 说明 |
|---|---|---|
| `domains/regulation/watcher.go` | 新增 | `RegulationWatcher`：定时拉取官方公告，diff 检测变更 |
| `domains/regulation/diff.go` | 新增 | 条文级 diff（新增/修改/删除），输出 `ChangeSet` |
| `domains/regulation/impact.go` | 新增 | 变更影响分析：扫描在办案件，标记受影响的 case |
| `domains/regulation/sources.go` | 新增 | 数据源配置（国知局公告 URL / 最高院 URL） |
| `agentcore/hooks.go` | 扩展 | 注册定时 LifecycleHook（每日/每周触发） |
| `tools/web_search.go` | 复用 | 检索官方公告 |

**验收标准：**
- [ ] 能检测到条文级变更（非整页变更误报）
- [ ] 变更通知包含：变更条款 + 变更类型（新增/修改/废止）+ 影响的在办案件列表
- [ ] 误报率 < 10%（以人工抽检为准）

---

### 任务 6：对抗性无效预演（Adversarial Invalidation Drill）

**目标：** 撰写完成后/授权后，模拟"如果我是无效请求人会怎么打这份专利"，提前加固权利要求或准备防御预案。这是考试大纲完全没有的攻防能力。

> **依赖：** 任务1（审查员模拟器提供单维审查）+ 任务2（决策回溯链提供防御依据）+ 任务3（判例库提供攻击先例）

**价值：**
- 撰写阶段就能识别"最可能被无效的权利要求"，提前加固
- 授权后能准备无效答辩预案

**范围边界：**
- ✅ 做：三方角色博弈（请求方/权利方/合议组），输出无效风险报告 + 防御建议
- ❌ 不做：替代真实无效程序；不生成正式法律意见

**技术方案：**

构建三方对抗子图，复用 `reasoning` 图框架。

| 文件 | 改动类型 | 说明 |
|---|---|---|
| `domains/invalidation/drill.go` | 新增 | `InvalidationDrill`：编排三方博弈 |
| `domains/invalidation/requester.go` | 新增 | 请求方角色：寻找无效理由（新颖性/创造性/公开不充分/...） |
| `domains/invalidation/defender.go` | 新增 | 权利方角色：反驳 + 修改权利要求 |
| `domains/invalidation/panel.go` | 新增 | 合议组角色：裁决（复用 `syllogism.go` 三段论裁判） |
| `domains/invalidation/graph.go` | 新增 | 三方对抗子图：requester → defender → panel → （迭代 N 轮）→ verdict |
| `domains/invalidation/types.go` | 新增 | `InvalidationReport`、`AttackPath`、`DefensePlan` |

**博弈流程：**
```
请求方攻击（基于判例库找先例 + 规则找法条依据）
    ↓
权利方防御（基于决策回溯链找撰写理由 + 修改权利要求）
    ↓
合议组裁决（三段论：大前提=法条 / 小前提=事实 / 结论=是否成立）
    ↓
若攻击成立且防御失败 → 标记为高风险，输出加固建议
若防御成功 → 标记为已验证，进入下一轮攻击
（最多 3 轮）
```

**验收标准：**
- [ ] 能输出至少 2 个攻击路径（不同无效理由）
- [ ] 每个攻击路径有判例或法条支撑
- [ ] 防御建议能引用撰写阶段的决策理由（任务2 的决策链）
- [ ] 最终风险报告含"高风险权利要求"清单 + 加固建议

---

## Phase 4: P3 — 多法域扩展（第 12-15 周，依赖 Phase 2）

### 任务 7：PCT 及多法域知识扩展（Multi-Jurisdiction Expansion）

**目标：** 超越单法域（中国），支持 PCT 全流程 + 美欧日韩主要法域的申请策略。考试大纲对中国法考察细致，但对国际申请只要求"了解"。

> **依赖：** 任务3（判例库架构可复用为各国法规库）+ 任务4（权衡矩阵可扩展为多国申请策略）

**范围边界：**
- ✅ 做：PCT 全流程知识（国际申请/检索/初步审查/进入国家阶段）、主要法域差异对比
- ✅ 做：多国申请策略权衡（在哪国申请/何时进入国家阶段/翻译修改策略）
- ❌ 不做：替代各国本地代理师（需与当地代理机构协作）；不做实时法域法规全量追踪

**技术方案：**

| 文件 | 改动类型 | 说明 |
|---|---|---|
| `domains/rules/data/jurisdictions/` | 新增 | 各国专利法结构化数据（US/EU/JP/KR） |
| `domains/rules/data/pct/` | 新增 | PCT 全流程规则 + 时限 |
| `retrieval/domain/jurisdiction_retriever.go` | 新增 | 多法域法规检索（复用 SQLiteStore FTS） |
| `domains/pct/timeline.go` | 新增 | PCT 时限计算器（优先权日/进入国家阶段截止） |
| `domains/pct/strategy.go` | 新增 | 多国申请策略生成（复用任务4 权衡矩阵） |
| `domains/patent.go` | 扩展 | PatentAgentConfig 增加 PCT/国际申请能力声明 |

**验收标准：**
- [ ] 给定一个优先权日，能计算 PCT 各阶段时限（国际检索/初步审查/进入国家阶段）
- [ ] 能对比同一发明在中/美/欧的授权条件差异（如软件专利可专利性边界）
- [ ] 多国申请策略权衡矩阵能输出"推荐申请国家 + 理由 + 费用估算"

---

## 风险与跨任务依赖

### 关键依赖路径
```
任务1 审查员模拟器 ──┐
                    ├──→ 任务6 无效预演（需要 1 的单维审查 + 2 的决策链 + 3 的判例）
任务2 决策回溯链 ────┘
                    ┌──→ 任务7 多法域（需要 3 的检索架构 + 4 的权衡矩阵）
任务3 判例库 ───────┤
任务4 权衡矩阵 ────┘
任务5 法规检测（独立，可任意时间启动）
```

### 共性风险

| 风险 | 影响 | 对策 |
|---|---|---|
| LLM 幻觉导致法条/判例引用错误 | 高（专业性领域零容忍） | 所有法条/判例引用必须经 `rules.Engine` 或 `CaseLawRetriever` 校验，禁止 LLM 自由编造 |
| 检索召回率不足导致漏检 | 高 | 复用 `search-commander` skill 多轮检索；关键判断走多源融合（RRF） |
| 角色混淆（审查员/代理师/请求人视角串味） | 中 | 每个角色独立 system prompt + 独立 session，不复用 patent-agent 配置 |
| 知识库时效性 | 中 | 任务5 法规检测器解决法规侧；判例库定期增量更新 |
| 护栏触发（涉及法律意见边界） | 中 | 所有输出保留"AI 辅助生成，不构成正式法律意见"声明（现有 `PatentAgentConfig` 已有） |

### 安全红线提醒（参考 AGENTS.md）

涉及以下文件的改动需人工审阅，不可直接合入：
- `domains/rules/data/articles/*.yaml`（规则数据修改）
- `guardrails/`（护栏文案）
- `disclosure/report.go`（报告中断信号）

---

## 验收标准汇总（里程碑）

| 里程碑 | 完成任务 | 验收标志 |
|---|---|---|
| M1（第 3 周末） | 任务1 + 任务2 | 一份示例申请文件能自动产出：模拟 OA + 决策回溯链 |
| M2（第 7 周末） | + 任务3 + 任务4 | 模拟 OA 能引用判例；关键决策点能输出权衡矩阵 |
| M3（第 11 周末） | + 任务5 + 任务6 | 法规变更能预警；撰写后能输出无效风险报告 |
| M4（第 15 周末） | + 任务7 | 支持 PCT 全流程 + 多国申请策略 |

---

## 与考试大纲的能力对照（交付后预期）

| 考试大纲维度 | 大纲要求 | 交付后 Mady 能力 |
|---|---|---|
| 知识掌握 | 中国法规则记忆 | + 判例库 + 法理理解 + 法规动态追踪 |
| 撰写能力 | 符合格式 + 基础清楚性 | + 审查员模拟预审 + 无效预演加固 + 决策回溯 |
| 答复能力 | 针对性陈述 | + 决策回溯精准反驳 + 多方案权衡 |
| 无效能力 | 基础请求书/陈述书 | + 三方对抗博弈 + 判例支撑攻击 |
| 国际申请 | "了解"PCT | + PCT 全流程时限 + 多国策略权衡 |
| 商业价值 | 不考察 | + FTO 预警 + 布局建议（任务3/4 衍生） |

---

*本计划基于 2026-07-21 的代码库状态制定，执行时需根据实际进度调整。每个任务启动前应先用 `codegraph_context` 确认依赖模块的最新接口。*
