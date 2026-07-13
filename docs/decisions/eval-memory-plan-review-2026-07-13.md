# 评估闭环与记忆自学习方案评审报告

**评审日期**: 2026-07-13
**评审对象**: 《评估闭环模块》《记忆自学习模块》《整体阶段划分》三部分方案
**评审依据**: 项目源码（552 Go 文件）、`docs/decisions/AI_CHANGELOG.md`、`docs/adr/006-memory-module.md`、`docs/specs/vector-retrieval/04-tasks.md`、`docs/memory.md`
**评审结论**: **理念合格，落地需大改** — 方向正确但对项目现状掌握严重不足，照原样执行将产生两套并行系统

---

## 目录

1. [总体结论](#1-总体结论)
2. [方案与项目现状的重大脱节](#2-方案与项目现状的重大脱节)
3. [方案中真正有价值、确实缺失的部分](#3-方案中真正有价值确实缺失的部分)
4. [修正后的落地路线](#4-修正后的落地路线)
5. [安全约束与红线](#5-安全约束与红线)
6. [数据获取与成本约束](#6-数据获取与成本约束)
7. [附：评审核实的关键代码位置索引](#附评审核实的关键代码位置索引)

---

## 1. 总体结论

方案在三个方向上体现了成熟的工程判断：

- **评估闭环**的设计（两层 Benchmark、指标体系、反馈→回归用例）符合软件工程质量门控实践
- **记忆自学习**的风险递增分层和"机器提议、人类批准"的中道原则契合项目安全哲学
- **阶段依赖**（D 严格依赖 A+B+C）是整个方案里最清醒的一条

但方案基于**过时的项目快照**，对 `memory/`、`agentcore/evaluate/`、`retrieval/` 三个模块的成熟度严重低估，存在 4 处重大脱节。若按原样执行：

- 评估框架会产生 `EvalCase`/`EvalResult`/`RunEvalSuite` 与现有 `TestCase`/`CaseResult`/`EvaluateBatch` 两套并行系统
- 记忆模块会产生 `LawyerPreference`/`RuleCandidate` 与现有 `MemoryEntry`/`compiler.Strategy` 两套并行系统
- **关键遗漏：方案没有意识到 `memory` 只有 `InMemoryStore` 纯内存实现，若直接把 Tier 1/2 落在这上面，重启后数据会丢失；`FactBlackboard` 虽有完整序列化，但 `StageCheckpoint` 也缺持久化实现**
- 向量检索会重复已完成的工作

**建议**：保留方案的安全设计、指标设计和阶段依赖逻辑，所有"新建"动作改为"扩展已有设施"，删除已完成的向量检索部分；同时把 **memory 持久化** 和 **StageCheckpoint 持久化** 作为前置基础设施显式加入落地路线。

---

## 2. 方案与项目现状的重大脱节

### 2.1 脱节一（最严重）：向量检索 Stage 2/3 已全部完成

方案"阶段 A"将"向量检索 Stage 2/3 落地"列为待办。

**项目实际**：`docs/specs/vector-retrieval/04-tasks.md:6` 首行明确标注：

> 状态：阶段1已完成 ✅ | 阶段2全部完成 ✅ | 阶段3全部完成 ✅ | 端到端验证通过 ✅

`AI_CHANGELOG.md` 记录 2026-07-13（评审当日）完成的工作：

| 阶段 | 内容 | 关键指标 | CHANGELOG 条目 |
|------|------|----------|----------------|
| 阶段1 | SQLite backend 接线（FTS + Vector RRF 融合） | 81K 文档/144K chunks 可检索 | L15-28 |
| 阶段2 | `VectorIndex` 内存全量加载 + Cross-encoder 重排 | 15.2ms < 50ms 预算；87x 加速 | L30-46 |
| 阶段2 T2.5 | Benchmark 基线 | 端到端 29.8ms < 500ms 预算 | L48-61 |
| 阶段3 | `WritableStore` + 三路 RRF + `add_document` 工具 | user.db 写入隔离 | L63-78 |

**处理**：从阶段 A 删除此条。方案作者基于过时快照。

---

### 2.2 脱节二：评估基础设施已存在，方案在重复造轮子

方案 2.3 提议新建 `EvalCase`/`EvalResult`/`RunEvalSuite`。

**项目实际**：`agentcore/evaluate/evaluator.go` 已有完整等价物：

| 方案提议 | 项目已有 | 位置 | 差异 |
|----------|----------|------|------|
| `EvalCase{ID, Input, GoldenAnswer, Domain}` | `TestCase{ID, Input, Expected, RequiredCitations}` | evaluator.go:9 | 仅缺 `Domain` 字段（一行可补） |
| `EvalResult{CaseID, Passed, Score, ActualOutput, Diagnostics}` | `CaseResult{CaseID, Passed, Scores, Average, Prediction}` | evaluator.go:17 | 仅缺 `Diagnostics`（可扩展） |
| `RunEvalSuite(cases, pipeline)` | `EvaluateBatch(ctx, cases, RunFunc)` | evaluator.go:95 | 签名等价 |
| （未提及） | `EvaluateStatic`（golden-file 回归） | evaluator.go:136 | **正好消费方案 2.4 的回归用例** |
| `Metric` 体系 | `Metric` 接口 + `CitationCompleteness` + `LengthScore` | metrics.go | 可直接实现新 Metric |

更关键：`knowledge/eval.go:22` 已有 `EvalHook`（lifecycle hook），实现 `Faithfulness`/`AnswerRelevancy`/`ContextPrecision` 启发式评分，注释明确写：

> Phase 3 实现启发式评分；Phase 4+ 将接入 LLM 评分器。（knowledge/eval.go:21）

这正是方案 2.5 步骤3（LLM 裁判 Prompt）想做的事，且已排期。

**处理**：扩展 `Evaluator`（补 `Domain` 字段、新增 LLM 裁判 `Metric` 实现、扩展 `Diagnostics`），不新建并行框架。

---

### 2.3 脱节三：记忆模块已有抽象，但持久化层缺失

方案 3.1 提出"三层记忆模型（Tier 1/2/3）"，并认为"技术上大部分已就位"。

**项目实际**：`memory/` 包确实已有抽象层，但持久化层存在明显缺口：

| 方案 Tier | 风险 | 项目已有抽象 | 项目实际缺失 | 位置 |
|-----------|------|--------------|--------------|------|
| Tier 1 案件记忆 | 低 | `LayerSession` + `FactBlackboard` | `memory` 无持久化；`StageCheckpoint` 只有内存版 `MemoryCheckpointStore` | memory/types.go:48; domains/reasoning/fact_blackboard.go:12; domains/reasoning/checkpoint.go:36 |
| Tier 2 用户偏好 | 中 | `LayerUser` + 四维 `MemoryScope{UserID,AgentID,SessionID,ProjectID}` | `MemoryStore` 仅 `InMemoryStore` 纯内存实现 | memory/types.go:27,47; memory/store.go:14-16 |
| Tier 3 规则蒸馏 | 高 | `memory/compiler/`：`Compiler` + `Strategy` + `ExecutionTrace` + ε-greedy 探索 + 成功率统计 | 从策略学习桥接到 `rule_engine` 的 `CheckRule` 候选生成尚未实现 | memory/compiler/learning.go:12 |

**关键细节**：

- `FactBlackboard` 已实现完整 JSON 序列化/反序列化（fact_blackboard.go:322-372），具备被持久化的能力。
- `domains/reasoning/checkpoint.go` 已实现 `StageCheckpoint` 的 `Save`/`Load`/`ResumeFromCheckpoint`/`ContinueFromStage`（checkpoint.go:28-122），"案件记忆预热"机制的核心骨架已存在。
- 但 `CheckpointStore` 目前只有 `MemoryCheckpointStore`（checkpoint.go:36）——内存实现，重启后丢失。
- `memory/store.go:14-16` 明确写明 `InMemoryStore` 是"Phase 1 纯内存实现，无外部依赖"。

方案 3.3 提议新建 `LawyerPreference` 结构。项目已有更通用的 `MemoryEntry`（memory/types.go:72），支持 `Remember`/`Recall`/`Forget`/`Update`/`List` 全套操作（memory/types.go:216-256），用户可查看编辑的能力已具备。但**只要没有持久化后端，这些偏好就无法跨重启保留**。

方案 3.4 的 `RuleCandidate{TriggerPattern, SuggestedChange, SupportingCases, SampleSize}` 与项目 `compiler.Strategy`（含 ID/Description/Guidance/Successes/Failures/LastUsedAt）高度重合。差异在于：

- 项目 `compiler` 学习的是**提示策略**（guidance 注入上下文），见 learning.go:81
- 方案说的是 **CheckRule 规则调整**（`workflows/patent/rule_engine.go:94` 的 `RuleEngine`）

这是两个不同层面的学习，但 `ExecutionTrace` + 统计 + 候选机制可复用。

**处理**：
- Tier 1 案件记忆：基于 `domains/reasoning/checkpoint.go` 的 `StageCheckpoint` + `FactBlackboard` 序列化，实现持久化 `CheckpointStore`（文件或 SQLite），而非基于 `memory` Session 层
- Tier 2 用户偏好：先用 `MemoryEntry` + `MemoryScope{UserID}` 存储，但**必须同步实现 `MemoryStore` 的持久化后端**（如 SQLite），否则无法跨重启保留
- Tier 3 扩展 `memory/compiler/` 的 trace 机制，桥接到 `rule_engine` 的候选生成（需独立技术预研）

---

### 2.4 脱节四：Checkpoint 概念混淆

方案多处使用"Checkpoint 反馈"指代人工复核环节。

**项目实际**：`graph/checkpoint.go:14` 的 `Checkpoint` 是**图执行状态快照**（GraphID/NodeName/StepIndex/State），用于 Pregel 图的断点恢复，与人工复核无关。

方案实际指的是 `domains/approval.go:29` 的 `ApprovalGate`（lifecycle hook），已通过 TUI `/review` 命令接入（见 `AI_CHANGELOG.md` L103-111）。但 `ApprovalGate.AfterModelCall`（domains/approval.go:76）目前只做 `Agent.Steer` 注入审批提示，**确实没有结构化留痕**——这是真实缺口。

**处理**：术语纠正为"ApprovalGate 留痕"；留痕机制是真实新工作，需新建。

---

## 3. 方案中真正有价值、确实缺失的部分

剥离重复建设后，方案的真正贡献是以下**项目目前确为空白**的点：

1. **Golden Benchmark 第一层数据集**（专利代理人资格考试真题）— `evaluate` 框架有了，但没有真实用例数据集填充。"有引擎没燃料"。

2. **三项缺失的评估指标**：
   - Judge 一致性（AI 裁决与人工最终结论一致率，针对 `multi_hypothesis` 策略）
   - 护栏漏报率（高风险输出假阴性）
   - 人工复核采纳率（全部采纳/部分修改/拒绝比例）

   `EvalHook`（knowledge/eval.go:68）目前只测了 Faithfulness/AnswerRelevancy/ContextPrecision，上述三项均无。

3. **"漏报率优先于误报率"的权重设计** — 正确且重要。护栏"过于严格多问几句"的代价远小于"该拦的没拦住"。契合 `AGENTS.md` 护栏等级（`guardrails.Level`）不可降的安全红线。

4. **ApprovalGate 结构化留痕**（采纳/修改/拒绝 + diff） — 真实缺口，且是"反馈→回归用例"和 Tier 3 候选生成的共同前置依赖。

5. **半自动"反馈→回归用例"转化流程**（候选草稿 + 人工确认入库） — 正确的工程实践。`EvaluateStatic`（evaluator.go:136）正好能消费这些回归用例。

6. **Tier 3 安全约束设计**：样本量过滤、人工批准、晋升前影子评估、AI_CHANGELOG 留痕 — 完全契合项目"中道"精神，方向无可挑剔。

7. **阶段 D 严格依赖 A+B+C** — 没有评估闭环，候选规则无法验证是真改进还是过拟合；没有 Checkpoint 反馈数据，候选生成本身无米之炊。这条依赖不能打破。

---

## 4. 修正后的落地路线

### 阶段 A（可立即并行，互不依赖）

| 任务 | 动作 | 对接设施 | 方案原条目 |
|------|------|----------|------------|
| A1 | ~~向量检索 Stage 2/3~~ | **删除**（已完成） | 原阶段 A 第一条 |
| A2 | Golden Benchmark 第一层：用考试真题填充 `TestCase` 数据集 | `agentcore/evaluate.TestCase`（evaluator.go:9）+ `EvaluateBatch`（evaluator.go:95） | 2.5 步骤1-2 |
| A3 | 评估指标体系设计：新增 `JudgeConsistency` Metric；定义 `GuardrailFalseNegative`/`AdoptionRate` 聚合指标 | `Metric` 接口（metrics.go:10）。LLM 裁判引入额外模型调用成本，需设置预算和抽样比例 | 2.2 指标体系 / 2.5 步骤3 |
| A4 | CI 化：Prompt/Rule/Skill 改动前跑 Eval Suite | 复用现有 `go test` CI 门禁 + `EvaluateBatch` | 2.5 步骤6（提前） |
| A5 | **新增基础设施**：`MemoryStore` 持久化后端实现（如 SQLite） | 实现 `MemoryStore` 接口（types.go:216）；为 `LayerUser`/`LayerSession` 提供跨重启持久化 | 方案原未明确 |
| A6 | **新增基础设施**：`CheckpointStore` 持久化实现（文件或 SQLite） | 实现 `CheckpointStore` 接口（domains/reasoning/checkpoint.go:29）；让 `StageCheckpoint` + `FactBlackboard` 可持久化 | 方案原未明确 |

### 阶段 B（依赖 A 基础设施就绪，特别是 A5/A6 持久化）

| 任务 | 动作 | 对接设施 | 方案原条目 |
|------|------|----------|------------|
| B1 | ApprovalGate 留痕机制 | 扩展 `domains/approval.go:76` 的 `AfterModelCall`，记录采纳/修改/拒绝 + diff，写入结构化存储 | 2.5 步骤4 / 3.5 步骤3 |
| B2 | Tier 1 案件记忆预热 | 基于 `domains/reasoning/checkpoint.go:88` 的 `ResumeFromCheckpoint` + `FactBlackboard` 序列化，从持久化 `CheckpointStore` 恢复；非全量历史重放 | 3.2 / 3.5 步骤1 |
| B3 | LLM 裁判 Metric + 抽样人工校准 | 实现 `JudgeConsistency` Metric，对接 `knowledge/eval.go` Phase 4 排期；低置信度/边界案例人工复核 | 2.5 步骤3 |

### 阶段 C（依赖 B 积累真实反馈数据）

| 任务 | 动作 | 对接设施 | 方案原条目 |
|------|------|----------|------------|
| C1 | Golden Benchmark 第二层：从 ApprovalGate 留痕半自动转化回归用例 | `EvaluateStatic`（evaluator.go:136）消费；候选草稿 + 人工确认入库 | 2.4 / 2.5 步骤5 |
| C2 | Tier 2 用户偏好：用 `memory` LayerUser 持久化存储 | `MemoryEntry`（types.go:72）+ `MemoryScope{UserID}`（types.go:27）+ `Remember`/`Recall`/`Forget` 工具；依赖 A5 的 SQLite 持久化后端；**不新建 `LawyerPreference` 类型** | 3.3 / 3.5 步骤2 |

### 阶段 D（依赖 A+B+C 全部就绪，是严格前置门槛；D1 需先独立技术预研）

| 任务 | 动作 | 对接设施 | 方案原条目 |
|------|------|----------|------------|
| D1 | Tier 3 候选生成（月度批量分析） | 扩展 `memory/compiler/` 的 `ExecutionTrace`（learning.go:89）+ `Strategy` 统计，桥接到 `workflows/patent/rule_engine.go:94` 的 `CheckRule` 候选 | 3.4 / 3.5 步骤4 |
| D2 | 人工审查队列 + 晋升前影子评估 | 候选规则在 Golden Benchmark（尤其第二层）上跑 `EvaluateBatch` 影子测试 | 3.5 步骤5 |
| D3 | 晋升流程上线 + AI_CHANGELOG 记录 | 复用现有 `docs/decisions/AI_CHANGELOG.md` 机制，不另建一套 | 3.5 步骤6 |

**D1 风险说明**：`memory/compiler` 的 `Strategy` 是"提示策略"（learning.go:81），而 `rule_engine` 的 `CheckRule` 是法律判断规则，两者抽象层级不同。D1 应先做独立技术预研和小范围 prototype，验证从策略学习到规则候选生成的映射可行性，再接入生产 rule_engine。

**依赖关系**：

```
阶段 A（A2/A3/A4/A5/A6 并行，其中 A5/A6 是 B2/C2 的前置）
   │
   └─→ 阶段 B（B1/B2/B3 并行）
          │
          └─→ 阶段 C（C1/C2 并行）
                 │
                 └─→ 阶段 D（D1 先做技术预研 → D2 → D3 串行）
```

阶段 D 不能提前；阶段 A 中的 A5/A6 持久化基础设施是 B2/C2 的隐藏前置，必须在前面补齐。

---

## 5. 安全约束与红线

以下约束照方案执行，不得放松（对应 `AGENTS.md` 安全红线）：

### 5.1 护栏不可降

- `RiskTolerance`（用户偏好）**只影响措辞保守程度，绝不影响 `guardrails.Level` 本身**
- 律师个人偏好不能成为降低护栏严格度的理由
- 评估权重设计体现"漏报率优先于误报率"的不对称性

### 5.2 Tier 3 永不全自动

- 样本量太小的模式不生成候选（建议 ≥5 次同类修正才进入候选队列）
- 候选规则**从不自动生效**，必须进入人工审查队列
- 晋升前必须过一次评估闭环（Golden Benchmark 影子测试）
- 每次晋升写入 `AI_CHANGELOG.md`，记录 `SupportingCases` 和批准人

### 5.3 隐私边界

- Golden Benchmark 第二层真实案例必须严格脱敏
- 对应 Workspace/Project 设计的隐私边界（`util.MadyHome()` 隔离）
- Tier 2 用户偏好存储在 `MemoryScope{UserID}` 隔离域内，不跨用户泄漏

### 5.4 概念澄清

- 文档及代码中"Checkpoint 反馈"统一纠正为"ApprovalGate 留痕"
- `graph/checkpoint.go` 的 `Checkpoint`（图状态快照）与人工复核无关，不得混用术语

---

## 6. 数据获取与成本约束

以下约束在方案中被低估，需作为落地前提：

### 6.1 第一层 Golden Benchmark 数据可行性

- 专利代理人资格考试真题需确认**版权/公开可用性**和**格式统一性**
- 若真题受版权限制或格式难以批量处理，备选方案：自建 50-100 道模拟题作为 MVP 数据集，确保阶段 A 的 Eval Suite 能跑起来

### 6.2 LLM 裁判成本

- 方案 2.5 步骤3 的"LLM 裁判 + 抽样人工校准"会引入额外模型调用成本
- 建议初期：LLM 裁判做粗筛，人工只复核低置信度/边界案例；设置每月评估预算上限和抽样比例

---

## 附：评审核实的关键代码位置索引

| 模块 | 关键符号 | 位置 |
|------|----------|------|
| 评估框架 | `Evaluator` / `TestCase` / `EvaluateBatch` / `EvaluateStatic` | agentcore/evaluate/evaluator.go:39,9,95,136 |
| 评估指标 | `Metric` 接口 / `CitationCompleteness` | agentcore/evaluate/metrics.go:10,136 |
| 评估 Hook | `EvalHook` / `EvalConfig` / `EvalResult` | knowledge/eval.go:22,29,83 |
| 记忆类型 | `MemoryScope` / `MemoryLayer` / `MemoryEntry` / `MemoryStore` | memory/types.go:27,44,72,216 |
| 内存记忆 | `InMemoryStore`（Phase 1 纯内存，无持久化） | memory/store.go:14-16 |
| 记忆学习 | `Compiler` / `Strategy` / `ExecutionTrace` / `FinishTurn` | memory/compiler/learning.go:12,89 |
| 事实黑板 | `FactBlackboard` + `MarshalJSON` / `UnmarshalJSON` | domains/reasoning/fact_blackboard.go:12,322,345 |
| 工作流检查点 | `StageCheckpoint` / `CheckpointStore` / `ResumeFromCheckpoint` / `ContinueFromStage` | domains/reasoning/checkpoint.go:18,29,88,113 |
| 审批门 | `ApprovalGate` / `AfterModelCall` / `needsApproval` | domains/approval.go:29,76,101 |
| 护栏 | `guardrail` / `AfterModelCall` / `SuppressPersist` | guardrails/levels.go:85,90 |
| 规则引擎 | `RuleEngine` / `Evaluate` / `CheckRule` | workflows/patent/rule_engine.go:94,138 |
| 向量检索 | `VectorIndex` / `WritableStore` / `ModelReranker` | knowledge/sqlite/vector_index.go; knowledge/sqlite/writable.go; retrieval/model_rerank.go |
| 图快照 | `Checkpoint`（注意：非人工复核） | graph/checkpoint.go:14 |
