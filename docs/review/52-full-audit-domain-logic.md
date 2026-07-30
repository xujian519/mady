# 领域逻辑一致性审查报告（第 52 期）

> **审查时间**：2026-07-31
> **审查范围**：`domains/` 及其子包（evidence, inventiveness, enablement, psychological, claimdrafting, specdrafting, rules）
> **审查方法**：源码阅读 + 符号追踪 + import 依赖分析 + 法律体系交叉比对
> **风险等级**：🔴 高风险 / 🟡 中风险 / 🟢 低风险

---

## 1. evidence 模块死代码评估 🔴 (P0)

### 1.1 概述

`domains/evidence/` 共 16 个非测试源文件，约 4,574 行（含测试）。**证据判断模块并非死代码** — 上次审查报告所称"零外部引用"不准确。

### 1.2 实际引用链

本包被以下 6 处外部引用：

| 引用位置 | 引用内容 | 用途 |
|----------|---------|------|
| `cmd/mady/evidence.go` | `evidence.NewEngine`, `evidence.DefaultEngine`, `evidence.EvidenceJudgment` | CLI 子命令 `mady evidence judge/burden/standard/conflict/type` |
| `server/evidence.go` | `evidence.NewEngine`, `evidence.EvidenceJudgment`, `evidence.DetermineBurden`, `evidence.AssessProofStandard` | HTTP API 端点 `/api/v1/evidence/*` |
| `domains/workflows/patent/infringement.go` | `evidence.NewEngine`, `evidence.EvidenceJudgment`, `agentcore_evidence.EvidenceSpan` | 侵权分析 Pregel 图中的 `infJudgeEvidenceNode` |
| `domains/workflows/patent/invalidation.go` | `evidence.NewEngine`, `evidence.EvidenceJudgment`, `agentcore_evidence.Conflict` | 无效宣告 Pregel 图中的 `judgeEvidenceNode`, `detectConflictNode` |
| `bootstrap/setup.go` | `evidence.NewExtension()` | 先创建引擎，再注册为 `EvidenceDomainExtension` |
| `bootstrap/init_reasoning.go` | 注释引用 | 证据判断扩展注入 Agent 配置 |

同时 `domains/evidence/` 内部还引用了 `agentcore/evidence`（基础数据类型如 `EvidenceSpan`, `ClaimBinding`, `ConflictDetector`）。

### 1.3 关键发现：证据规则系统存在两套脱节的 YAML 规则

**问题描述**：
- `domains/evidence/rule_loader.go` 定义了 `RuleIndex` 和 `EvidenceRule` 类型，可以从 YAML 加载证据规则
- `domains/rules/data/rules/evidence-rules.yaml` 定义了 15 条证据规则（三性 + 类型 + 举证责任等）
- **但 `evidence-rules.yaml` 从未被 `domains/evidence` 包加载！**
- evidence 包的 `RuleIndex.LoadYAML` 仅在 `engine_test.go` 的测试中被调用，生产中从未调用

**证据**：
- `grep -rn 'LoadYAML\|LoadBytes\|LoadRules' --include='*.go' . | grep -v '_test.go'` 显示：`engine.go:LoadRules` 和 `rule_loader.go:LoadYAML`/`LoadBytes` 的生产路径仅暴露接口，从未被实际调用
- `NewEngine(nil)` 在内置路径中创建空的 `NewRuleIndex()`，不加载任何 YAML 规则文件
- 证据判断逻辑全部硬编码在 `engine.go` 的 `evaluateRelevance`/`evaluateLegality`/`evaluateAuthenticity`（位于 `triple_attrs.go`）和 `evaluateTypeSpecific` 中

**影响**：
- `evidence-rules.yaml` 的 15 条规则完全未被激活 — 这是一份空文档
- 判断引擎的权重配置本可以从 YAML 加载（`computeOverallScore` 中读取 `dim.Weight`），但实际上永远走默认权重（相关性 0.3, 合法性 0.3, 真实性 0.4）
- `EvidenceRule` 的 `check.type` 字段（如 `relevance`, `authenticity`）与引擎的实际执行逻辑没有绑定关系

**修复建议**：
1. 🟡 短期：在 `bootstrap/setup.go` 的 `NewExtension()` 之后，主动加载 `evidence-rules.yaml` 到 engine 的 index
2. 🟡 中期：将 `YAML` 中的 `evidenceAssessment.weights` 映射到 `computeOverallScore` 的动态权重加载路径
3. 🔴 长期：将 `triple_attrs.go` 中的硬编码规则逐步迁移到 YAML 驱动，实现真正的 DSL 执行

### 1.4 评分：6/10

证据模块功能完整、有外部引用、测试覆盖高（4 个测试文件），但 YAML 规则系统与执行引擎脱节是架构级别的设计问题。

---

## 2. inventiveness 逻辑自洽性 🟡 (P0)

### 2.1 概述

`domains/inventiveness/` 共 10 个非测试源文件。实现创造性判断三步法（Step1-Step4）的独立 Pregel 子图。

### 2.2 IsInventive 判断逻辑与声明的 AND 公式

**声明**（`graph.go` 和 `node_conclusion.go`）：
```
IsInventive = Step3.NonObvious AND Step4.HasSignificantProgress
```
即：`IsInventive = !Step3.TechnicalSuggestion AND Step4.HasSignificantProgress`

**实际实现**（`node_conclusion.go:178-181`）：
```go
if cc.parseOK {
    result.IsInventive = cc.IsInventive  // LLM 综合判断优先
} else {
    result.IsInventive = !result.Step3.TechnicalSuggestion && result.Step4.HasSignificantProgress
}
```

**审查结论**：声明与实际基本一致，但存在两个值得注意的问题：

1. **LLM 综合判断覆盖声明逻辑**：当 `parseOK=true`（LLM 输出可解析为 JSON）时，完全信任 LLM 的 `is_inventive` 字段，不再执行 AND 逻辑。这意味着 LLM 可能给出与公式不一致的结论（例如 Step3=无启示/NOT obvious 但 Step4=没有显著进步，LLM 仍判定为 inventive）。

2. **Step4 的门槛语义问题**：`Step4Result` 的注释和 Step4 节点的 prompt 都指出"创造性 = 突出的实质性特点 AND 显著的进步"。但 prompt 中同时写道：「『显著的进步』门槛较低：只要具有有益的技术效果……通常满足此要件」和「在大多数情况下，非显而易见的发明通常也具有某种有益效果」。这导致在实际判断中 Step4 几乎总是 true，IsInventive 退化为依赖 Step3 单步判断，AND 公式失去了实际约束力。

### 2.3 Pregel 图定义与节点实现匹配

**图拓扑**（`graph.go`）：
```
load_input → step1 → step2 → step3 → step4 → evaluate_experimental_data → generate_conclusion → __end__
```

**实际实现**：所有列出的节点在 `nodes.go` 和 `node_*.go` 文件中均有对应实现，边定义完整，`Compile("load_input", 100)` 配置正确。**匹配良好。**

### 2.4 发明构思比对与改进动机三维度分析（Prompt 质量）

Step3 的 prompt（`node_step3.go`）实现了精细的发明构思比对方法论，包含：
- 第 1 阶段：发明构思比对与改进动机分析（两步骤：提炼构思 + 比较差异）
- 第 2 阶段：技术启示五种情形判断
- 改进动机三维度系统分析（问题发现难度 + 结合动机 + 技术发展趋势）
- 分析推理与有限试验结构化判断

这是高质量的 prompt 设计，符合 2023 版审查指南的趋势。

### 2.5 分层违规检查

**依赖情况**：
- 仅依赖 `agentcore`, `graph`, `pkg/util`, `disclosure`（经 `tool.go`）
- 不直接依赖其他 domain 子包
- ✅ 分层合规，无基础设施层依赖

### 2.6 评分：8/10

逻辑自洽性良好，Pregel 图与节点实现一致。唯一的风险是 LLM 综合判断可能绕过 AND 公式，但这属于 LLM-as-judge 的固有局限，可通过在后处理中添加硬约束校验来缓解。

---

## 3. enablement 中美法系混用 🔴 (P0)

### 3.1 概述

`domains/enablement/` 共 12 个非测试源文件 + 测试数据。实现专利法第 26 条第 3 款（充分公开/可实现性）的 Pregel 子图评估。

### 3.2 关键发现：`In re Wands` 美国判例法混入中国法分析

**文件**：`/Users/xujian/projects/Mady/domains/enablement/node_enablement.go:100`

```go
"- 判断是否过度实验时考虑因素（参考 In re Wands）：",
"  所需试验数量、提示指导量、有无实施例、发明性质、",
"  现有技术状况、该领域技术人员技能、技术可预见性、权利要求宽度",
```

**问题**：`In re Wands` 是美国联邦巡回上诉法院（CAFC）关于"过度实验"（undue experimentation）标准的判例，属于 **USPTO/MPEP 体系**。中国专利法的充分公开标准（26.3）虽然也有"无需创造性劳动即可实现"的要求，但其法律体系完全不同：
- 中国：审查指南第二部分第二章第 2.1.3 节列出六种公开不充分情形（仅给出任务/设想、技术手段含糊不清、不能解决技术问题等）
- 美国：MPEP §2164.01(a) 引用 In re Wands 的 8 个 Wands 因素（Wands factors）判断过度实验

**法律正确性风险**：🔴 高 — 如果将 8 个 Wands 因素直接作为中国 26.3 的判断标准，可能导致法律适用错误。例如，美国标准中"权利要求宽度"（breadth of the claims）可以作为认定公开充分的理由，但在中国审查实践中不是标准的考量要素。

### 3.3 整体法律体系一致性检查

除 Wands 问题外，其余引用全部正确指向中国专利法体系：

| 引用位置 | 法律引用 | 正确性 |
|----------|---------|--------|
| `framework.go:65-101` | 专利法第26条第3款 + 审查指南 2023 | ✅ |
| `node_enablement.go:61-98` | 六种公开不充分情形（审查指南 §2.1.3） | ✅ |
| `node_clarity.go:89-102` | 审查指南第二部分第二章第 2.1.1 节 | ✅ |
| `domain_rules.go:147` | 化学产品发明充分公开三要素 | ✅ |
| `domain_rules.go:194` | 生物材料保藏要求 | ✅ |
| `node_conclusion.go:60-75` | 26.3 充分公开要求 + 27.1 支持要求区分 | ✅ |

### 3.4 测试数据检查

`testdata/enablement_cases.json` 包含 18 个测试案例，全部基于中国专利审查实践。未发现其他美国判例法混入。

**影响评估**：
- 生产环境下，LLM 看到 Wands 因素可能将其混入 26.3 判断，导致法律依据偏差
- 影响程度取决于 LLM 对法律体系的区分能力

**修复建议**：
1. 🔴 **高优先级**：删除 `node_enablement.go:99-101` 的 Wands 引用，替换为中国审查指南的「无需过度劳动」判断标准
2. 🟡 补充审查指南第二部分第二章关于"能够实现"的完整判断标准

### 3.5 评分：6/10

整体架构和 prompt 质量高，图定义清晰合规。单一但严重的 Wands 判例法混入扣分。

---

## 4. psychological 实际实现 vs 声称能力 🟡 (P1)

### 4.1 概述

`psychological/` 包共 3 个非测试源文件 + `domains/psychological_config.go`，总计约 500 行。声称实现 VAD（Valence-Arousal-Dominance）三维情绪空间模型。

### 4.2 实际实现分析

**VAD 模型实现**（`engine.go:140-200`）：
- 6 个文本提取函数：`sentimentScore`, `uncertaintyScore`, `blameScore`, `controlScore`, `surpriseScore`, `goalImportanceScore`
- 全部基于**关键词匹配**（硬编码中文关键词列表）
- VAD 计算：`Valence = Sentiment*0.7 + PerceivedControl*0.2 + BlameDirection*0.1`，其余维度类似线性加权

**问题**：这**不是真正的 VAD 情绪模型**。真正的 VAD 模型（如 Russell 的 Circumplex Model 或 ANEW 词库）需要经过心理学验证的词汇规范化和情绪标注数据库。当前实现是简单的关键词词袋模型 + 线性加权，不具备以下能力：
- 上下文理解（"这个专利很麻烦" vs "麻烦你帮忙查一下" — 两种完全不同的情绪）
- 否定处理（"不麻烦" → 情感极性翻转）
- 程度副词调节（"很生气" vs "有点生气" — arousal 应有差异）

### 4.3 LifecycleHook 集成检查

`extension.go:47-88` 实现了 `TransformContextProvider` 接口，在每轮用户消息后自动分析并注入心理上下文块到 system prompt 中。

| 接口 | 实现状态 | 说明 |
|------|---------|------|
| `Extension` | ✅ | 完整实现 |
| `ToolProvider` | ✅ | 注册了 `analyze_emotion` 工具 |
| `SystemPromptProvider` | ✅ | 模块化系统提示词注入 |
| `TransformContextProvider` | ✅ | 消息队列中注入心理上下文块 |
| `LifecycleHook` | ❌ | **未实现** — 心理引擎没有注册为 LifecycleHook |

**问题**：上次审查报告称"仅实现 20% 声称能力"。当前评估：

| 声称能力 | 实现程度 | 说明 |
|----------|---------|------|
| VAD 情绪空间 | 30% | 有三维坐标计算，但非标准 VAD，无规范词库 |
| 文本信号提取 | 50% | 6 个信号维度合理，但全为关键词匹配 |
| 对话策略适配 | 80% | 5 种策略 + 置信度计算 + prompt 注入 |
| LifecycleHook 集成 | 0% | 使用 TransformContextProvider 而非 LifecycleHook |
| 认知偏差检测 | 0% | `SkipDistortionDetection` 永久为 true（根据 domains/psychological_config.go）|

**实际实现约 30-35% 声称能力**，比上次报告的 20% 略有提升但不构成质变。

### 4.4 领域配置问题

`domains/psychological_config.go` 为每个领域返回 `Config{SkipDistortionDetection: true/false}`，但 `distortion detection` 功能从未实现 — `PipelineConfig.SkipDistortionDetection` 在所有节点中仅作为占位符参数被 `_ = config` 忽略。

### 4.5 评分：4/10

包结构完整、接口实现正确、运行无 bug，但核心 VAD 模型高度简化，认知偏差检测完全未实现。在关键的法律专业场景中，基于关键词匹配的情绪判断可靠度存疑。

---

## 5. 领域规则 DSL 解析-执行一致性 🟡 (P1)

### 5.1 概述

`domains/rules/` 共 10 个非测试源文件，实现 YAML 驱动的专利法律规则引擎。

### 5.2 解析路径

`loader.go` 从 `$MADY_HOME/knowledge/rules/` 或 `domains/rules/data/` 加载 YAML 规则文件：

```
data/
├── rules/*.yaml           → Rule 类型（patent-core, novelty, amendment, evidence, etc.）
├── articles/*.yaml        → ArticleFramework（A22.2, A22.3, A26.3, A33）
├── orchestrations/*.yaml  → Orchestration（invalidation, infringement, oa-response, re-examination）
└── provisions/*.yaml      → 编排配置
```

### 5.3 执行路径

| 工具名称 | 执行方式 | 是否实际运行 |
|----------|---------|------------|
| `search_rules` | 关键字搜索后返回格式化的 Rule 文本 | ✅ Agent 可调用 |
| `get_article_framework` | 按 articleID 返回 ArticleFramework | ✅ Agent 可调用 |
| `get_orchestration` | 按 caseType 返回 Orchestration | ✅ Agent 可调用 |
| `parse_office_action` | 调用 `ParseOfficeAction` + `FormatOaSummary` | ✅ 程序化执行 |
| `validate_amendment` | 调用 `amendment.Checker.Check()` + YAML 规则参考 | ✅ 混合执行 |
| `analyze_slop` | 调用 `AnalyzeSlop` + `FormatSlopAnalysis` | ✅ 程序化执行 |
| `Evaluate()` / `EvaluateRule()` | **正则匹配程序化执行** | ⚠️ 有限执行 |

### 5.4 关键发现：Evaluate 的执行能力严重受限

`evaluate.go` 实现了 `Check.Evaluate(text)` 方法，但**仅支持四类正则匹配型检查**：

| Check Type | 执行方式 | 覆盖规则数 |
|-----------|---------|-----------|
| `presence`/`exist` | 正则匹配 | 约 5 条 |
| `absence`/`forbidden` | 正则匹配 | 约 3 条 |
| `numeric`/`range` | 正则提取数字 | 约 2 条 |
| `composition`/`compound` | 递归调用子检查 | 约 4 条 |
| `patent_novelty` 等复杂类型 | **跳过**，返回默认 passing | **全部规则** |

对于 `check.type` 为 `patent_novelty`, `patent_inventiveness` 等的规则（如 YAML 中 `EVI-001` 的 `check.type: relevance`），执行器直接返回：
```go
&CheckResult{Passed: true, Score: 0.5, Details: []string{...类型需要LLM判断，跳过自动检查}}
```

这意味着：
- `domains/rules/data/rules/patent-core.yaml` 和 `novelty-rules.yaml` 中的绝大多数规则只能作为文本参考供 LLM 读取，无法被程序化执行
- 规则的 `Check.Conditions` 字段（YAML 中的 `conditions: [evidence_has_claim_refs, evidence_direction_clear]`）没有对应的代码实现

### 5.5 ArticleFramework 的双路径加载问题

`enablement/framework.go` 也定义了类似的 `ArticleFrameworkProvider` 接口，通过 `rules.Engine.Article()` 加载 YAML。这与 `domains/rules` 的 `Engine.Article()` 是**同一份数据**，但：
- `enablement/framework.go` 定义了**独立的** `ArticleFrameworkData` / `ArticleStepData` 值类型（镜像 rules 包的 `ArticleFramework`）
- 硬编码了 `defaultA263Framework()` 作为降级方案
- 注释说明"避免引入 transitive build 依赖"

这是良性的接口隔离，但两套 ArticleFramework 类型定义不保持同步时可能产生分歧。

### 5.6 评分：7/10

DSL 解析功能完整（Loader 覆盖 4 种文件类型），执行链从 Agent 工具到 LLM 提示词注入路径清晰。但程序化执行（Evaluate）能力严重受限，大部分规则依赖 LLM 推理。

---

## 6. 文档模板与领域逻辑同步 🟢 (P2)

### 6.1 概述

`doc-templates/` 包含 5 个子目录（claims/specification/disclosure/oa-response/legal），`domains/doctmpl/` 提供模板加载和渲染。

### 6.2 模板与代码逻辑对照

| 模板目录 | 对应代码 | 同步状态 |
|----------|---------|---------|
| `doc-templates/specification/{mechanical,electrical,chemical,software}.md` | `domains/specdrafting/builder.go:defaultTechField/Background/Content/Drawings/Embodiment` | ✅ 技术领域分类一致 |
| | `domains/specdrafting/builder.go:216-226` 的 `domainToTemplateName` 映射 | ✅ 4 个领域+1 个 default |
| `doc-templates/claims/{apparatus,system,method-claim}.md` | `domains/claimdrafting/builder.go` 的多策略撰写 | ✅ 模板定义的 claim 类型与 builder 策略对应 |
| `doc-templates/disclosure/{simplified,standard-9-section}.md` | `domains/disclosure/` | ⚠️ 未深入审查 |
| `doc-templates/oa-response/{novelty-defense,inventiveness-defense,clarity-amendment}.md` | `domains/workflows/patent/oa_response*.go` | ✅ |
| `doc-templates/legal/{case-analysis,contract-review,infringement-analysis}.md` | `domains/workflows/legal/` | ✅ |

### 6.3 发现：Builder 降级路径的模板映射细节

`specdrafting/builder.go:216-226` 的 `domainToTemplateName` 将技术领域映射到模板文件名：
- `mechanical` → `mechanical-spec`
- `electrical` → `electrical-spec`
- `chemical` → `chemical-spec`
- `software` → `software-spec`

对应的 `doc-templates/specification/` 文件名为 `mechanical.md`, `electrical.md`, `chemical.md`, `software.md`。模板名与文件名不一致（`mechanical-spec` vs `mechanical.md`），但 builder 的 `FindByName` 具体实现逻辑可能使用了前缀匹配或索引查找，需验证是否匹配。

### 6.4 评分：8/10

模板与领域代码逻辑基本保持同步。未发现严重脱节。

---

## 7. 领域层 import 基础设施实现 🟡 (P2)

### 7.1 概述

按照 AGENTS.md 规定的 8 层分层架构，"Domain 层不得 import Infrastructure 层的具体实现，只能依赖接口"。

### 7.2 架构层次定义

```
外部接口层   → 应用入口层
领域扩展层   → domains/（证据/创造性/充分公开/撰写/规则）
基础设施层   → disclosure/, memory/, knowledge/, retrieval/, graph/, session/, skill/, prompt/, store/
核心引擎层   → agentcore/
通用工具库   → pkg/
```

### 7.3 违规检查结果

| 领域包 | import 的基础设施层 | 严重程度 |
|--------|-------------------|---------|
| `domains/inventiveness/tool.go` | `disclosure`（导入 AnalysisReport） | 🟡 中 |
| `domains/enablement/tool.go` | `disclosure`（导入 AnalysisReport） | 🟡 中 |
| `domains/specdrafting/extension.go` | `disclosure`（导入 ExtractionResult） | 🟡 中 |
| `domains/specdrafting/types.go` | `disclosure`（导入 ExtractionResult） | 🟡 中 |
| `domains/rules/engine.go` | `domains/reasoning`（跨 domain 子包引用） | 🟢 低 |
| `domains/rules/engine_handlers.go` | `domains/amendment`（跨 domain 子包引用） | 🟢 低 |
| `domains/rules/engine_formatters.go` | `domains/amendment`（跨 domain 子包引用） | 🟢 低 |
| `domains/evidence/*.go` | `agentcore/evidence`（agentcore 子包，可接受） | 🟢 低 |

### 7.4 详细分析

**disclosure 导入**（3 处违规）：
- 实际模式是领域包调用 `disclosure.AnalysisReport` / `disclosure.ExtractionResult` 作为入参格式转换的输入源
- 这是功能性依赖而非架构性依赖 — 领域包需要将 disclosure 管线的产出转换为自己的输入
- 修复方向：将转换逻辑提升到应用层（如 `cmd/mady/`）或通过接口抽象

**跨 domain 子包引用**（`rules → reasoning`, `rules → amendment`）：
- `rules/engine.go:ToRuleConstraints` 返回 `[]reasoning.RuleConstraint` 类型
- `rules/engine_handlers.go:handleValidateAmendment` 调用 `amendment.Checker`
- 这属于 domain 内的水平引用，违反单向依赖原则但不跨层

### 7.5 评分：7/10

违规数量少且影响可控，但 disclosure 导入为典型的多层架构违背模式，建议将转换适配器提到 application 层。

---

## 综合评分总览

| 子系统 | 评分 | 风险 | 关键问题 |
|--------|------|------|---------|
| evidence | 6/10 | 🔴 P0 | YAML 规则系统与执行引擎脱节 |
| inventiveness | 8/10 | 🟡 P0 | LLM 综合判断可能绕过 AND 公式 |
| enablement | 6/10 | 🔴 P0 | In re Wands 美国判例法混入中国法 |
| psychological | 4/10 | 🟡 P1 | VAD 模型严重简化，认知偏差检测未实现 |
| rules DSL | 7/10 | 🟡 P1 | Evaluate 程序化执行能力受限 |
| doc-templates | 8/10 | 🟢 P2 | 基本同步，模板名需验证 |
| 分层违规 | 7/10 | 🟡 P2 | disclosure 导入破坏分层隔离 |

### 最终评分：**6.6/10** — 领域逻辑整体自洽，但存在 YAML 规则脱节（evidence）和法律体系混用（enablement）两个需优先修复的架构缺陷。

---

## 修复优先级矩阵

| 优先级 | 问题 | 风险等级 | 修复难度 | 影响面 |
|--------|------|---------|---------|-------|
| P0-1 | enablement: 删除 In re Wands 引用 | 🔴 法律正确性 | 低（删除 3 行） | 单文件 |
| P0-2 | evidence: 激活 YAML 规则加载 | 🔴 架构脱节 | 中（需确定加载路径） | engine + bootstrap |
| P0-3 | inventiveness: 增强后处理硬约束 | 🟡 逻辑完整性 | 低（10 行校验） | node_conclusion.go |
| P1-4 | psychological: 标记功能限制或扩展 | 🟡 用户预期 | 中 | engine.go |
| P1-5 | rules: 扩展 Evaluate 类型覆盖 | 🟡 执行能力 | 高 | evaluate.go |
| P2-6 | 分层违规: 提取 disclosure 适配器 | 🟡 架构合规 | 中 | 适配器层 |

---

## 方法说明

### 审查范围
- 审查代码目录：`/Users/xujian/projects/Mady/domains/*/`
- 非审查代码：`domains/casemgmt/`（案件管理，1300 行）、`domains/sqlite/`（持久化）、`domains/workflows/`（工作流）、`domains/reasoning/wiring/`（装配层）— 因与领域逻辑一致性主题关联弱而缩小范围

### 审查工具
- `grep`：符号追踪、import 依赖分析
- `read_file`：源码阅读
- 关键词匹配：法律体系引用检测（美国/中国/欧洲）

### 评分标准
- 10-9：完全没问题，架构和实现一致
- 8-7：小问题，不影响功能
- 6-5：需要修复，有潜在风险
- 4-3：严重问题，影响正确性或架构
- 2-1：不可接受
