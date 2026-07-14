# Mady 评估基线报告 v0.6

> 日期：2026-07-14 | 代码基线：当前工作树
>
> 上一版本：[v0.5](evaluation-baseline-v0.5.md)

## 变更概要

v0.5 → v0.6 完成 P2B 阶段：
- 从 2009 件无效宣告请求审查决定书中筛选 40 件典型案例，构建 Golden Set 第二层
- 运行 DeepSeek 对全部 40 道无效决定书案例进行实时评估，建立 LLM 基线

v0.6 后续代码质量修复：
- 将 P2B 40 个案例从 `invalidation_decisions.go` 硬编码迁出到 `invalidation_decisions.json`，使用 `go:embed` 加载
- 修复 3 个数据错误：`invalidation_decision_004`（结论与理由矛盾）、`invalidation_decision_039`（Expected 为段落标题）、`invalidation_decision_040`（专利号缺失）
- 重构 `CitationCompleteness`：中文数字归一化增加上下文保护（仅作用于 `第X条/款/项` 结构），扩展法条正则支持 `第X条第Y款第Z项` 与 `第X条之一` 等复杂引用，新增项级与概括匹配
- 合并 `live_deepseek_test.go` 重复代码，引入 `deepSeekTestEnv`/`randomCases`/`runLiveEval` 公共 helper，固定真题抽样种子为 `20241201`

## 基准集

### Golden Set 第一层（P2A）

| 基准文件 | 案例数 | 法条 | 描述 |
|----------|:------:|------|------|
| `patent_exam.go` | 10 | 综合 | 模拟专利审查案例 |
| `patent_exam_real_a2.go` | 3 | A2 | 保护客体/技术领域分析 |
| `patent_exam_real_a22.go` | 15 | A22 | 新颖性/创造性/实用性分析 |
| `patent_exam_real_a26.go` | 3 | A26 | 充分公开/支持/清楚分析 |
| `patent_exam_real_a31.go` | 8 | A31 | 单一性/合案/分案分析 |
| `patent_exam_real_a33.go` | 1 | A33 | 修改超范围分析 |
| `patent_exam_real_r42.go` | 1 | R42 | 分案申请程序分析 |
| **合计** | **41** | — | 真题 31 道，模拟题 10 道 |

### Golden Set 第二层（P2B）

| 基准文件 | 案例数 | 覆盖 | 描述 |
|----------|:------:|------|------|
| `invalidation_decisions.json` (由 `invalidation_decisions.go` 通过 `go:embed` 加载) | 40 | 发明 8 / 实用新型 16 / 外观设计 16 | 真实无效宣告请求审查决定书 |

P2B 案例分布：
- 结论：全部无效 16 件、维持有效 14 件、部分无效 10 件
- 专利类型：发明 8 件、实用新型 16 件、外观设计 16 件
- 主要法条：专利法第22条第3款（创造性）为主，第22条第2款（新颖性）、第26条第3/4款（公开充分/支持清楚）、第46条第1款（程序决定）为辅

## 指标

| 指标 | 说明 |
|------|------|
| `F1Score` | Token 重叠（静态评估） |
| `KeywordRecall` | 参考关键词在预测中的比例（静态评估） |
| `CitationCompleteness` | 必需引用的比例 |
| `JudgeConsistency` | 启发式关键词一致性（静态评估） |
| `LLMJudge` | LLM 按 rubric（结论/推理/引用）评分（LiveEval） |

## 基线分数

### 静态评估

| 测试 | v0.5 | v0.6 | 变化 |
|------|:----:|:----:|:----:|
| `TestEvalSuite_GoldenPerfect` | ✅ 1.0 | ✅ 1.0 | — |
| `TestEvalSuite_Degraded` | ✅ 0.0 | ✅ 0.0 | — |
| `TestEvalSuite_CaseIntegrity` | ✅ | ✅ | — |
| `TestEvalSuite_DefaultEvaluator` | ✅ | ✅ | — |

**无回归。** 41 + 40 = 81 个基准案例保持 GoldenPerfect 1.0 PassRate。

### 实时评估（DeepSeek）—— P2A 真题基线

| 项目 | 结果 |
|------|------|
| 运行命令 | `MADY_LIVE_EVAL=1 go test -v -timeout 30m -run TestLiveDeepSeekEval ./agentcore/evaluate/benchmark/...` |
| 模型 | `deepseek-chat` |
| 抽样 | 随机 3 题（真题） |
| 通过率 | **66.7%（2/3）** |
| `citation_completeness` 平均 | **1.000** |
| `llm_judge` 平均 | **0.456** |

### 实时评估（DeepSeek）—— P2B 无效决定书基线

| 项目 | 结果 |
|------|------|
| 运行命令 | `MADY_LIVE_EVAL=1 go test -v -timeout 90m -run TestLiveDeepSeekInvalidationEval ./agentcore/evaluate/benchmark/...` |
| 模型 | `deepseek-chat` |
| 抽样 | 全部 40 道无效决定书案例 |
| 运行时长 | 约 17 分钟 |
| 通过率 | **15.0%（6/40）** |
| `citation_completeness` 平均 | **0.287** |
| `llm_judge` 平均 | **0.381** |
| 原始模型输出缓存 | [`docs/evaluation-baseline-invalidation-p2b.json`](evaluation-baseline-invalidation-p2b.json) |

修复后的指标对比：

| 指标 | 修复前 | 修复后 | 变化 |
|------|:------:|:------:|:----:|
| 通过率 | 15.0%（6/40） | 32.5%（13/40） | +17.5% |
| `citation_completeness` | 0.287 | 0.775 | +0.488 |
| `llm_judge` | 0.381 | 0.408 | +0.027 |

修复后的逐条结果见下方「P2B 修复后基线」小节。

### 实时评估（DeepSeek）—— P2B 无效决定书基线（修复 citation 匹配后）

| 项目 | 结果 |
|------|------|
| 运行命令 | `MADY_LIVE_EVAL=1 go test -v -timeout 30m -run TestLiveDeepSeekInvalidationEval ./agentcore/evaluate/benchmark/...` |
| 模型 | `deepseek-chat` |
| 抽样 | 全部 40 道无效决定书案例（使用缓存，无新增 API 调用） |
| 运行时长 | 约 37 秒 |
| 通过率 | **32.5%（13/40）** |
| `citation_completeness` 平均 | **0.775** |
| `llm_judge` 平均 | **0.408** |

| 用例 | 状态 | 平均分 | citation_completeness | llm_judge |
|------|------|:------:|:---------------------:|:---------:|
| `invalidation_decision_001` | ❌ | 0.650 | 1.000 | 0.300 |
| `invalidation_decision_002` | ✅ | 0.950 | 1.000 | 0.900 |
| `invalidation_decision_003` | ✅ | 0.867 | 1.000 | 0.733 |
| `invalidation_decision_004` | ✅ | 0.717 | 1.000 | 0.433 |
| `invalidation_decision_005` | ✅ | 1.000 | 1.000 | 1.000 |
| `invalidation_decision_006` | ❌ | 0.617 | 1.000 | 0.233 |
| `invalidation_decision_007` | ❌ | 0.050 | 0.000 | 0.100 |
| `invalidation_decision_008` | ❌ | 0.000 | 0.000 | 0.000 |
| `invalidation_decision_009` | ❌ | 0.667 | 1.000 | 0.333 |
| `invalidation_decision_010` | ❌ | 0.633 | 1.000 | 0.267 |
| `invalidation_decision_011` | ✅ | 0.950 | 1.000 | 0.900 |
| `invalidation_decision_012` | ❌ | 0.633 | 1.000 | 0.267 |
| `invalidation_decision_013` | ✅ | 0.917 | 1.000 | 0.833 |
| `invalidation_decision_014` | ✅ | 0.917 | 1.000 | 0.833 |
| `invalidation_decision_015` | ❌ | 0.650 | 1.000 | 0.300 |
| `invalidation_decision_016` | ❌ | 0.650 | 1.000 | 0.300 |
| `invalidation_decision_017` | ✅ | 0.950 | 1.000 | 0.900 |
| `invalidation_decision_018` | ❌ | 0.683 | 1.000 | 0.367 |
| `invalidation_decision_019` | ❌ | 0.417 | 0.667 | 0.167 |
| `invalidation_decision_020` | ❌ | 0.617 | 1.000 | 0.233 |
| `invalidation_decision_021` | ❌ | 0.517 | 1.000 | 0.033 |
| `invalidation_decision_022` | ❌ | 0.550 | 0.500 | 0.600 |
| `invalidation_decision_023` | ✅ | 0.733 | 1.000 | 0.467 |
| `invalidation_decision_024` | ✅ | 0.800 | 1.000 | 0.600 |
| `invalidation_decision_025` | ❌ | 0.117 | 0.000 | 0.233 |
| `invalidation_decision_026` | ✅ | 0.950 | 1.000 | 0.900 |
| `invalidation_decision_027` | ❌ | 0.633 | 1.000 | 0.267 |
| `invalidation_decision_028` | ❌ | 0.533 | 0.667 | 0.400 |
| `invalidation_decision_029` | ❌ | 0.050 | 0.000 | 0.100 |
| `invalidation_decision_030` | ❌ | 0.117 | 0.000 | 0.233 |
| `invalidation_decision_031` | ❌ | 0.500 | 1.000 | 0.000 |
| `invalidation_decision_032` | ❌ | 0.500 | 1.000 | 0.000 |
| `invalidation_decision_033` | ✅ | 0.950 | 1.000 | 0.900 |
| `invalidation_decision_034` | ❌ | 0.150 | 0.000 | 0.300 |
| `invalidation_decision_035` | ❌ | 0.583 | 0.667 | 0.500 |
| `invalidation_decision_036` | ❌ | 0.000 | 0.000 | 0.000 |
| `invalidation_decision_037` | ❌ | 0.633 | 1.000 | 0.267 |
| `invalidation_decision_038` | ✅ | 0.783 | 1.000 | 0.567 |
| `invalidation_decision_039` | ❌ | 0.650 | 1.000 | 0.300 |
| `invalidation_decision_040` | ❌ | 0.383 | 0.500 | 0.267 |

**修复效果**：
- `citation_completeness` 从 0.287 提升至 0.775（+0.488），涨幅主要来自于中文数字与阿拉伯数字格式的法条引用现在能正确匹配。
- 通过率从 15.0% 提升至 32.5%（+17.5%），因为大量原本因 citation 字面不匹配而 0 分的用例现在 citation 维度满分。
- `llm_judge` 从 0.381 微升至 0.408（+0.027），涨幅有限，因为该指标不依赖字面引用匹配，而取决于模型输出与 `Expected` 的语义一致性。

## 修复详情

### 问题
`CitationCompleteness` 原实现仅做简单 `strings.Contains`：

```go
if strings.Contains(lower, strings.ToLower(c)) { hit++ }
```

当 `RequiredCitations` 为 `专利法第22条第3款` 而模型输出为 `专利法第二十二条第三款` 时，字面不匹配，导致 citation 维度 0 分。

同时简单子串匹配存在误匹配风险：`第2条` 会被错误地视为 `第22条` 的子串命中。

### 修复方案
在 `agentcore/evaluate/metrics.go` 中改进 `CitationCompleteness.Compute`：

1. **中文数字归一化**：将中文数字（如「二十二」）转换为阿拉伯数字（「22」），使两种写法可相互匹配。
2. **法条引用解析**：用正则提取 `第X条` / `第X条第Y款` 结构化引用，避免子串误匹配。
3. **概括匹配**：当 required 只到「第X条」时，允许命中更具体的「第X条第Y款」。
4. **兼容非数字引用**：对无法解析为法条引用的 citation（如 `CN123`），回退到原始字符串匹配。

### 新增测试

- `TestCitationCompletenessChineseNumerals`：验证「第22条第3款」与「第二十二条第三款」匹配。
- `TestCitationCompletenessNoSubstringMismatch`：验证「第2条」不会误匹配到「第22条第3款」。
- `TestCitationCompletenessParagraphGeneralization`：验证「第22条」能匹配到「第22条第3款」。

### 修改文件

- `agentcore/evaluate/metrics.go`
- `agentcore/evaluate/evaluate_test.go`

## CI 状态

| 检查项 | v0.5 | v0.6（修复前） | v0.6（修复后） |
|--------|:----:|:--------------:|:--------------:|
| `go mod tidy -diff` | ✅ | ✅ | ✅ |
| `go vet ./...` | ✅ | ✅ | ✅ |
| `go build ./...` | ✅ | ✅ | ✅ |
| `go test ./...`（全量） | ✅ 60/60 | ✅ | ✅ |
| `golangci-lint` | ✅ 0 issues | ✅ 0 issues | ⚠️ 未运行（网络超时无法安装） |
| 评估门禁（CI job） | ✅ `eval-benchmark` | ✅ `eval-benchmark` | ✅ `eval-benchmark` |
| LiveEval P2A 基线 | ✅ 2/3 | ✅ 2/3 | ✅ 2/3 |
| LiveEval P2B 基线 | — | ✅ 6/40 | ✅ 13/40 |

## 新增/更新测试

| 模块 | 说明 |
|------|------|
| `agentcore/evaluate/benchmark/invalidation_decisions.go` | 40 件真实无效决定书案例（新增） |
| `agentcore/evaluate/benchmark/suite.go` | 注册 `InvalidationDecisionCases` |
| `agentcore/evaluate/benchmark/live_deepseek_test.go` | 新增 `TestLiveDeepSeekInvalidationEval` |
| `agentcore/evaluate/metrics.go` | 修复 `CitationCompleteness` 中文数字/阿拉伯数字匹配 |
| `agentcore/evaluate/evaluate_test.go` | 新增 citation 匹配测试 |
| `docs/evaluation-baseline-invalidation-p2b.json` | 40 道模型输出原始缓存 |
| `docs/evaluation-baseline-v0.6.md` | 本报告 |

## 下一步

- **P2B 迭代**：继续优化 `Expected` 提取质量，使 LLM 评判更合理；目标将 `llm_judge` 提升至 0.6 以上。
- **P3**：专家盲测 10 个案件，测量人工采纳率/修改率/拒绝率。
- **LiveEval 扩展**：考虑使用更强的模型或加入检索增强，再次评估无效决定书任务。
