# Phase 4 远期特性 Backlog

> 日期: 2026-07-18 | 汇总所有代码中标注 Phase 4 的待实现项

## 代码标注汇总

| 文件 | 行号 | 内容 | 优先级 |
|------|------|------|--------|
| `domains/reasoning/plan_compiler.go` | 17 | `multi_hypothesis`: dual-advocate + judge subgraph | 高 |
| `domains/reasoning/five_step_runner.go` | 239 | Phase 4: promote to multi_hypothesis intent | 高 |
| `agentcore/evaluate/judge_metrics.go` | 13 | Phase 4+: LLM-based judge (currently heuristic) | 中 |
| `knowledge/eval.go` | 21 | Phase 4+ 接入 LLM 评分器 | 中 |
| `tui/component/statusbar.go` | 38, 148 | Phase 4: enhanced status fields (case name, pending count) | 低 |
| `tui/chat/chat_app_layout.go` | 63, 124 | Phase 4.4: responsive sidebar panel | 低 |

## 建议排期

### multi_hypothesis（双雄辩论 + 法官裁决）

- **依赖**: five_step_runner.go Phase 4 升级
- **价值**: 高——专利无效/创造性分析需要"正反两方辩论 + 法官裁定"模式
- **实现路径**: plan_compiler.go 的 `multi_hypothesis` 子图编译 + `dual-advocate` 策略

### LLM 裁判评估升级

- **依赖**: Golden Benchmark 第二层数据积累（T6）+ 评估预算
- **价值**: 中——启发式 judge 已有 60% 准确率，LLM 裁判可提升到 85%+
- **实现路径**: `DefaultEvaluatorWithLLMJudge` 已提供注入点

### TUI 增强（状态栏 + 侧边栏）

- **依赖**: UI 框架 Phase 4.2 mouse 坐标映射
- **价值**: 低——当前 TUI 功能完整，增强为锦上添花
- **实现路径**: statusbar.go 扩展字段 + chat_app_layout.go 侧边栏

## 不在 Phase 4 范围

以下代码标注虽提及 Phase 4 但已通过其他方式解决：

- `disclosure/report.go` 的 LLM 混合模式 → 已在 Phase 2 (T3) 实现
- `guardrails/citation_source.go` 的 S2 索引 → 已在 Phase 2 (T1) 实现
