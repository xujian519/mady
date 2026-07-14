# Mady 评估基线报告 v0.3

> 日期：2026-07-14 | 审阅基线：`a9fba93`

## 概述

本项目首次正式评估基线测量。评估框架 `agentcore/evaluate/` 使用静态回放（`EvaluateStatic`）：预测 = 参考答案时，所有指标应为 1.0。

## 基准集

| 基准文件 | 案例数 | 法条 | 描述 |
|----------|:------:|------|------|
| `patent_exam.go` | 10 | 综合 | 模拟专利审查案例 |
| `patent_exam_real_a2.go` | 3 | A2 | 技术领域/技术方案分析 |
| `patent_exam_real_a22.go` | 15 | A22 | 新颖性/创造性分析 |
| `patent_exam_real_a26.go` | 3 | A26 | 支持/实施分析 |
| `patent_exam_real_a31.go` | 8 | A31 | 特定新颖性分析 |
| `patent_exam_real_a33.go` | 1 | A33 | 修改分析 |
| `patent_exam_real_r42.go` | 1 | R42 | 单一性分析 |
| **合计** | **41** | — | 全部域为 `patent` |

## 指标

| 指标 | 说明 |
|------|------|
| `F1Score` | Token 重叠 |
| `KeywordRecall` | 参考关键词在预测中的比例 |
| `CitationCompleteness` | 必需引用的比例 |
| `JudgeConsistency` | 启发式关键词一致性 |

## 基线分数

| 测试 | 结果 |
|------|:----:|
| `TestEvalSuite_GoldenPerfect` | ✅ PASS（PassRate=1.0） |
| `TestEvalSuite_Degraded` | ✅ PASS（空预测 PassRate=0） |
| `TestEvalSuite_CaseIntegrity` | ✅ PASS（41 案例均完整） |
| `TestEvalSuite_DefaultEvaluator` | ✅ PASS |

**结论：评估框架正常工作，41 个案例结构完整。** 当前基线作为后续所有变更的回归基准。

## CI 状态

| 检查项 | 状态 |
|--------|:----:|
| `go mod tidy -diff` | ✅ 干净 |
| `go vet ./...` | ✅ 干净 |
| `go build ./...` | ✅ 干净 |
| `go test ./...`（全量） | ✅ 58/58 包通过 |
| `golangci-lint` | ⚠️ 无法运行（本地代理限制，需 CI 确认） |

## 下一步

- 在 CI 中增加基准回归门禁（`go test ./agentcore/evaluate/benchmark/ -run TestEvalSuite_...`）
- 增加 live LLM 评估（`MADY_LIVE_EVAL=1`）
- 逐步扩展案例至 50+ 覆盖更多法条场景
