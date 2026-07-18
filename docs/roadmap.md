# Mady 开发路线图

> 更新日期：2026-07-18 | 代码基线：`a8730ca`
>
> 基于 Manus AI 路线图审阅 + 项目代码深度验证 + 7-9 月执行反馈 + 07-17 人机协作路线图共识 + Open Design 三阶段引入完成

---

## 一、产品定位

**Mady = 证据驱动的专利案件工作台**

**北极星**：每个专业工作成果中，经人工确认、可定位到权威来源且无需结构性重做的证据化结论数量 ÷ 专业人员投入时间。

**首要用户旅程**：技术交底书 → 技术特征树 → 现有技术证据包 → 新颖性初评 → 人工复核报告 → 导出

## 二、模块成熟度（代码验证后修正 — 2026-07-18）

| 模块 | 路线图原评分 | 修正评分 | 关键事实 |
|------|:---------:|:------:|------|
| 知识/检索 | 3.5 | **5.0** | FTS5 trigram BM25 + 144K 向量并行 + 三路 RRF + cross-encoder |
| 评估框架 | 2.0 | **5.0** | 6 套专利法条基准 + LLM Judge + RAG 评估 + CLI 引擎 |
| 推理引擎 | — | **5.0** | FactBlackboard + Syllogism + 多假设 + 五阶段引擎 + 策略路由 |
| Agent 运行时 | 4.0 | **5.0** | 生命周期/事件/压缩/Handoff/流式/结构化输出/DoomLoop/Atom/Plugin |
| TUI | 3.5 | **4.5** | 8 层 Elm 架构，斜杠命令完整，流式 11× 提速，品牌主题 |
| 引用核验 | — | **4.5** | S1 静态表 + S2 法条索引双级核验，82 条全覆盖，7 层回放零误报 |
| 工程 CI/CD | 3.0 | **4.0** | golangci-lint v2 + 多平台矩阵 + Codecov + eval 门禁 |
| 工作流 | 4.0 | **4.0** | Pipeline/Parallel/Router + PER 模式 + Pipeline Atoms |
| 状态持久化 | 3.0 | **4.0** | SQLiteCheckpoint + SQLiteMemory + SQLiteApproval |
| 护栏安全 | 1.5 | **4.0** | 两层防御 + 熔断器 + fails-closed + 引用核验 Gate |
| 人机审批 | 2.0 | **3.5** | ApprovalState 状态机 + SQLite 持久化 + /approve /reject + RecordDecision 留痕 + review_gate 主动中断 |
| 专业工作流 | 2.5 | **4.0** | disclosure 11 节点管线 + 五步工作法 + 确认阀闭环 + 插件系统 |
| 证据/引用 | 2.0 | **3.5** | EvidenceSpan + ClaimBinding + ConflictDetector + Receipt/Ledger + EvidenceGroundedness 指标 |
| 产品可用性 | 1.5 | **3.5** | DOCX/Markdown 导出 + TUI 审批流程 + 品牌主题/命令系统/设置持久化/键位配置/文档模板 |
| 协议安全 | — | **3.5** | ACP 认证 / A2A Origin 校验 / MCP 配置信任 / TLS / 速率限制 / Agent 池引用计数 |

## 三、当前执行状态（2026-07-18）

### ✅ 全部完成的里程碑

| 里程碑 | 状态 | 说明 |
|--------|:----:|------|
| P0 止血（CI/评估基线/功能冻结） | ✅ | 完成 |
| P1A 证据底座 | ✅ | EvidenceSpan / Case / Approval / SQLiteStore |
| P1B 闭环集成 | ✅ | 新颖性初判节点 / 证据包裹 / e2e 测试 |
| P1C 打磨（导出/CI 门禁/TUI 审批） | ✅ | 完成 |
| P2A 真题 Golden Set + 基线 | ✅ | 31 题全量基线 v0.8 |
| P2B 无效决定案例重建 + 五层评估 | ✅ | 100 例 + L0→L5 定论 |
| P2 引用核验 Gate（P1a-P1c） | ✅ | lawcite + S1 静态表 + S2 法条索引 |
| 协议安全 C1-C8 Critical | ✅ | ACP 认证 / A2A / MCP 信任 / TLS |
| 巨型文件拆分 | ✅ | main.go 821→85 / computer_use 2564→532 |
| 视觉分析真实化 | ✅ | 多模态模型接入 |
| TUI 产品化（12 批次） | ✅ | 流式 11× 提速 / 品牌主题 / 命令系统 / 设置持久化 / 键位配置 |
| Open Design 思路引入（三阶段） | ✅ | MCP Install / SKILL.md 扩展 / 提示词模板 / DocumentStyle / 插件系统 / 文档模板 / Pipeline Atoms / Agent 适配器 |
| DoomLoop 死循环检测 | ✅ | 6 种探测器 + 7 个端到端测试 |
| ReasoningStrategyRouter | ✅ | 4 个领域 Agent 全部接入 |
| 引用核验 S2 法条索引 | ✅ | 专利法 82 条全覆盖 |
| Code Review 6 问题修复 | ✅ | 全部 P1/P2/P3 修复完成 |
| 评估框架增强 | ✅ | CLI 引擎 / enhanced 格式 / ToolAccuracy / WorkflowQuality / Reflection |

### 📋 下一阶段：P3 专家盲测就绪

| 事项 | 状态 | 说明 |
|------|:----:|------|
| HITL 触点全留痕（TUI/Server/ACP） | ✅ | 已打通 |
| P2A 全量 31 题基线 | ✅ | 本地 MLX 免费端点，可高频复跑 |
| 盲测方案成文 | ✅ | `docs/design/p3-blind-test-plan.md` |
| **启动真实盲测** | ⏸️ | 等待人工决策（共识：真人真测，当前只做数据收集） |

### ❄️ 封存项（P3 通过前不启动）

- DomainTrademark 第二垂直
- Memory 生产集成（默认关闭）
- 阶段 D 规则蒸馏（等真人 HITL 数据）
- YAML 声明式工作流收尾（`workflows/patent_novelty.yaml`）
- ACP Rebuildable 完整实现

## 四、停止规则

> **main 不绿 → 不加新功能**
>
> **Golden Set 不能说明质量差异 → 不换模型/Prompt**
>
> **首个专利工作流未通过专家盲测 → 不启动第二垂直**
