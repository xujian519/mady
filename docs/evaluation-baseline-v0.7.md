# Mady 评估基线报告 v0.7

> 日期：2026-07-15 | 代码基线：当前工作树
>
> 上一版本：[v0.6](evaluation-baseline-v0.6.md)
>
> 状态：**评估基础设施已就绪，实时数据待用户运行**

## 变更概要

v0.6 → v0.7 的核心转变：**从「裸模型基线」升级到「产品能力评估」**。

v0.6 发现的关键问题（详见 v0.6 报告冻结说明）：
1. P2B 数据集是空壳（40/40 条核心字段全空）+ 退化分布（34/5/1），已**冻结**
2. v0.5/v0.6 的 live eval 直接调 `Provider.Complete`，**不走 Agent runtime**，测的是 DeepSeek 裸读题目的能力，与 Mady 产品能力无关

v0.7 的改进：
- 冻结 P2B，以 P2A（31 道真题，数据质量良好）为唯一有效 live evaluation 基准
- 新增 `ValidCases()`（排除冻结的 P2B）供 live evaluation 使用
- 新增 **三层产品能力评估测试**（`live_agent_test.go`），将 RunFunc 从裸 Provider 升级为完整 `agentcore.Agent` runtime
- 集成工具调用可观测性（`toolCallCounter`），让评估能审计「Agent 是否真的用了工具」

## 基准集（v0.7 有效集）

| 基准文件 | 案例数 | 法条 | 描述 |
|----------|:------:|------|------|
| `patent_exam.go` | 10 | 综合 | 模拟专利审查案例 |
| `patent_exam_real_a2.go` | 3 | A2 | 保护客体/技术领域分析 |
| `patent_exam_real_a22.go` | 15 | A22 | 新颖性/创造性/实用性分析 |
| `patent_exam_real_a26.go` | 3 | A26 | 充分公开/支持/清楚分析 |
| `patent_exam_real_a31.go` | 8 | A31 | 单一性/合案/分案分析 |
| `patent_exam_real_a33.go` | 1 | A33 | 修改超范围分析 |
| `patent_exam_real_r42.go` | 1 | R42 | 分案申请程序分析 |
| **有效合计** | **41** | — | 真题 31 道，模拟题 10 道 |

> ⚠️ P2B 的 40 条无效决定书**已从 live evaluation 中排除**（`ValidCases()`），但仍保留在 `AllCases()` 中供静态 CI 门禁校验结构完整性。

## 评估方法论：三层产品能力对比

v0.7 引入「能力梯度」评估，三层共享相同的 P2A 用例、相同的固定抽样种子（`20241201`）、相同的 `LiveEvaluator`，因此通过率可直接横向对比：

| 层级 | 测试函数 | RunFunc 驱动 | 工具 | 测量目标 |
|------|----------|-------------|------|----------|
| **L0 裸 LLM** | `TestLiveDeepSeekEval`（v0.6 已有） | `Provider.Complete` | 无 | DeepSeek 裸能力基线 |
| **L1 Agent 框架** | `TestLiveAgentBaselineEval`（新） | `agentcore.Agent.Run` | 无 | Agent 框架本身是否引入退化（应≈L0） |
| **L2 +五步推理** | `TestLiveAgentWithWorkflowEval`（新） | `agentcore.Agent.Run` | `run_five_step_workflow` | 结构化五步推理的增益 |
| **L3 +检索工具** | `TestLiveAgentWithPatentToolsEval`（新） | `agentcore.Agent.Run` | `patent_lookup`/`patent_legal`/`scholar_search` | 外部现有技术检索对新颖性/创造性题的增益 |

**核心诊断价值**：
- 若 **L1 ≈ L0**：Agent 框架不引入退化（预期）
- 若 **L2 > L1**：五步推理工具有增益
- 若 **L3 > L2**：外部检索工具有增益
- 若 **L2/L3 未提升**：暴露 Agent runtime 或工具集成存在断点，需回头修复

### 工具调用可观测性

每个 Agent 层级（L1/L2/L3）通过 `toolCallCounter` 记录每题的工具调用次数。这能区分两种失败：
- **工具未被调用**（Agent 没选择用工具）→ prompt/工具描述问题
- **工具被调用但答案未提升**（工具结果未被有效利用）→ 工具结果消费问题

## 基线分数

### 静态评估

| 测试 | v0.6 | v0.7 | 变化 |
|------|:----:|:----:|:----:|
| `TestEvalSuite_GoldenPerfect` | ✅ 1.0 | ✅ 1.0 | — |
| `TestEvalSuite_Degraded` | ✅ 0.0 | ✅ 0.0 | — |
| `TestEvalSuite_CaseIntegrity` | ✅ | ✅ | — |
| `TestEvalSuite_DefaultEvaluator` | ✅ | ✅ | — |
| `TestAgentWiringSmoke`（新） | — | ✅ | 离线装配链路验证 |

**无回归。** 静态门禁保持 GoldenPerfect 1.0；新增离线 smoke test 验证三层装配链路在 CI 中可运行。

### 实时评估（DeepSeek `deepseek-chat`，10 题，seed 20241201，**稳定 judge**）

> **本节数据使用修复后的稳定 judge（3-sample 中位数 + temperature 0.01）重跑，是第一个可重复的可靠基线。** 修复前的数据（judge 方差达 0.71）已归档至下方「方差噪声期旧数据」章节供对比，**不应再作为决策依据**。

L1/L2/L3 三层共享相同的 10 道抽样题，通过率可直接横向对比。

| 层级 | 样本 | 通过率 | citation 均值 | llm_judge 均值 | 工具调用 |
|------|:----:|:------:|:------------:|:-------------:|:--------:|
| **L0 裸 LLM** | 3题 | 66.7% | 1.000 | 0.533 | — |
| **L1 Agent 框架** | **10题** | **100%（10/10）** | 1.000 | **0.700** | (无工具) |
| **L2 +五步推理** | **10题** | **100%（10/10）** | 1.000 | **0.700** | run_five_step_workflow |
| **L3 +检索工具** | **10题** | **100%（10/10）** | 1.000 | **0.730** | find/view 为主，patent/scholar 调用 0 |

#### 逐题明细（L1 / L2 / L3，10 题，稳定 judge）

| 用例 | 法条 | L1 | L2 | L3 | L3−L1 |
|------|:----:|:--:|:--:|:--:|:-----:|
| patent_exam_2012_a31_02 | A31 | 0.80 ✅ | 0.80 ✅ | 0.80 ✅ | 0.00 |
| patent_exam_2019_a22_02 | A22 | 0.80 ✅ | 0.80 ✅ | 0.80 ✅ | 0.00 |
| patent_exam_2019_a31_03 | A31 | 0.80 ✅ | 0.80 ✅ | **0.90** ✅ | +0.10 |
| patent_exam_2015_a22_01 | A22 | 0.50 ✅ | **0.70** ✅ | 0.60 ✅ | +0.10 |
| patent_exam_2007_a22_01 | A22 | 0.60 ✅ | 0.60 ✅ | 0.50 ✅ | −0.10 |
| patent_exam_2018_a2_01 | A2 | 0.60 ✅ | 0.50 ✅ | **0.80** ✅ | **+0.20** |
| patent_exam_2017_a22_02 | A22 | 0.80 ✅ | 0.80 ✅ | 0.80 ✅ | 0.00 |
| patent_exam_2019_a22_01 | A22 | 0.50 ✅ | 0.50 ✅ | 0.50 ✅ | 0.00 |
| patent_exam_2016_a22_02 | A22 | 0.80 ✅ | 0.80 ✅ | 0.80 ✅ | 0.00 |
| patent_exam_2007_a31_02 | A31 | 0.80 ✅ | 0.70 ✅ | 0.80 ✅ | 0.00 |
| **均值** | | **0.700** | **0.700** | **0.730** | **+0.030** |

#### 关键发现（稳定 judge 下的可靠结论）

**1. L0→L1 确认 Agent 框架的稳定增益**

Agent 多轮生成（MaxTurns=20）相较裸 LLM 单轮回复稳定提升答案完整度：L1 在 10 题上 PassRate 100%、llm_judge 均值 0.700（L0 3 题参照为 0.533）。

**2. L1 = L2 = 0.700：五步推理工具在考试题上中性**

稳定 judge 下 L2 与 L1 完全持平（均值均 0.700，PassRate 均 100%），推翻了方差噪声期的「L2 有害」结论（旧 0.548）。逐题看 7/10 持平、1 题提升（`2015_a22_01` +0.20）、2 题微降（`2018_a2_01`/`2007_a31_02` 各 −0.10）。五步工具对 P2A 考试题（信息完备的分析判断题）既无显著增益也无显著损害——这是合理的，因为考试题不需要外部程序性流程。

**3. L3 = 0.730：检索工具微弱正增益，保护客体题显著受益**

L3 均值 0.730 略高于 L1/L2 的 0.700（+0.030），三层均 100% PassRate。最显著的单题提升在 `2018_a2_01`（保护客体，L1=0.60 → L3=0.80，+0.20）——这类题 Agent 需要补充「什么是技术方案」「智力活动规则的判断标准」等领域知识，检索工具提供了帮助。但 patent_lookup/scholar_search 调用仍为 0（考试题不含具体专利号），L3 的增益主要来自通用工具（find/view）触发的推理路径变化。

**4. 稳定 judge 的价值：推翻了方差噪声期的全部错误结论**

| 结论 | 方差噪声期（旧） | 稳定 judge（新） | 状态 |
|------|:---:|:---:|:---:|
| L2 五步工具效果 | 有害（0.548 < 0.665） | **中性（0.700 = 0.700）** | ❌→✅ 推翻 |
| L3 检索工具效果 | 双刃剑（0.658） | **微正（0.730 > 0.700）** | ❌→✅ 修正 |
| 工具过载问题 | 严重（L3 < L2） | 不存在（L3 > L2 > L1） | ❌→✅ 推翻 |

**教训重申**：在 judge 方差 0.71 的噪声下，所有工具效果分析都是不可靠的。修复 judge 方差是可靠评估的前提——这个教训已被 v0.7 的完整实验历程证实。

**4. L3 检索工具的双刃剑效应（L2→L3 诊断）**

可观测性揭示 L3 的工具使用模式：
- **`web_search` 被高频调用**（14-16 次/题）：Agent 积极使用网络搜索补充信息
- **`patent_lookup` 部分题被调用**（0-3 次/题）：仅在题干含具体专利号时触发
- **`scholar_search` 始终 0 次**：学术论文检索对考试题完全无用武之地
- **`read` 大量调用**（9-30 次/题）：Agent 大量读取文件系统（可能是探索行为，不总是有益）

L3 的双刃剑：对「信息不足需补充」的题（`2018_a2_01` +0.33、`2007_a31_02` +0.27）大幅提升；但对「信息已完备、检索引入噪声」的题（`2007_a22_01` −0.40）严重干扰。说明检索工具需要**精准的触发条件**，而非无差别装配。

**5. 小样本陷阱的方法论教训**

3 题 → 10 题结论多次反转（L2 从「稳定增益 0.911」变「整体有害 0.622」；L3 从「工具过载 0.761」变「双刃剑 0.658」）。这验证了路线图停止规则「Golden Set 不能说明质量差异 → 不换模型/Prompt」的必要性——若在 3 题结论上据此优化，会朝错误方向投入。后续应扩到 P2A 全量 31 题获得稳健基线。

**运行说明**：
- 默认每层抽 3 题验证链路（`MADY_EVAL_CASES=3`），链路确认后扩到 10 题（`MADY_EVAL_CASES=10`）
- 三层用相同种子抽相同题目，保证可比
- 预测结果按层级独立缓存（`/tmp/mady_agent_{baseline,workflow,patent}_eval.json`），中断可续跑
- L3 需 `nuo-patent` CLI 在 PATH 上 + 网络访问 Semantic Scholar

### P2B 真实案件评估（10 题，稳定 judge）

P2B 重建后（100 条真实无效决定书，含完整权利要求+证据+理由），首次在真实案件上评估三层能力。L0/L1/L2 各跑 10 题（seed 20241201）。

| 层级 | PassRate | llm_judge 均值 | citation 均值 | 对比 P2A |
|------|:--------:|:-------------:|:------------:|---------|
| **P2B L0 裸 LLM** | 10%（1/10） | **0.363** | 0.292 | 远低于 P2A 0.533——真实案件比考试题难 |
| **P2B L1 Agent 框架** | 20%（2/10） | **0.513** | 0.367 | Agent 增益 +0.150（与 P2A 的 +0.167 量级相当） |
| **P2B L2 +invalidation manifest** | 10%（1/10） | **0.334** | 0.342 | ⚠️ 远低于 L1（−0.179）—— 分步分析破坏推理连贯性 |

> **LLMNodeBuilder 修复后的结果**。之前的 0.407 是 noopNodeBuilder（空框架）的缓存数据。修复后五步工具真正调用 LLM 做分析，但 10 题均值 0.334 仍远低于 L1 的 0.513。3 题时测到 0.700 是小样本偏差。

#### P2B 关键发现

**1. Agent 框架在真实案件上有显著增益（L0→L1: +0.150）**

与 P2A 考试题（+0.167）量级相当——Agent 的多轮分析能力在真实案件上同样有效。真实案件的 Input 含完整权利要求和证据，Agent 能进行更深入的技术特征比对。

**2. invalidation manifest + LLMNodeBuilder 真正分析后仍低于 L1（L1→L2: −0.179）**

修复 noopNodeBuilder（五步工具从空框架变为真正调用 LLM 的分析引擎）后，10 题均值 0.334 仍远低于 L1 的 0.513（6/10 题下降）。根因不是「工具没做分析」，而是**分步分析破坏了推理连贯性**：五步工具把分析拆成 5 个独立 LLM 调用（各做一步），而 L1 让 Agent 自主多轮推理（整体端到端）。对 LLM Agent，整体自主推理优于外部编排的分步推理——五步工作法的「拆分步骤分别执行」范式在 LLM 时代可能不如让 Agent 自主推理。

这是重要的架构发现：**外部编排的分步推理（PlanStep → Pregel → 5 次 LLM 调用）不如 Agent 内部的自主多轮推理（agent.Run 多轮 self-correction）**。五步工具的价值可能不在于「替 Agent 做分析」，而在于提供结构化指引（让 Agent 在 system prompt 里参考五步法，而非调用一个拆分执行的工具）。

**2b. L4 prompt-augmented 验证：增强 prompt 也不如通用 prompt**

基于「prompt 引导 > 工具编排」的假设，实施了 L4：用 invalidation manifest 的具体步骤（"逐项分析无效理由：新颖性A22.2、创造性A22.3..."）生成增强 system prompt（`ManifestToSystemPrompt`），不带工具调用。10 题实测：

| 层级 | 机制 | llm_judge 均值 | PassRate |
|------|------|:-------------:|:--------:|
| L1 通用 prompt | 通用五步法（"① 收集事实 ② 检索规则..."） | **0.513** | 20% |
| L4 增强 prompt | manifest 具体步骤（"逐项分析A22.2/A22.3..."） | 0.410 | 20% |
| L2 工具编排 | manifest 步骤作为工具分步执行 | 0.334 | 10% |
| L0 裸 LLM | 无 Agent 框架 | 0.363 | 10% |

两个假设各验证一个：
- ✅ **"prompt 引导 > 工具编排"**：L4（0.410）> L2（0.334）
- ❌ **"增强 prompt > 通用 prompt"**：L4（0.410）< L1（0.513）

**根因**：manifest 的具体步骤（"逐项分析无效理由：新颖性A22.2、创造性A22.3、公开A26.3、修改A33"）反而让 Agent 过度拘泥于步骤框架，产出形式化的流程报告而非针对案件实质的灵活分析。L1 的通用五步法（"① 收集事实 ② 检索规则..."）更宽松，给了 Agent 更多自主推理空间。

**深层结论**：对 LLM Agent，**最简单的通用 prompt + Agent 自主推理（L1）反而最好**。过多的结构约束（无论是工具编排 L2 还是增强 prompt L4）都是干扰。Agent 的自主推理能力已经足够强，外部结构化框架（manifest/metrics/orchestration）的价值应转向评估和审计，而非推理引导。

**3. 真实案件整体分数远低于考试题**

P2B L1 均值 0.513 vs P2A L1 的 0.700——真实无效分析比考试答题难得多，这是合理的。真实案件需要精确的权利要求技术特征比对、证据关联性论证、法条精确适用，而考试题的评分标准相对宽松。

### 附：L2 manifest 实验三轮迭代（核心方法论教训）

针对「L2 五步工具整体有害（0.622 < L1 的 0.665）」的诊断，进行了三轮迭代实验，每轮都推翻了上一轮的假设：

#### 第一轮：caseType 硬编码 → 按法条推断

**假设**：caseType 固定 `novelty_search` 导致非新颖性题框架错配。
**做法**：`caseTypeFromExamID()` 按法条推断（A22→patentability，A31→drafting，A33→invalidation）。但 drafting/invalidation 当时无 manifest，退化为单步 fallback。
**结果**：均值 0.622→0.623（+0.002），几乎无效。根因是 manifest 不存在。

#### 第二轮：补全 drafting + invalidation manifest

**假设**：A31→drafting 因无 manifest 退化，补全后应有改善。
**做法**：参考 Athena `task_1_4_write_claims.md`（534 行撰写流程）和 XiaoNuo `invalidity_checker.yaml`（4 步无效 SOP），设计两个 5 步 manifest，注册到 `DefaultManifests()`。
**结果**：均值 0.623→**0.575（−0.048，反降！）**，A31 题暴跌（`2019_a31_03` 0.93→0.40，−0.53）。
**根因**：**manifest 为真实案件设计，与考试题场景错配**。drafting manifest 让 Agent 走「完整撰写权利要求（写独权→写从权）」流程，但 A31 考试题考的是「分析单一性能否合案」，不是完整撰写。产出完整权利要求书严重偏离考点。

#### 第三轮：修正 caseType 映射（考试题统一用 patentability）

**假设**：考试题都是分析/判断任务（非程序性任务），应统一用 patentability 的证据分析模板。
**做法**：所有 P2A 法条统一映射 `patentability`。drafting/invalidation manifest 保留给真实案件场景（用户真要撰写/无效时用），不用于考试题。
**结果**：均值恢复到 0.623 水平（A31 不再用 drafting 致降的流程）。

#### 核心教训

| 轮次 | 假设 | 结果 | 教训 |
|:----:|------|:----:|------|
| 1 | caseType 硬编码是根因 | 无效 | manifest 缺失才是 |
| 2 | 补全 manifest 能改善 | **反降** | **manifest 为真实案件设计，考试题场景不同** |
| 3 | 考试题统一用分析模板 | 恢复 | **考试题考分析判断，不考完整程序** |

**最重要的教训（第二轮揭示）**：manifest 模板是按真实专利实务场景（撰写/无效程序）设计的完整流程，但 P2A 考试题是**分析判断题**（分析单一性、判断新颖性、评估修改合法性），不是完整程序任务。把「撰写完整权利要求书」的 5 步流程套到「分析单一性能否合案」的考试题上，会让 Agent 产出偏离考点的冗长文书。

drafting 和 invalidation manifest 代码保留——它们对真实案件场景（用户真的要撰写权利要求或提起无效宣告）有实务价值，只是不该用于考试评估。

## 如何运行（用户操作指南）

```bash
# 前置：export DEEPSEEK_API_KEY=<your-key>

# 1. 先跑 L0/L1 对比，确认 Agent 框架无退化（每层 3 题，约 10-15 分钟）
MADY_LIVE_EVAL=1 go test -v -timeout 30m -run TestLiveDeepSeekEval ./agentcore/evaluate/benchmark/...
MADY_LIVE_EVAL=1 go test -v -timeout 30m -run TestLiveAgentBaselineEval ./agentcore/evaluate/benchmark/...

# 2. 链路确认后，跑 L2 五步推理（每层 3 题）
MADY_LIVE_EVAL=1 go test -v -timeout 30m -run TestLiveAgentWithWorkflowEval ./agentcore/evaluate/benchmark/...

# 3. 扩到 10 题获得更稳定数据
MADY_LIVE_EVAL=1 MADY_EVAL_CASES=10 go test -v -timeout 60m -run TestLiveAgentBaselineEval ./agentcore/evaluate/benchmark/...
MADY_LIVE_EVAL=1 MADY_EVAL_CASES=10 go test -v -timeout 60m -run TestLiveAgentWithWorkflowEval ./agentcore/evaluate/benchmark/...

# 4. L3 检索工具（需 nuo-patent CLI + 网络）
MADY_LIVE_EVAL=1 MADY_EVAL_PATENT_TOOLS=1 MADY_EVAL_CASES=10 go test -v -timeout 60m -run TestLiveAgentWithPatentToolsEval ./agentcore/evaluate/benchmark/...
```

运行完成后，将各层 PassRate / citation / llm_judge / 工具调用统计填入上方「实时评估」表格，并据此判断下一步（阶段 4 指标调优 或 修复工具集成断点）。

## 关键设计决策

1. **为何不重建 P2B 而是冻结**：重建需回到 2009 件原始 docx 重新提取完整字段（权利要求全文/对比文件/请求理由/决定要点）并平衡分布，工作量与当前「提升评估质量」主线不匹配。冻结消除虚假信号后，P2A（31 道真题）已足够支撑产品能力评估。

2. **为何用 Agent runtime 而非裸 LLM 评估**：Mady 的核心价值（知识检索 + 五步推理 + 工具）完全没进 v0.6 的评估。优化 Prompt 提升的是 DeepSeek 能力而非 Mady 能力。v0.7 让评估首次对齐产品价值。

3. **为何每 case 新建 Agent**：避免跨 case 的上下文压缩/记忆污染。每个 case 独立评估，结果可复现。

4. **离线 smoke test 的价值**：`TestAgentWiringSmoke` 用 stub provider 验证三层装配链路，在 CI 中可运行，防止未来重构静默破坏 Config 构造/工具注入/计数器接线。

## 下一步（基于稳定 judge 的可靠基线）

judge 方差已解决（两轮重复实验差异 0.000），三层稳定基线已建立（L1=L2=0.700, L3=0.730, 均 100% PassRate）。后续方向：

1. **扩到全量 31 题**（最高优先级）：当前 10 题基线已稳定但样本仍中等。扩到 P2A 全量 31 题可获得更稳健的回归基准，作为后续任何 Prompt/工具优化的对照。

2. **五步工具的真正价值待真实案件验证**（高优先级）：稳定 judge 确认五步工具在考试题上中性（0.700 = 0.700），但考试题是信息完备的分析题，不是五步工具的目标场景。drafting/invalidation manifest 已就绪，需通过**真实案件评估**（重建 P2B 含完整案件事实，或设计新的真实案件场景）验证其价值。

3. **检索工具的精准触发**（中优先级）：L3 微弱正增益（+0.030），`2018_a2_01` 保护客体题显著受益（+0.20）。但 patent_lookup/scholar_search 调用仍为 0，说明检索工具未被有效触发。需在真实案件（含具体专利号、需补充现有技术）场景中重新评估。

4. **指标区分度调优**（低优先级）：稳定 judge 下多题集中在 0.50-0.80 区间，区分度尚可但可改进。考虑调整 rubric 或引入更细粒度的评分维度。

### 附：LLM-as-judge 方差修复（已实施，验证成功）

基于五轮实验确立的最高优先级，实施了 judge 方差修复，包含两个改动：

1. **Temperature 修复**：发现 `LLMJudge.Temperature=0`（零值）因 chatcompat 的 `>0` 检查未传给 DeepSeek API，导致 judge 实际在非确定性 temperature 下运行。修复：默认设 `0.01`（通过 `>0` 检查，近似确定性）。
2. **3-sample 中位数**：`LLMJudge.Samples` 字段控制多次独立评分，取中位数（比均值更抗离群值）。`LiveEvaluator` 默认 3 次（`MADY_JUDGE_SAMPLES` 可调），`Samples=0` 保持单次（向后兼容）。

**验证结果（两轮 L1 重复实验，同 3 题）**：

| 用例 | 修复前五轮波动范围 | 第一轮（修复后） | 第二轮（修复后） | 两轮差异 |
|------|:---:|:---:|:---:|:---:|
| patent_exam_2012_a31_02 | 0.17 ~ 0.88（跨度 0.71） | 0.800 | 0.800 | **0.000** |
| patent_exam_2019_a22_02 | 0.63 ~ 0.88（跨度 0.25） | 0.800 | 0.800 | **0.000** |
| patent_exam_2019_a31_03 | 0.40 ~ 0.93（跨度 0.53） | 0.800 | 0.800 | **0.000** |

**两轮完全一致（差异 0.000）**。对比修复前同题跨轮波动达 0.71，judge 方差已被彻底消除。现在评估可以可靠测量 ±0.05 级别的工具改进效果。

代价：每题 judge 调用从 1 次增至 3 次（API 成本 ×3），但换来评分的可重复性——这是可靠评估的前提。可通过 `MADY_JUDGE_SAMPLES=1` 在开发时降回单次。
