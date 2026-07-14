# Mady 评估基线报告 v0.5

> 日期：2026-07-14 | 代码基线：`5a75779`
>
> 上一版本：[v0.4](evaluation-baseline-v0.3.md)（基线：`5a75779`）

## 变更概要

v0.4 → v0.5 之间完成 P2A 阶段：
- 31 道公开专利考试真题已按法条归类并集成到 Golden Set 第一层
- 静态评估门禁持续全绿
- 首次运行 DeepSeek 实时评估，建立 LLM 基线

## 基准集

| 基准文件 | 案例数 | 法条 | 描述 |
|----------|:------:|------|------|
| `patent_exam.go` | 10 | 综合 | 模拟专利审查案例 |
| `patent_exam_real_a2.go` | 3 | A2 | 保护客体/技术领域分析 |
| `patent_exam_real_a22.go` | 15 | A22 | 新颖性/创造性/实用性分析 |
| `patent_exam_real_a26.go` | 3 | A26 | 充分公开/支持/清楚分析 |
| `patent_exam_real_a31.go` | 8 | A31 | 单一性/合案/分案分析 |
| `patent_exam_real_a33.go` | 1 | A33 | 修改超范围分析 |
| `patent_exam_real_r42.go` | 1 | R42 | 分案申请程序分析 |
| **合计** | **41** | — | 全部域为 `patent`；真题 31 道，模拟题 10 道 |

**Golden Set 第一层**：31 道 2007-2019 年全国专利代理人资格考试《专利代理实务》真题，按专利法/实施细则条款归类为 A2/A22/A26/A31/A33/R42 六组。

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

| 测试 | v0.4 | v0.5 | 变化 |
|------|:----:|:----:|:----:|
| `TestEvalSuite_GoldenPerfect` | ✅ 1.0 | ✅ 1.0 | — |
| `TestEvalSuite_Degraded` | ✅ 0.0 | ✅ 0.0 | — |
| `TestEvalSuite_CaseIntegrity` | ✅ | ✅ | — |
| `TestEvalSuite_DefaultEvaluator` | ✅ | ✅ | — |

**无回归。** 41 个基准案例保持 1.0 PassRate。

### 实时评估（DeepSeek）

| 项目 | 结果 |
|------|------|
| 运行命令 | `MADY_LIVE_EVAL=1 go test -v -timeout 30m -run TestLiveDeepSeekEval ./agentcore/evaluate/benchmark/...` |
| 模型 | `deepseek-chat` |
| 抽样 | 随机 3 题（真题） |
| 通过率 | **66.7%（2/3）** |
| `citation_completeness` 平均 | **1.000** |
| `llm_judge` 平均 | **0.456** |

| 用例 | 状态 | 平均分 | citation_completeness | llm_judge |
|------|------|:------:|:---------------------:|:---------:|
| `patent_exam_2011_a22_01` | ✅ | 0.733 | 1.000 | 0.467 |
| `patent_exam_2008_r42_01` | ❌ | 0.650 | 1.000 | 0.300 |
| `patent_exam_2008_a26_03` | ✅ | 0.800 | 1.000 | 0.600 |

**关键发现**：
- 所有用例 `citation_completeness` 均为 1.0，说明 LLM 能在答案中引用题目要求的法条。
- `llm_judge` 分数偏低，表明 LLM 答案在法律结论准确性、推理深度或引用质量上仍有提升空间。
- 通过率的随机性较大（3 题样本），建议后续扩大样本量至 10 题以上以获得稳定基线。

## CI 状态

| 检查项 | v0.4 | v0.5 |
|--------|:----:|:----:|
| `go mod tidy -diff` | ✅ | ✅ |
| `go vet ./...` | ✅ | ✅ |
| `go build ./...` | ✅ | ✅ |
| `go test ./...`（全量） | ✅ 60/60 | ✅ 60/60 |
| `golangci-lint` | ✅ 0 issues | ✅ 0 issues |
| 评估门禁（CI job） | ✅ `eval-benchmark` | ✅ `eval-benchmark` |
| LiveEval 基线 | — | ✅ 2/3 |

## 新增/更新测试

| 模块 | 说明 |
|------|------|
| `agentcore/evaluate/benchmark/` | 31 道真题按法条归类（6 文件） |
| `agentcore/evaluate/benchmark/live_deepseek_test.go` | DeepSeek 实时评估测试 |

## 下一步

- P2B：Golden Set 第二层 — 3-5 件代理人合作脱敏案件
- P3：专家盲测 10 个案件，测量人工采纳率/修改率/拒绝率
- 扩展 LiveEval 样本量至 10 题以上，降低随机波动
- 针对 `llm_judge` 低分用例进行 Prompt/工作流优化
