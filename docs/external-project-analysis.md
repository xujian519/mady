# Mady 专利智能体与外部开源项目：代码级对比与嵌入分析

> 生成日期：2026-07-22
> 版本：v1.0
> 基于对 Mady 代码库（`/Users/xujian/projects/Mady`）及 10 个外部项目的深度调研

---

## 1. 分析框架：Mady 的三个技术理解子任务

```
                      ┌─────────────────────────┐
                      │  Mady 专利技术理解能力    │
                      ├─────────────────────────┤
 T1: 技术交底书理解   │  disclosure/ 管线        │
     PFE 三元组提取   │  extract.go              │
                      │  consistency.go          │
                      ├─────────────────────────┤
 T2: 对比文件理解     │  disclosure/novelty.go   │
     新颖性/创造性    │  domains/reasoning/      │
     特征映射         │  multi_hypothesis.go     │
                      ├─────────────────────────┤
 T3: 专利全文合规性   │  domains/enablement/     │
     A26.3/A26.4      │  nodes.go + framework.go │
                      │  domains/rules/          │
                      └─────────────────────────┘
```

---

## 2. Mady 现有架构全景

### 2.1 核心模块总览

| 模块 | 路径 | 核心能力 |
|------|------|---------|
| **技术交底书提取** | `disclosure/` | 9 段章节切分 → 三个并行 Agent（Problem/Feature/Effect）→ PFE 三元组合并 → 一致性校验（含 2 轮回退） |
| **新颖性初判** | `disclosure/novelty.go` | LLM + 证据注入的逐特征新颖性评估，JSON Schema 输出，含回退机制 |
| **A26.3 充分公开** | `domains/enablement/` | 三步法（完整性→清楚性→可实施性），YAML 定义规则框架，LLM Agent 逐节点执行 |
| **推理引擎** | `domains/reasoning/` | FactBlackboard + 三段论引擎 + 正反方辩论 + MultiHypothesis + TopologyExtractor |
| **规则引擎** | `domains/rules/` | 确定性 YAML 规则 + OA 解析器 + Slop 检测 |
| **知识检索** | `retrieval/` + `knowledge/` | SQLite FTS5 + 向量余弦 + RRF 融合 + 知识图谱增强 + 自动注入 |
| **质量护栏** | `guardrails/` | Citation Gate + 证据校验 + Guideline Source 检查 |
| **工作流编排** | `workflows/` | 五步推理 Runner（①发现事实→②发现规则→③规划→④执行→⑤检查） |

### 2.2 YAML 规则定义示例

`domains/rules/data/articles/patent-law-a26.3.yaml` 定义了专利法第 26 条第 3 款的完整判断框架：

```yaml
steps:
  - order: 1
    name: "检查说明书结构完整性"
    outputSchema:
      hasTechField: "bool"
      hasEmbodiment: "bool"
      missingSections: "[]string"
  - order: 2
    name: "检查说明书清楚性"
    outputSchema:
      isClear: "bool"
      orphanFeatures: "[]string"
      orphanEffects: "[]string"
  - order: 3
    name: "检查能够实现性（核心标准）"
    outputSchema:
      canImplement: "bool"
      missingKeyMeans: "bool"
      onlyTaskNoMeans: "bool"
```

这意味着 Mady 的规则框架**不是纯 prompt 驱动的**，而是 **YAML 定义结构 + LLM 填充具体内容**的混合模式——这是它与纯 LLM 方案的本质区别。

---

## 3. 外部项目全景对比速查表

| # | 项目 | GitHub | 星数 | 核心贡献 | 对 Mady 的补充价值 |
|---|------|--------|------|---------|-------------------|
| 1 | **Proof of Time** | — | — | 科学想法判断的未来可验证基准 | **元评估框架**：验证 Mady 判断的准确率 |
| 2 | **TruthHypo + KnowHD** | `gzxiong/TruthHypo` | 11 | 假说真实性 + 幻觉检测 | **后提取过滤**：减少 LLC 幻觉蔓延 |
| 3 | **SciVer** | — | — | 多模态科学声明验证基准 | **证据驱动对比**：特征级声明验证管道 |
| 4 | **BioDSA-1K** | — | — | 四轴假说验证框架 | Mady 已超越（PFE 因果链更精确） |
| 5 | **BioVerge** | — | — | 假说生成 + 自评估 | Mady 已超越（三段论校验更严格） |
| 6 | **AI-Researcher** | `HKUDS/AI-Researcher` | 5,611 | 端到端科研自动化 | Mady 架构更适配专利（无需代码执行） |
| 7 | **InternAgent** | `InternScience/InternAgent` | 1,379 | 假说→验证闭环，12 类任务 | **方案结构化模板**可参考 |
| 8 | **RD-Agent** | `microsoft/RD-Agent` | 13,982 | RD-Loop 研发自动化 | **验证闭环范式**可参考 |
| 9 | **Roundtable Policy** | — | — | 多 LLM + 置信度加权 | **仲裁增强**：争议性判断的可靠度提升 |
| 10 | **Karpathy autoresearch** | `karpathy/autoresearch` | 91,771 | 极简自主实验 | 与专利无关 |

---

## 4. Mady 已超越外部项目的领域

以下模块 Mady **已经做得更好**，不需要外部参考：

### 4.1 PFE 三元组提取（vs BioDSA-1K 四轴框架）

**Mady 优势**：
- 三个并行 Agent 提取 + **一致性校验** + **2 轮回退重试**→ 超越了 BioDSA-1K 的纯评估四轴框架
- BioDSA-1K 只评估"假说判定/证据一致性/推理正确性/代码可执行性"
- Mady 是**从零提取 + 因果关系链闭合验证**，粒度更细

**关键代码**：`disclosure/extract.go` → `buildExtractionPrompt` + `consistency.go` → `newConsistencyCheckNode`

### 4.2 A26.3 充分公开判断框架（vs TruthHypo/KnowHD）

**Mady 优势**：
- KnowHD 只做通用 groundedness 评分（"某个主张是否基于现有知识"）
- Mady 的 `domains/rules/data/articles/patent-law-a26.3.yaml` 定义了精确的三步法 + 四种公开不充分经典情形判断
- 结合 `domains/enablement/nodes.go` 的三个 LLM Agent 节点，是**专利定制的充分公开判断器**

### 4.3 正反方辩论+三段论校验（vs Roundtable Policy）

**Mady 优势**：
- Roundtable 只是多 LLM 投票 + 置信度加权
- Mady 的 `multi_hypothesis.go` 有 Advocate A/B + SyllogismJudge + EvidenceJudge + Retrieval back-edge
- 校验通过 `domains/reasoning/syllogism.go` 的 `RuleAssertion` 函数确保每条结论都引用黑板上的事实和法条

**关键代码**：`domains/reasoning/multi_hypothesis.go` → `Judge` 节点逻辑
`domains/reasoning/syllogism.go` → `RuleAssertion(bb, syllogism)` 校验

### 4.4 证据检索与注入管道（vs 纯 RAG）

- Mady 的 `retrieval/rerank.go` + `RFFuser` + `citation_gate.go` 构成了完整的**证据检索→过滤→注入→验证**闭环
- 大多数外部项目（包括 AI-Researcher、InternAgent）在证据层面远不如 Mady 精细

---

## 5. Mady 的 4 个真实缺口及嵌入方案

### 🔴 缺口 1：后提取阶段的 Groundedness 过滤（P1）

**目标模块**：`disclosure/extract.go` → `merge_extractions` 之后、`consistency_check` 之前

**问题**：三个提取 Agent 的产出是**纯 LLM 产物**，没有验证"提取出的内容是否真的在原文中有坚实依据"。当前 Consistency 阶段只检查 PFE 闭环，但没检查"这个 feature 是不是 LLM 脑补的"。

**外部参考**：**TruthHypo / KnowHD**（IJCAI 2025）

**嵌入方案**：

```go
// 在 disclosure/pipeline.go 中新增节点
func groundednessFilterNode(provider agentcore.Provider) graph.PregelNode {
    return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
        ext := state[StateKeyExtraction].(*ExtractionResult)

        // 对每个 feature 调用 KnowHD-style groundedness 评分
        for i, f := range ext.Features {
            score := evaluateGroundedness(f.Description, state[StateKeyDoc])
            if score < 0.6 {
                f.Confidence = score
                // 标记低置信度特征，触发回退重提取
            }
        }

        state[StateKeyExtraction] = ext
        return state, nil
    }
}
```

**改动量**：~80 行（新增 filter 节点 + groundedness 评分函数）
**收益**：减少 LLM 在提取阶段产生的幻觉内容混入后续分析

---

### 🟡 缺口 2：证据驱动的逐特征对比管道（P0 — 最高优先级）

**目标模块**：`disclosure/novelty.go` → `check_novelty` 节点

**问题**：当前 `noveltyNode` 让 LLM 一次看所有特征 + 所有证据片段 → 一次输出所有评估。这导致：
- LLM 说"特征被公开了"但引用的证据片段实际没提到该特征
- 无法精确追踪每个特征的证据来源

**外部参考**：**SciVer**（多模态科学声明验证基准）

**嵌入方案**：

```go
// 在 disclosure/novelty.go 中新增特征级验证管道
func featureLevelSciVerCheck(feature TechFeature, evidence []EvidenceChunk) FeatureVerdict {
    // 对每个特征独立走声明验证管道
    for _, ev := range evidence {
        // 使用 SciVer 的推理类型: 蕴含 / 矛盾 / 中立
        relation := classifyFeatureVsEvidence(feature.Description, ev.Snippet)
        switch relation {
        case "entailment":
            return FeatureVerdict{Status: "disclosed", EvidenceID: ev.DocID, Confidence: 0.9}
        case "contradiction":
            return FeatureVerdict{Status: "undisclosed", EvidenceID: ev.DocID, Confidence: 0.85}
        case "neutral":
            return FeatureVerdict{Status: "unclear", Confidence: 0.3}
        }
    }
    return FeatureVerdict{Status: "no_evidence", Confidence: 0.1}
}
```

**改动量**：~200 行（新增特征级管道 + 关系分类逻辑）
**收益**：解决"引用的证据不支持结论"这个最头疼的硬伤

**注意**：`featureAssessment` 结构体已预留 `CitedEvidenceIDs []string` 字段，可直接对接。

---

### 🟢 缺口 3：多 LLM 仲裁用于争议性判断（P2）

**目标模块**：`domains/reasoning/multi_hypothesis.go` → Judge 节点

**问题**：当前 Judge 节点使用**单一 LLM** 做裁决。对于"区别特征是否显而易见""对比文件是否给出结合启示"等争议性判断，单模型可靠性有限。

**外部参考**：**Roundtable Policy**（MIT/UCLA）

**嵌入方案**：

```go
// domains/reasoning/multi_hypothesis.go — JudgeConfidenceWeightTable
type JudgeConfig struct {
    ModelWeights map[string]float64 // model_id → 历史准确率权重
    MinAgreement float64            // 最低一致率 (0.5-1.0)
}

func (j *SyllogismJudge) WeightedJudge(
    args []Argument,
    facts FactBlackboard,
    rules ConfirmedRuleSet,
) Verdict {
    // 每个 Advocate 的输出经多个 LLM 独立评估
    scores := make(map[string][]float64)
    for _, arg := range args {
        for modelID, weight := range j.ModelWeights {
            score := evaluateArgument(arg, modelID)
            scores[arg.HypothesisID] = append(scores[arg.HypothesisID], score*weight)
        }
    }
    // 加权求和后裁决
    return calculateWeightedVerdict(scores, j.MinAgreement)
}
```

**改动量**：~150 行（新增 ConfidenceTable + WeightedJudge 逻辑）
**收益**：争议性判断（A22.3 结合启示、等同特征认定）的可靠性显著提升

---

### 🔵 缺口 4：元评估框架 — 校准 Mady 的判断准确率（P3）

**目标模块**：`benchmark/` + `evaluate/`

**问题**：没有系统性的方法来量化 "Mady 的判断到底有多准" → 不知道哪里 over-confident、哪里 under-confident

**外部参考**：**Proof of Time**（哈佛医学院）

**嵌入方案**：

```text
benchmark/
├── pot_eval/              # Proof-of-Time 风格评估
│   ├── cases/             # 历史 OA 案例（ground truth = 审查员最终决定）
│   ├── time_split.go      # 时间分割：只给 Mady 看到某个时间点前的现有技术
│   ├── calibrate.go       # 校准曲线：Mady 的 confidence vs 实际准确率
│   └── report.go          # 输出校准报告
```

```go
// benchmark/pot_eval/calibrate.go
type CalibrationPoint struct {
    PredictedConfidence float64 // Mady 输出的置信度
    IsCorrect           bool    // 是否与 ground truth 一致
}

func BuildCalibrationCurve(results []CalibrationPoint) {
    // 分桶统计：confidence 0.0-0.1, 0.1-0.2, ...
    for _, r := range results {
        bucket := int(r.PredictedConfidence * 10)
        total[bucket]++
        if r.IsCorrect {
            correct[bucket]++
        }
    }
    // 输出校准图：期望置信度 vs 实际准确率
    // 如果曲线偏离对角线，说明 Mady 存在系统性 over/under-confidence
}
```

**改动量**：~300 行（构建评估套件）
**收益**：知道系统真实能力上限，针对性调整 prompt 和阈值

---

## 6. 最终代码级嵌入优先级

```
优先级矩阵（影响 × 易实施性）：

  高 │
  影 │  ❖ 缺口2 (P0)
  响 │  证据特征对比
  力 │
     │  ❖ 缺口1 (P1)     ❖ 缺口3 (P2)
     │  Groundedness      多LLM仲裁
  低 │                     ❖ 缺口4 (P3)
     │                     元评估
     └───────────────────────────
       易             难
          实施难度
```

| 优先级 | 缺口 | 代码位置 | 预计改动量 | 预期效果 |
|--------|------|---------|-----------|---------|
| **P0** | 缺口 2：证据驱动的逐特征对比 | `disclosure/novelty.go` | ~200 行 | 解决"引用的证据不支持结论"这个用户最可见的硬伤 |
| **P1** | 缺口 1：Groundedness 过滤 | `disclosure/pipeline.go` | ~80 行 | 减少 LLM 幻觉从提取阶段蔓延到下游 |
| **P2** | 缺口 3：多 LLM 仲裁 | `domains/reasoning/multi_hypothesis.go` | ~150 行 | 争议性判断更可靠 |
| **P3** | 缺口 4：元评估框架 | `benchmark/pot_eval/` | ~300 行 | 量化知道系统真实能力上限 |

---

## 7. 不变的建议：哪些项目不需要关注

| 项目 | 跳过理由 |
|------|---------|
| **AI-Researcher** (★5,611) | 端到端科研自动化，侧重代码执行+实验验证。Mady 做的是法律判断而非实验执行，架构不匹配 |
| **InternAgent** (★1,379) | 12 类科学任务的方案理解→执行，泛化但不够专。Mady 的 PFE 提取 + 三段论在专利领域更深 |
| **RD-Agent** (★13,982) | 工业 R&D 自动化，量化金融场景。RD-Loop 范式可参考但代码本身无法直接复用 |
| **Karpathy autoresearch** (★91,771) | 极简 nanochat 训练。纯属不同赛道 |
| **BioVerge** | 自评估机制有参考价值但 Mady 已有 syllogism 校验和 citation gate，更严格 |

---

## 8. 总结

> **Mady 的架构骨架已经超越了上述所有外部项目的专利理解能力**——PFE 因果链提取 + YAML 规则驱动的三步法判断 + FactBlackboard 三段论推理 + 证据检索注入管道的组合，在专利领域是独一份的。
>
> 外部项目填补的是 **"置信度校准"** 和 **"幻觉过滤"** 这两个硬缺口。尤其是 **SciVer 风格的特征级证据匹配**（P0）和 **PoT 风格的元评估**（P3），是 Mady 目前最缺的两块拼图。
>
> 建议的实施顺序：**缺口 2（特征级对比）→ 缺口 1（groundness 过滤）→ 缺口 3（多 LLM 仲裁）→ 缺口 4（元评估）**。
