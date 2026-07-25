# Mady 全面审阅 Phase 5 总结报告 — 2026-07-25

> 依据：`Mady 全面审阅计划 v1.0`
> 执行者：AI（Grok）
> Human Owner：[NEEDS CLARIFICATION]

## 执行摘要

本次审阅按计划完成全部 6 个阶段：

| 阶段 | 范围 | 产出 |
|------|------|------|
| Phase 0 | 基线收集 | `docs/review/2026-07-25-phase0-baseline.md` |
| Phase 1 | P0 急诊验证 | `docs/review/2026-07-25-phase1-p0-triage.md` |
| Phase 2 | 未审业务核心深审（R6-R9） | 4 份报告 |
| Phase 3 | 新增代码审阅（R10-R15） | 6 份报告 |
| Phase 4 | 数据隐私专项（R16-R17） | 1 份报告 |
| Phase 5 | 收敛与 roadmap | `docs/decisions/phase4-backlog.md` + 5 条 AI_CHANGELOG + 本报告 |

**代码改动量：0。** 本次为 read-only 审阅，未修改任何源码；所有发现均已落入 backlog 等待人工 owner 指派后实施。

## 关键结论

### 1. 基线健康

- `make verify` 全绿：lint 0 问题、`go test -race` 全模块通过、golangci-lint 0 问题。
- govulncheck 无代码漏洞；gitleaks 无硬编码密钥（Phase 0 基线）。
- 历史 5 个 P0 已在 HEAD 修复，无遗留急诊。

### 2. 颠覆性发现

本次审阅定位到 2 个对项目健康度影响最大的结论：

| 发现 | 位置 | 影响 |
|------|------|------|
| **evidence 模块当前为零外部引用的孤立模块** | `domains/evidence/` | 在仓库快照中零外部引用，但负责人正在其他窗口集成；需待集成完成后再评估是否删除或重构 |
| **psychological 仅实现 VAD** | `psychological/` | 文档/设计声称的 OCC/EMA/SDT/CBT 四模型在代码中不存在 |

这两个问题本质是**“设计与实现严重脱节”**，建议优先清理/修正，避免后续开发基于错误假设。

### 3. 数据隐私是最大风险聚集区

Phase 4 发现：
- **22 条 LLM outbound 路径**中，仅 memory/extractor 对凭证做了有限过滤，其余均无 PII 脱敏。
- **memory 跨项目泄漏**：全局单库 + 查询只按 `UserID` 过滤。
- **tasklist 全局目录**：顺序数字 ID，workspace 之间可互相读写任务。

这三项直接触及专利/法律场景的数据安全底线，建议作为 P0 立即修复。

### 4. 新增代码引入真实攻击面

- **PDF Chrome 渲染存在 HTML 注入**：LLM 生成的变量经 `gmhtml.WithUnsafe()` 透传进 headless Chrome。
- **OrchestrationExecutor 无递归深度限制**：重复历史 handoff #P1 的栈溢出模式。

### 5. 规范与实践脱节

- AI_CHANGELOG 135 条记录中仅 0.7% 含 Human Owner，0% 含 Risk，100% 使用与 CONTRIBUTING.md 不符的四段式格式。
- 本次已按“路径 A”修订建议，在保留四段式的同时强制追加 5 字段占位。

## 产出清单

### 审阅报告（10 份）

1. `docs/review/2026-07-25-phase0-baseline.md`
2. `docs/review/2026-07-25-phase1-p0-triage.md`
3. `docs/review/2026-07-25-phase2-inventiveness.md`
4. `docs/review/2026-07-25-phase2-enablement.md`
5. `docs/review/2026-07-25-phase2-evidence.md`
6. `docs/review/2026-07-25-phase2-psychological.md`
7. `docs/review/2026-07-25-phase3-orchestration-tasklist.md`
8. `docs/review/2026-07-25-phase3-tui-pdf.md`
9. `docs/review/2026-07-25-phase3-mcp.md`
10. `docs/review/2026-07-25-phase3-changelog.md`
11. `docs/review/2026-07-25-phase4-privacy-isolation.md`
12. `docs/review/2026-07-25-phase5-summary.md`（本文件）

### 统一 Backlog

- `docs/decisions/phase4-backlog.md`
- 共 35 项可执行任务，总估算工时 **24.0 人日**
- 风险分布：Critical 1 项、High 20 项、Medium 12 项、Low 4 项

### AI_CHANGELOG 记录

本次按 D5 决策追加 5 条审阅决策记录（非完成功能改动，标记为“审阅决策/待实施”）：

1. LLM outbound 统一 PII redaction 层
2. memory 与 tasklist 按 workspace 隔离
3. evidence 模块死代码处置
4. AI_CHANGELOG 格式修订
5. OrchestrationExecutor 递归深度限制

> 说明：因本次审阅未修改源码，严格按 AGENTS.md 无需 AI_CHANGELOG；但为落实已批准计划 D5，仍记录 5 条关键审阅决策。Human Owner 均标记为 [NEEDS CLARIFICATION]。

## 优先级建议

### P0 — 建议 1-2 天内启动

1. **R17-02** memory 查询按 ProjectID + SessionID 过滤
2. **R17-11** tasklist 按 workspace 分目录
3. **R16-03** Session Summarizer 接入 redactor

### P1 — 建议 1-2 周内完成

- R8-L2-1 evidence 死代码删除
- R8-L3-1 证明标准矛盾统一
- R9-F1/F2/F4 psychological 公式/死代码修复
- R6-F01/R7-F01/R7-F02/R7-F03 业务核心正确性修复
- R13-1 PDF HTML 注入防护
- R10-F1 Orchestration 递归深度限制
- R16-15 Context Compaction redaction

### P2 — 建议排入下个 Sprint

- R16-14 Agent 主循环统一 redaction
- R16-06/11 Embedding 链路 redaction
- R15 AI_CHANGELOG 格式修订
- R14-1/2 MCP TOCTOU 与 owner 测试
- R17-05/R17-08 session/filecheckpoint 隔离改进

### P3 — 长期债务

- R6-F02 inventiveness nodes.go 拆分
- 低优先级文档/一致性问题

## 方法学与质量声明

- 所有结论均要求带文件行号证据；颠覆性结论（死代码、VAD-only 等）均经过主审 grep/read_file 复核。
- Phase 2/3/4 部分模块使用 code-reviewer 子代理并行探索，主审对 H/C 级发现抽样验证。
- 未修改任何源代码，因此未触发构建/测试验证（与本次审阅范围一致）。

## 下一步动作

1. **人工 Owner 指派**：为 P0/P1 项分配负责人。
2. **创建 Spec/任务单**：对 redaction 层、workspace 隔离、evidence 清理等较大改动走 Spec-Driven 四步。
3. **执行并验证**：按“先读后改、改后验证”原则逐个修复，更新 `docs/decisions/AI_CHANGELOG.md`。
4. **关闭本次审阅**：当 P0 项全部修复并验证后，可在 Phase 5 总结报告中勾选完成。

---

> 本报告为 `Mady 全面审阅计划 v1.0` 的最终交付物。所有相关报告、backlog、AI_CHANGELOG 记录均已落盘。
