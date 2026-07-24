# 专利侵权判定模块实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建 `domains/infringement/` 领域模块，完全替代旧 `workflows/patent/infringement.go`，提供原告/被告双视角 L1-L4 全层次侵权判定引擎

**Architecture:** 参考 `domains/inventiveness/` Pregel 子图模式，10 节点线性拓扑 + Perspective 参数控制视角行为 + YAML 规则引擎 + KnowledgeRetriever 增强 + agentcore.Tool 包装

**Tech Stack:** Go 1.26, Pregel 图引擎, LLM Agent (Temperature=0.1), JSON Schema 约束输出, YAML 规则引擎

**Spec:** `docs/superpowers/specs/2026-07-23-infringement-module-design.md`

**Full Plan:** `/Users/xujian/.claude/plans/users-xujian-library-mobile-documents-i-polymorphic-aho.md`

---

## 任务总览

| 序号 | 任务 | 文件 | 类型 |
|------|------|------|------|
| 1 | 核心类型定义 | `domains/infringement/types.go` | 创建 |
| 2 | 规则引擎 | `domains/infringement/rules.go` | 创建 |
| 3 | 法条框架 | `domains/infringement/framework.go` | 创建 |
| 4 | 知识检索接口 | `domains/infringement/knowledge.go` | 创建 |
| 5 | 评分器 | `domains/infringement/scorer.go` | 创建 |
| 6 | Pregel 图构建 | `domains/infringement/graph.go` | 创建 |
| 7 | LLM Agent 节点 | `domains/infringement/nodes.go` | 创建 |
| 8 | Tool 包装 | `domains/infringement/tool.go` | 创建 |
| 9 | 包文档 | `domains/infringement/doc.go` | 创建 |
| 10 | YAML 规则文件 | `domains/rules/data/` (3个文件) | 创建 |
| 11 | 路由注册 | `domains/router.go` | 修改 |
| 12 | 引用核验扩充 | `guardrails/citation_table.go` | 修改 |
| 13 | 知识库扩充 | `knowledge/store.go` | 修改 |
| 14 | 风格指南 | `styles/patent-standard.yaml` | 修改 |
| 15 | 标记旧代码 | `workflows/patent/infringement.go`, `tool.go` | 修改 |
| 16 | 测试 | `domains/infringement/infringement_test.go` | 创建 |
| 17 | 验证 | `go build ./... && go test ./...` | 验证 |

## 执行顺序

Tasks 1-5 可并行（无依赖），Tasks 6-8 依赖 Task 1（类型），Task 10 独立（YAML），Tasks 11-15 依赖核心模块完成，Task 16 全程并行，Task 17 最后执行。
