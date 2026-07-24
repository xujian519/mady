# 专利侵权判定模块设计规范

> 状态: 设计中 | 日期: 2026-07-23 | 决策来源: Wiki 知识库 + 项目代码深度分析

## 一、背景与动机

### 1.1 问题

项目中已有 `workflows/patent/infringement.go` 提供了基础的 6 节点 Pregel 侵权分析图，但存在以下局限：

- **特征匹配过于朴素**：采用子字符串匹配（5字以上重叠），缺乏 LLM 语义理解
- **无视角分化**：仅提供中立的侵权分析，未区分原告主张和被告抗辩两种截然不同的法律思维路径
- **范围不完整**：仅覆盖全面覆盖 + 等同，缺少抗辩体系、救济评估、策略建议
- **未与领域架构对齐**：workflows/ 是轻量工具层，未采用 domains/ 的 Agent 模块模式（Extension + ToolProvider + YAML 规则 + KnowledgeRetriever）

### 1.2 知识来源

基于尹新天《中国专利法详解》第7章和崔国斌《专利法：原理与案例（第二版）》第10/13章，涵盖：
- 侵权判定四步法（生产经营目的 → 权利要求解释 → 全面覆盖/等同对比 → 抗辩审查）
- 四大判定原则（全面覆盖、等同、禁止反悔、捐献）
- 六大抗辩事由（现有技术、先用权、合法来源、权利用尽、权利冲突、专利权滥用）
- 救济体系（临时禁令五要素、损害赔偿四层递进、惩罚性赔偿）
- 诉讼策略（管辖、举证、陷阱取证、诉讼中止、无效宣告联动）

### 1.3 目标

构建 `domains/infringement/` 领域模块，**完全替代** `workflows/patent/infringement.go` 的旧实现，提供从原告和被告两个视角出发的、覆盖 L1-L4 全层次的专利侵权判定引擎。

## 二、架构决策记录

| 决策 | 选项 | 理由 |
|------|------|------|
| 与旧代码关系 | 完全替代 | 统一到 domains/ 架构，避免维护两套实现 |
| 双视角实现 | 统一图 + Perspective 参数 | 代码量最小，与 inventiveness 模式对齐 |
| 范围深度 | L1+L2+L3+L4 全做 | 用户明确要求，一次构建完整闭环 |
| 专利类型 | Phase 1 仅发明/新型 | 外观设计判定逻辑本质不同，独立迭代 |

## 三、模块结构

```
domains/infringement/
  doc.go                 — 包文档
  types.go               — 核心类型定义
  framework.go           — 法条判断框架（ArticleFrameworkProvider 接口）
  graph.go               — Pregel 子图构建（10节点拓扑 + StateSchema）
  nodes.go               — LLM Agent 节点（提示词 + JSON Schema + parse 函数）
  tool.go                — agentcore.Tool 包装
  scorer.go              — 侵权可能性多维度评分器
  rules.go               — 规则引擎（Rule 接口 + Engine + 确定性规则检查）
  knowledge.go           — KnowledgeRetriever 接口 + 审查指南/类案检索增强
  infringement_test.go   — 单元测试 + 端到端测试 + 典型案例
```

### 3.1 对标关系

| 文件 | 对标模块 | 说明 |
|------|---------|------|
| types.go | inventiveness/types.go | 纯数据类型，零依赖 |
| framework.go | inventiveness/framework.go | YAML 法条框架 + 硬编码降级 |
| graph.go | inventiveness/graph.go | Pregel 图编译 + StateSchema |
| nodes.go | inventiveness/nodes.go | LLM Agent 工厂 + 每节点提示词 + JSON Schema |
| tool.go | inventiveness/tool.go | `evaluate_infringement` 工具注册 |
| scorer.go | claimdrafting/scorer.go | 多维度加权评分 |
| rules.go | claimdrafting/rules.go | Rule 接口 + 确定性规则引擎 |
| knowledge.go | enablement/types.go | KnowledgeRetriever 接口 |

## 四、核心类型设计

### 4.1 Perspective（视角枚举）

```go
type Perspective string

const (
    PerspectivePatentee  Perspective = "patentee"   // 原告/专利权人
    PerspectiveDefendant Perspective = "defendant"  // 被告/被控侵权人
)
```

### 4.2 InfringementInput

```go
type InfringementInput struct {
    PatentClaims    string      `json:"patent_claims"`     // 权利要求文本
    PatentSpec      string      `json:"patent_spec"`       // 说明书（可选，用于解释权利要求）
    ProsecutionHistory string  `json:"prosecution_history"` // 审查历史（可选，用于禁止反悔）
    AccusedProduct  string      `json:"accused_product"`   // 被控侵权产品/方法描述
    Perspective     Perspective `json:"perspective"`        // 分析视角
    PatentType      PatentType  `json:"patent_type"`        // 发明/实用新型
    PriorArtRefs    []string    `json:"prior_art_refs"`     // 已知现有技术引用（可选）
    LicenseInfo     *LicenseInfo `json:"license_info"`      // 许可信息（可选）
}

type LicenseInfo struct {
    HasLicense      bool    `json:"has_license"`
    LicenseType     string  `json:"license_type"`     // exclusive/non-exclusive/compulsory
    LicenseScope    string  `json:"license_scope"`
    RoyaltyRate     float64 `json:"royalty_rate"`
}
```

### 4.3 InfringementOutput

```go
type InfringementOutput struct {
    Verdict           InfringementVerdict  `json:"verdict"`
    ClaimScope        ClaimScopeResult     `json:"claim_scope"`
    FeatureMapping    []FeatureComparison  `json:"feature_mapping"`
    LiteralResult     LiteralResult        `json:"literal_result"`
    EquivalenceResult EquivalenceResult    `json:"equivalence_result"`
    DefenseAnalysis   []DefenseAssessment  `json:"defense_analysis"`
    RemedyAssessment  RemedyResult         `json:"remedy_assessment"`
    StrategyAdvice    StrategyResult       `json:"strategy_advice"`
    Confidence        float64              `json:"confidence"`       // 0-1
    Disclaimer        string               `json:"disclaimer"`
    CitationRefs      []CitationRef         `json:"citation_refs"`
}

type InfringementVerdict struct {
    Conclusion      string   `json:"conclusion"`       // infringed / not_infringed / uncertain
    Likelihood      float64  `json:"likelihood"`        // 0-1 侵权可能性
    Basis           []string `json:"basis"`             // literal / equivalence
    KeyFindings     []string `json:"key_findings"`
    RiskLevel       string   `json:"risk_level"`        // high / medium / low
}
```

### 4.4 各步骤结果类型

```go
type ClaimScopeResult struct {
    InterpretedScope    string   `json:"interpreted_scope"`
    KeyTerms            []TermDefinition `json:"key_terms"`
    DisclaimersIdentified []string `json:"disclaimers_identified"`
}

type FeatureComparison struct {
    ClaimFeature    string `json:"claim_feature"`
    ProductFeature  string `json:"product_feature"`
    MatchType       string `json:"match_type"` // literal / equivalent / missing
    MatchReasoning  string `json:"match_reasoning"`
}

type LiteralResult struct {
    AllElementsMet   bool   `json:"all_elements_met"`
    MissingFeatures  []string `json:"missing_features"`
    ExtraFeatures    []string `json:"extra_features"`
}

type EquivalenceResult struct {
    EquivalentFeatures  []EquivalenceAssessment `json:"equivalent_features"`
    EstoppelApplied     bool   `json:"estoppel_applied"`
    EstoppelDetails     string `json:"estoppel_details"`
    DedicationApplied   bool   `json:"dedication_applied"`
    DedicationDetails   string `json:"dedication_details"`
}

type EquivalenceAssessment struct {
    ClaimFeature    string `json:"claim_feature"`
    ProductFeature  string `json:"product_feature"`
    SameMeans       bool   `json:"same_means"`
    SameFunction    bool   `json:"same_function"`
    SameEffect      bool   `json:"same_effect"`
    NonObviousness  bool   `json:"non_obviousness"`
    IsEquivalent    bool   `json:"is_equivalent"`
    Reasoning       string `json:"reasoning"`
}
```

### 4.5 抗辩相关类型

```go
type DefenseAssessment struct {
    DefenseType     string  `json:"defense_type"`   // prior_art/prior_use/legal_source/exhaustion/rights_conflict/abuse
    Applicable      bool    `json:"applicable"`
    ViabilityRating string  `json:"viability_rating"` // high/medium/low/none
    Analysis        string  `json:"analysis"`
    EvidenceNeeded  []string `json:"evidence_needed"`
    LegalBasis      string  `json:"legal_basis"`     // 法条引用
}

type PriorArtDefense struct {
    PriorArtRef     string              `json:"prior_art_ref"`
    ComparisonResult FeatureComparison  `json:"comparison_result"`
    IsIdentical     bool                `json:"is_identical"`
    IsEquivalent    bool                `json:"is_equivalent"`
    DefenseStrength string              `json:"defense_strength"`
}
```

### 4.6 救济相关类型

```go
type RemedyResult struct {
    DamageEstimate    DamageEstimate    `json:"damage_estimate"`
    InjunctionAnalysis InjunctionAnalysis `json:"injunction_analysis"`
    PunitiveRisk      *PunitiveRisk     `json:"punitive_risk,omitempty"`
}

type DamageEstimate struct {
    Method          string  `json:"method"`          // actual_loss/infringer_profit/license_fee/statutory
    EstimatedAmount float64 `json:"estimated_amount"`
    RangeLow        float64 `json:"range_low"`
    RangeHigh       float64 `json:"range_high"`
    CalculationBasis string `json:"calculation_basis"`
}

type InjunctionAnalysis struct {
    PreliminaryInjunction *InjunctionFactors `json:"preliminary_injunction"`
    PermanentInjunction   *InjunctionFactors `json:"permanent_injunction"`
}

type InjunctionFactors struct {
    LikelihoodOfSuccess  string  `json:"likelihood_of_success"`
    IrreparableHarm      string  `json:"irreparable_harm"`
    BalanceOfHardships   string  `json:"balance_of_hardships"`
    PublicInterest       string  `json:"public_interest"`
    BondRequired         float64 `json:"bond_required"`
    Feasibility          string  `json:"feasibility"`
}
```

### 4.7 策略相关类型

```go
type StrategyResult struct {
    RecommendedActions  []StrategyAction `json:"recommended_actions"`
    JurisdictionAnalysis *JurisdictionAdvice `json:"jurisdiction_analysis,omitempty"`
    Timeline            []TimelineMilestone `json:"timeline"`
    SettlementAssessment *SettlementAdvice `json:"settlement_assessment,omitempty"`
    InvalidationRoute   *InvalidationStrategy `json:"invalidation_route,omitempty"`
}

type StrategyAction struct {
    Action      string `json:"action"`
    Priority    string `json:"priority"`  // immediate/short_term/long_term
    Rationale   string `json:"rationale"`
    RiskLevel   string `json:"risk_level"`
}
```

## 五、Pregel 图拓扑

### 5.1 节点序列

```
load_input (守卫节点)
  → claim_scope (L1: 权利要求解释)
  → feature_decomposition (L1: 技术特征分解)
  → literal_infringement (L1: 全面覆盖比对)
  → equivalence (L1: 等同原则 + 禁止反悔 + 捐献规则)
  → infringement_verdict (L1: 综合判定)
  → defense_review (L2: 抗辩体系)
  → remedy_assessment (L3: 救济评估)
  → strategy (L4: 策略建议)
  → conclude (输出格式化)
```

### 5.2 条件路由

- `load_input` → 输入无效时直接跳到 `conclude`（error 输出）
- `literal_infringement` → 全部特征字面匹配时，可跳过 `equivalence`（可选优化，由 StateSchema 控制）
- `infringement_verdict` → 结论为 `not_infringed` 时，`defense_review` 仍执行但标记为"理论分析"
- 所有节点通过 `stateHasSkip()` 支持短路

### 5.3 State Keys

```go
const (
    StateInput              = "inf_input"
    StatePerspective        = "inf_perspective"
    StateClaimScope         = "inf_claim_scope"
    StateClaimFeatures      = "inf_claim_features"
    StateProductFeatures    = "inf_product_features"
    StateFeatureMapping     = "inf_feature_mapping"
    StateLiteralResult      = "inf_literal_result"
    StateEquivalenceResult  = "inf_equivalence_result"
    StateVerdict            = "inf_verdict"
    StateDefenseAnalysis    = "inf_defense_analysis"
    StateRemedyAssessment   = "inf_remedy_assessment"
    StateStrategy           = "inf_strategy"
    StateOutput             = "inf_output"
    StateSkipped            = "inf_skipped"
)
```

## 六、节点设计要点

### 6.1 视角感知提示词

每个节点的 System Prompt 分为三段：
1. **中立法律框架**（所有视角共享）：法条原文、判断标准、法律解释
2. **视角特定指令**（通过 `{{.Perspective}}` 注入）：原告论证路径 vs 被告抗辩路径
3. **JSON Schema 约束**：统一的结构化输出格式

### 6.2 Agent 工厂

```go
func newInfringementAgent(provider agentcore.Provider, name, prompt string, schema map[string]any) *agentcore.Agent {
    cfg := agentcore.Config{
        ModelConfig: agentcore.ModelConfig{
            Name: name, Model: "default", Provider: provider,
            Temperature: 0.1,  // 法律判定需要高度确定性
        },
        SystemPrompt: prompt,
        ExecutionConfig: agentcore.ExecutionConfig{MaxTurns: 1},
    }
    if schema != nil {
        cfg.ResponseFormat = agentcore.NewJSONSchemaResponseFormat(name, schema)
    }
    return agentcore.New(cfg)
}
```

### 6.3 每个节点的核心提示词要点

| 节点 | 原告视角核心指令 | 被告视角核心指令 |
|------|----------------|----------------|
| claim_scope | 以最宽合理解释确定保护范围 | 识别限缩性用语，主张窄化解释 |
| feature_decomposition | 从权利要求中提炼尽可能多的特征点 | 注重特征粒度，细化差异点 |
| literal_infringement | 论证每个特征如何被被控产品覆盖 | 寻找缺失/不同的技术特征 |
| equivalence | 主张手段/功能/效果基本相同 | 论证存在实质性差异，援引禁止反悔/捐献 |
| infringement_verdict | 综合评估侵权成立的论证强度 | 评估不侵权论证的可信度 |
| defense_review | 预判被告可能提出的抗辩及其弱点 | 构建多层抗辩策略，排序优先级 |
| remedy_assessment | 最大化赔偿模型，论证禁令必要性 | 最小化风险敞口，专利贡献率分割 |
| strategy | 管辖/证据/时机最优策略 | 无效宣告/诉讼中止/和解谈判路径 |

## 七、规则引擎集成

### 7.1 确定性规则（Go 代码实现）

```go
type InfringementRule interface {
    Name() string
    Description() string
    LegalBasis() string        // 法条引用，如 "A11", "A62", "司法解释第7条"
    Severity() RuleSeverity    // LevelBlock/LevelMust/LevelShould/LevelQuality
    Check(input *RuleCheckInput) (*RuleCheckResult, error)
}
```

规则分类：
- **L1 核心判定规则**（~8条）：全部技术特征检查、等同三要素检查、禁止反悔适用条件、捐献规则适用条件
- **L2 抗辩规则**（~6条）：现有技术单独对比、先用权时间条件、合法来源双重条件、权利用尽边界
- **L3 救济规则**（~6条）：损害赔偿计算顺位、临时禁令五要素完整性、惩罚性赔偿故意要件
- **L4 策略规则**（~4条）：管辖选择、诉讼时效、无效中止条件

### 7.2 YAML 规则文件

新增文件：
- `domains/rules/data/articles/patent-law-infringement.yaml` — 侵权判定的 ArticleFramework（含 A11/A59/A62/A65/A66/A69/A70）
- `domains/rules/data/rules/infringement-rules.yaml` — ~24条规则定义
- `domains/rules/data/orchestrations/infringement-analysis.yaml` — 事务编排

## 八、评分器设计

参考 `claimdrafting/scorer.go` 模式，多维度加权评分：

```go
type InfringementScorer struct {
    weights map[string]float64
}

// 默认权重
var DefaultWeights = map[string]float64{
    "literal_match":    0.25,  // 字面匹配完整度
    "equivalence":      0.20,  // 等同认定强度
    "estoppel_risk":    0.10,  // 禁止反悔风险（被告有利）
    "dedication_risk":  0.05,  // 捐献规则风险（被告有利）
    "defense_strength": 0.20,  // 抗辩可行性（被告有利）
    "remedy_exposure":  0.10,  // 救济风险敞口
    "strategy_viability": 0.10, // 策略可行性
}
```

## 九、知识检索增强

### 9.1 KnowledgeRetriever 接口

```go
type KnowledgeRetriever interface {
    SearchGuidelines(ctx context.Context, query string) ([]GuidelineRef, error)
    SearchSimilarCases(ctx context.Context, query string) ([]CaseRef, error)
    SearchLegalProvisions(ctx context.Context, articles []string) ([]LegalProvision, error)
}
```

### 9.2 增强流程

在 `tool.go` 的 Execute 函数中，跑图前：
1. 调用 `SearchGuidelines` 检索审查指南侵权判定章节
2. 调用 `SearchLegalProvisions` 检索 A11/A59/A62/A65/A66/A69/A70 全文
3. 可选调用 `SearchSimilarCases` 检索类案
4. 将检索结果注入 `InfringementInput.GuidelineRefs` 和 `SimilarCases`

## 十、集成变更清单

| 变更位置 | 变更内容 | 类型 |
|---------|---------|------|
| `domains/infringement/` | 新建全部文件 | 新增 |
| `domains/rules/data/articles/patent-law-infringement.yaml` | 侵权法条框架 | 新增 |
| `domains/rules/data/rules/infringement-rules.yaml` | 侵权规则定义 | 新增 |
| `domains/rules/data/orchestrations/infringement-analysis.yaml` | 侵权事务编排 | 新增 |
| `domains/router.go` | 注册 infringment 工具到 PatentAgent | 修改 |
| `guardrails/citation_table.go` | 新增 A11/A62/A65/A66/A69/A70 主题映射 | 修改 |
| `knowledge/store.go` | 扩充种子数据（侵权相关法条） | 修改 |
| `styles/patent-standard.yaml` | 新增侵权场景反模式词表 | 修改 |
| `workflows/patent/infringement.go` | 标记 deprecated | 修改 |
| `workflows/patent/infringement_test.go` | 标记 deprecated | 修改 |
| `doc-templates/legal/infringement-analysis.md` | 更新变量以匹配新 Output 结构 | 修改 |

## 十一、测试策略

### 11.1 单元测试

- 类型序列化/反序列化
- 规则引擎每条规则的 Check 逻辑
- 评分器权重计算
- 视角参数传递
- 知识检索接口 mock
- parse 函数正确性

### 11.2 端到端测试

- 完整图执行：字面侵权场景（全部特征匹配）
- 完整图执行：等同侵权场景（部分特征差异）
- 完整图执行：不侵权场景（关键特征缺失）
- 双视角对比：同一输入，PerspectivePatentee vs PerspectiveDefendant 输出差异
- 抗辩场景：现有技术抗辩、先用权抗辩
- 错误处理：空输入、无效 JSON、Provider 不可用

### 11.3 典型案例

基于真实司法判例构建测试用例（脱敏后）：
- 全面覆盖典型案例
- 等同原则典型案例（手段/功能/效果三要素争议）
- 禁止反悔典型案例（审查过程修改导致限缩）
- 现有技术抗辩典型案例

## 十二、风险与缓解

| 风险 | 缓解措施 |
|------|---------|
| 10节点 LLM 调用链过长，延迟高 | Temperature=0.1 + MaxTurns=1 约束；后续考虑节点合并 |
| 双视角提示词维护复杂 | 提取中立框架文本为共享常量，视角差异仅注入差异化段落 |
| JSON Schema 过于复杂导致解析失败 | 每个 Schema 严格控制字段数 ≤ 10；实现降级解析（宽松模式） |
| 外观设计后续接入需重构 | Perspective + PatentType 双参数从开始就预留扩展点 |
| 旧代码迁移期间功能回退 | 先完成新模块+测试，最后一步才标记 deprecated |

## 十三、后续迭代

- **Phase 2**: 外观设计专利侵权判定（独立图拓扑）
- **Phase 3**: 间接侵权/共同侵权判定
- **Phase 4**: 标准必要专利（SEP）FRAND 分析
- **Phase 5**: 反不正当竞争法第6条混淆判断
