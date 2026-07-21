# 设计文档索引

> Mady 的设计文档分布在多个目录中，本文件提供统一索引。

## 架构决策记录 (ADR)

位于 `docs/adr/`，采用 ADR 标准格式：

| ADR | 标题 | 状态 |
|-----|------|------|
| 0001 | 采用分层架构 | Accepted |
| 0002 | 图引擎设计（DAG + Pregel） | Accepted |
| 0006 | 记忆模块设计 | Accepted |
| 0007 | 上下文组装器设计 | Accepted |

## 核心设计文档

位于 `docs/` 根目录：

| 文档 | 内容 |
|------|------|
| `docs/chat-assistant-architecture.md` | Chat/Assistant 分离的架构决策，包含领域配置矩阵和 Handoff 设计 |
| `docs/tone-style-guide.md` | 面向用户文案的措辞规范（禁用词表、护栏文案、报告约定） |
| `docs/knowledge.md` | 知识管理架构（触发策略、RAGAS 评估） |
| `docs/memory.md` | 长期记忆系统设计（三层模型、四维隔离、复合评分） |
| `docs/context_builder.md` | 统一上下文组装器（5 层构建、Static/Dynamic 分离） |
| `docs/workflows.md` | 专业领域工作流（交底书分析、新颖性分析、案例比较） |
| `docs/manifest-guide.md` | Agent Manifest JSON 注册指南 |

## Superpowers 生成的规范文档

位于 `docs/superpowers/`：

| 文档 | 内容 |
|------|------|
| `docs/superpowers/specs/2026-07-10-nuochat-psychological-engine-design.md` | 心理引擎设计规范 |
| `docs/superpowers/plans/2026-07-10-nuochat-psychological-engine-plan.md` | 心理引擎实施计划 |

## 开发流程文档

位于 `docs/specs/` 和 `docs/decisions/`：

| 文档 | 内容 |
|------|------|
| `docs/specs/README.md` | Spec-Driven 开发流程（四阶段文档链） |
| `docs/decisions/AI_CHANGELOG.md` | AI 决策变更日志 |

## 当前设计草案

位于 `docs/design/`：

| 文档 | 内容 |
|------|------|
| `docs/design/tui-overlay-optimization-v0.1.md` | 现有单主视图 TUI + 浮层优化方案，强调当前判断主轴、浮层分类、复核门与系统态设计 |
| `docs/design/p3-blind-test-plan.md` | P3 专家盲测总方案，覆盖目标、指标、案件来源、流程、数据采集与通过线 |
| `docs/design/p3-blind-test-case-jiaoche-daizhou.md` | “绞车带轴”单案盲测用例卡，固化输入材料、专家任务书、评分表与单案通过标准 |
