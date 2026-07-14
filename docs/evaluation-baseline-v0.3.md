# Mady 评估基线报告 v0.4

> 日期：2026-07-14 | 代码基线：`5a75779`
>
> 上一版本：[v0.3](evaluation-baseline-v0.3.md)（基线：`a9fba93`）

## 变更概要

v0.3 → v0.4 之间完成：
- CI 修复（lint 清零、全量测试通过）
- 证据底座（EvidenceSpan/Case/Approval 状态机）
- 闭环集成（新颖性 LLM 节点/证据包裹生成）
- 打磨（DOCX 导出/CI 评估门禁/TUI 审批）
- 代码审查 15 项修复

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

| 测试 | v0.3 | v0.4 | 变化 |
|------|:----:|:----:|:----:|
| `TestEvalSuite_GoldenPerfect` | ✅ 1.0 | ✅ 1.0 | — |
| `TestEvalSuite_Degraded` | ✅ 0.0 | ✅ 0.0 | — |
| `TestEvalSuite_CaseIntegrity` | ✅ | ✅ | — |
| `TestEvalSuite_DefaultEvaluator` | ✅ | ✅ | — |

**无回归。** 41 个基准案例在所有重构后保持 1.0 PassRate。

## CI 状态

| 检查项 | v0.3 | v0.4 |
|--------|:----:|:----:|
| `go mod tidy -diff` | ✅ | ✅ |
| `go vet ./...` | ✅ | ✅ |
| `go build ./...` | ✅ | ✅ |
| `go test ./...`（全量） | ✅ 58/58 | ✅ 60/60 |
| `golangci-lint` | ⚠️ 无法运行 | ✅ **0 issues**（v2.12.2） |
| 评估门禁（CI job） | — | ✅ `eval-benchmark` |

## 新增测试

| 模块 | 新增测试数 |
|------|:--------:|
| `agentcore/evidence/` | +15 |
| `domains/` | +14 |
| `domains/sqlite/` | +3 |
| `disclosure/` | +10 |
| **合计** | **~45** |

## 下一步

- 公开专利考试真题填充 Golden Set 第一层（10-20 题）
- `MADY_LIVE_EVAL=1` 启用 DeepSeek 实时评估
- 扩展案例至 50+，覆盖更多法条（A2.2/A2.3/R22 等）
