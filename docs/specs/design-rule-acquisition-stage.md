# 设计文档：获取规则阶段 — 知识资产接入与人工确认闭环

> 状态：草案（2026-07-16 已完成代码库核对，EvidenceSpan 字段 / PendingApproval 类型 / RuleAssertion 是否为 hook / 规则引擎装配现状 / Stage ② 已存在等均已订正，见相关章节标注）
> 关联：五步工作法「发现事实 → **获取规则** → 规划 → 执行 → 检查」（已实现于 `domains/reasoning/five_step_runner.go`，Stage ② 即"获取规则"）
> 关联组件：`domains/rules`、`retrieval/`、`knowledge/graph/`、`domains/reasoning`、`domains/approval.go`（ApprovalGate）、`domains/approval_state.go`（ApprovalRecordState）、`agentcore/evidence`（EvidenceSpan）

## 一、问题背景

> 2026-07-16 核对订正：原审计"有机无料、规则引擎未装配"的判断不准确，实际情况见下方"现状校准"。

现状审计发现：`retrieval/` 与 `knowledge/graph/` 两个包接口完整，但 disclosure 管线的核心节点（三提取、新颖性初判）完全未接入，均为"Phase 2 TODO"。新颖性判断依赖 LLM 参数化知识或本地启发式打分，不基于任何外部知识库或知识图谱证据。

本设计聚焦"获取规则"阶段：如何让 wiki 知识库、语义库（retrieval）、知识图谱三类资产，转化为约束 Agent 后续规划/执行/检查行为的**规则**，并在进入 Plan 之前，以 Markdown 形式交由人工确认，确保规则适用的准确性。

### 现状校准（2026-07-16 代码 + 运行时数据核对）

原设计基于"知识资产未接入"的假设，经核对需大幅校准：

- **规则引擎早已完整装配**：`cmd/mady/main.go:555` `rules.LoadEngineFromMadyHome()` → `engine.go:31` 解析 `$MADY_HOME/knowledge/rules`（软链接到 xiaonuo 的 17 个 YAML，含 novelty.yaml 的 NOV-001~004 等）→ `tui_session.go:112/135/181` 注入三种 agent。`search_rules` / `get_article_framework` 工具启动即上线。
- **知识数据早已就位**：`~/.mady/knowledge/{knowledge.db 6.5G, patent_kg.db 207M, laws-full.db 152M, wiki/ 1573md}` 全部软链接到 xiaonuo，`knowledge/sqlite` 查询层已接入 chat agent。
- **"获取规则"阶段框架已存在**：`domains/reasoning/five_step_runner.go` 的 Stage ②（`runStage2`）就是规则获取，用 `MultiSourceRetriever` 做三路召回。**真正的缺口不是"新建规则召回"，而是补全 Stage ② 的两路 nil 参数**（`RuleVectorStore` / `RuleSkillReader`）——此项已于 2026-07-16 完成（见 `domains/reasoning/wiring/`）。
- **本设计剩余的真正增量**是：①审批阀前移到规则确认阶段（第五节）；②`ConfirmedRuleSet` 约束 Plan/Execute/Check（第六节）；③Wiki 双向沉淀（第 3.2 节）。

## 二、三类知识资产的权威性分层

不同来源产出的"规则"性质不同，必须在最终确认单中显式区分权威性，避免人工审阅时混为一谈：

| 来源 | 产出性质 | 权威性 | 典型内容 |
|---|---|---|---|
| `domains/rules` YAML 规则引擎（已装配） | **确定性规则** | 最高，代码固化 | 专利法条文映射（`articles/` 8 个法条框架）、新颖性判定规则（NOV-001~004，4 条）、OA 拒绝类型规则（8 类）。规则目录 `$MADY_HOME/knowledge/rules/` 软链接到 xiaonuo 语料，启动即加载 |
| 语义库（`retrieval/` + `knowledge/sqlite`） | **规范性依据** | 中，取决于检索命中质量 | 审查指南原文片段、法条具体条款、判例摘录 |
| 知识图谱（`knowledge/graph`） | **结构化事实约束** | 中高，取决于图谱完整度 | 引用链、同族专利、既往关联案件网络 |
| Wiki（专家复核意见沉淀） | **实务经验补充** | 最低，仅供参考 | 历史复核意见、边界情形的审查实践倾向 |

原则：Wiki 内容永远标注为"经验参考，非法律依据"，不得与确定性规则同等呈现。

## 三、流程设计

```
发现事实（FactBlackboard 已产出）
        │
        ▼
   规则召回（RuleRecall）── 三路并行查询
        │
        ├─ domains/rules 精确匹配        → 确定性规则命中
        ├─ 语义库检索（RRF + Reranker）  → 法条/指南原文片段候选
        └─ 知识图谱查询                  → 关联事实候选
        │                                  （QueryCitationChain / ReasoningWalker.CollectAll）
        ▼
   规则编译（RuleCompile）── 三路结果去重、按权威性分层、生成 Markdown 确认单
        │
        ▼
   人工确认（RuleApprovalGate）── 复用 PendingApproval 状态机，审批触发点前移至此
        │
        ├─ 确认   → ConfirmedRuleSet（不可变，绑定本案）
        ├─ 修改   → 人工编辑后的规则集
        └─ 驳回   → 回到规则召回，或标记「无适用规则，转人工全程处理」
        │
        ▼
   规划（Plan）→ 执行（Execute）→ 检查（Check）— 均消费 ConfirmedRuleSet
```

### 3.1 规则召回：三路各查什么

- **确定性规则匹配**：用"发现事实"阶段抽出的问题类型（新颖性判断 / 创造性判断 / 权利要求解释 / OA 答复……）精确查询 `domains/rules`，复用现有实现。
- **语义库检索**：查询锚点是**法律问题本身**（如"权利要求 X 是否因现有技术 Y 而不具备新颖性"），检索目标为审查指南/法条全文库。返回结果必须带 `EvidenceSpan`（文档 ID、条款位置、原文），不得只给结论性摘要——人工确认的核心工作正是核对原文有无断章取义。
- **知识图谱查询**：价值不在"查更多文档"，而在"查关系"。查询本案技术特征关联到哪些既往已判案件（同族/引用链），以及这些案件中规则是如何被适用的，作为"规则适用先例"附在确认单中，辅助人工判断规则套用本案是否合适。

### 3.2 Wiki 的双向角色

Wiki 不是被动查询对象，需双向设计：

- **查询时**：作为补充说明附在对应规则条目下（"历史类似案件中，专家复核认为……"），明确标注非法律依据。
- **确认后回写**：本次人工确认/修改的规则集连同修改理由，结构化写回 wiki（或 `user.db`）。下次遇到相似问题类型时，规则召回阶段可优先检索"历史已确认规则组合"作为默认建议呈现。

**约束**：历史确认过的规则组合，再次出现时**仍需重新走人工确认**，不得因历史确认过而自动跳过。否则会变相绕过"重点节点必须人机协作"的原则，导致规则在无人真正复核的情况下蔓延到不适用的案件。

## 四、Markdown 确认单结构（示例）

```markdown
# 规则适用确认单 · 案件 [CASE-ID]

**问题类型**：新颖性判断
**涉及权利要求**：权利要求 1、3

## ① 确定性规则（法条引擎命中，权威性：高）
- [ ] 专利法第22条第2款 — 新颖性判定标准
  依据：domains/rules 规则引擎精确匹配
- [ ] 审查指南第二部分第三章 4.2 — 单独对比原则

## ② 检索依据（语义库命中，权威性：中，请核对原文）
- [ ] 现有技术文献 CN110xxxxx，claim 1 原文片段：
  > "……"（EvidenceSpan: doc_id / 页码）
  相似度：0.82
- [ ] 审查指南片段（doc_id: guide-p2-c3，第 4.2.3 条）：
  > "……"

## ③ 图谱关联事实（权威性：中高）
- [ ] 同族专利：CN110xxxxx 与本案申请人历史专利 CN109xxxxx 存在引用关系
- [ ] 相似案例先例：案件 [CASE-0091]，同类特征组合，历史确认规则为「①②均适用，未适用③」

## ④ 经验参考（Wiki，权威性：仅供参考，非法律依据）
> 历史复核意见（案件 CASE-0077）："此类电路拓扑特征在审查实践中，审查员倾向于……"

---
**人工确认**：☐ 全部通过　☐ 部分修改（见下）　☐ 驳回，转人工处理
**修改/驳回理由**：___________
**确认人**：___________　**确认时间**：___________
```

~~该结构直接复用现有 `EvidenceSpan`（②③天然复用此类型）与 `PendingApproval`（底部确认区即现有状态机的呈现），无需新造数据结构~~ **此判断不准确**（2026-07-16 核对）：

- **`EvidenceSpan` 字段与确认单假设不符**：实际字段是 `ID, TurnID, ReceiptID, DocVersion, PageRange(string), CharRange, ContentHash, Snippet, SourceURI, RetrievalAt, Direction, ClaimRefs`（`agentcore/evidence/span.go:12`）。**没有 `DocID` / `Page int` / `Similarity` 字段**——确认单②③要呈现的"doc_id/页码/相似度"需用 `SourceURI` + `PageRange` + 自定义字段映射，不能直接复用。
- **`PendingApproval` 不是类型**：它是状态枚举值 `StatePendingApproval`（`domains/approval_state.go:31`），不是状态机类。可复用的状态机包装是 `ApprovalRecordState`（`approval_state.go:102`）。
- **审批阀前移并非"无需新造"**：`ApprovalGate`（`domains/approval.go:88`）是 chat agent LLM 循环的 `AfterModelCall` lifecycle hook，**不能直接挂在 Pregel 节点上**（五步工作法是作为 agent 工具执行的 Pregel 子图，节点内部不触发 AfterModelCall）。前移需要新写 Pregel 节点 → `agentcore.NewInterruptErrorWithData` 的适配层。

因此底部确认区需新增"编译为 Markdown 呈现给人"的环节、一个更早的审批触发点、**以及一个 Pregel→InterruptError 适配层**，并非"无需新造数据结构"。

## 五、规则如何约束 Plan / Execute / Check

仅完成人工确认不够，`ConfirmedRuleSet` 必须成为后续三阶段的硬约束：

- **Plan**：`ConfirmedRuleSet` 作为规划 Agent 的输入约束，结构化为 `constraint_type`（禁止性 / 强制性 / 程序性）。规划输出若违反强制性规则（如漏掉"单独对比"），应在规划校验阶段直接打回重新规划，而非等执行完才发现。
- **Execute**：~~复用现有 `RuleAssertion` 断言校验器，作为 lifecycle hook（`AfterModelCall`）挂载~~ **`RuleAssertion` 不是 lifecycle hook**（2026-07-16 核对）：它是 `domains/reasoning/syllogism.go:65` 的普通函数 `func RuleAssertion(bb *FactBlackboard, s *Syllogism) error`，全仓库从未注册为 lifecycle hook（现有 hook 仅 ApprovalGate / guardrails / memory / retrieval / knowledge）。要实现 Execute 阶段的规则硬约束，需**新写一个 `AfterModelCall` 适配层**调用 `RuleAssertion`，或直接在 Pregel 执行节点内调用它，不能"复用现有 hook"。
- **Check**：`ConfirmedRuleSet` 转化为检查清单，报告中新增"规则符合性"小节——哪些确认过的规则被实际引用、哪些未被引用但存在（需说明不适用原因）。可扩展现有 `CitationCompleteness` 评估指标，新增 `RuleComplianceCompleteness`，用于监控"规则确认了但后续未被真正使用"这一新脱节点。

## 六、落地顺序建议

> 2026-07-16 核对：原"规则召回三路查询打通"列为第一步**已完成**（Stage ② 的 `MultiSourceRetriever` 三路接入，见 `domains/reasoning/wiring/`），下述顺序据此校准。

1. ~~**规则召回三路查询打通**~~ **✅ 已完成**（2026-07-16）：`MultiSourceRetriever` 的 KG / Vector / Skill 三路已接入真实数据（knowledge.db FTS + patent-cards 130 张 + 知识图谱），召回结果写入 `FactBlackboard.RuleConstraint`。召回质量验证可在本地 `mady tui` 实跑五步工作法观察。
2. **Markdown 编译 + 审批阀接入**：复用 `ApprovalRecordState` + 新写 Pregel→`agentcore.NewInterruptErrorWithData` 适配层，触发点前移至规则确认阶段。**注意不能直接复用 `ApprovalGate`**（它是 chat agent 的 lifecycle hook，挂不到 Pregel 节点上）。
3. **Plan/Execute/Check 消费 `ConfirmedRuleSet`**：此步完成后，规则才从"展示给人看"变为"真正管住 Agent 行为"。**不建议提前于第 2 步**——若规则未经人工确认质量验证即用于硬约束，一旦召回质量差，Agent 会被错误规则捆死。Execute 阶段需新写适配层调用 `RuleAssertion`（非复用现有 hook）。
4. **Wiki 双向沉淀 + 历史规则组合推荐**：最后实施，依赖前三步积累足够的"确认历史"样本。Wiki 当前已有 importer（`knowledge/loader/wiki.go`）但只索引 `Wiki/` 和 `cards/`，**未覆盖 `patent-cards/`**——双向沉淀需扩展索引范围或新增回写路径。

## 七、开放问题（2026-07-16 已核对）

- ~~`ConfirmedRuleSet` 与现有 `EvidenceSpan`、`PendingApproval` 的具体数据结构如何复用/扩展，需结合 `agentcore/evidence` 实际定义确定。~~ **已明确**：`EvidenceSpan` 字段见第四节订正（无 DocID/Page/Similarity）；`PendingApproval` 是枚举值，状态机用 `ApprovalRecordState`。三者均不在 `agentcore/evidence`，审批相关在 `domains` 包。
- ~~`RuleAssertion` 现有实现是否已支持作为 lifecycle hook 挂载，还是需要新增适配层。~~ **已明确**：需新增适配层（`RuleAssertion` 是普通函数，非 hook，见第五节订正）。
- `RuleComplianceCompleteness` 指标的具体计分逻辑，建议参考 `evaluate/metrics.go:147` 中 `CitationCompleteness` 的实现模式（已核实该指标存在，`RuleComplianceCompleteness` 与 `EvidenceGroundedness` 均未存在，待新建）。
