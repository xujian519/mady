# domains/inventiveness/ 审阅报告 — 2026-07-25

> Phase 2 子审阅（R6）｜依据：`Mady 全面审阅计划 v1.0` ｜执行者：AI（Grok）｜Human Owner：[NEEDS CLARIFICATION]
> 验证：本报告关键结论已由主审阅员 grep + read_file 二次核实

## 摘要（3 条最关键发现）

1. **【H｜法律逻辑一致性】`IsInventive` 结论的非对称覆盖逻辑（`nodes.go:786-797`）**：最终结论节点的 LLM 整体判断会**非对称地覆盖**确定性的"Step3 非显而易见 AND Step4 显著进步"计算。当 `cc.HasSignificantProgress==true && cc.IsInventive==false` 时直接强制 `IsInventive=false`，完全无视 Step3 独立得出的"非显而易见"结论。这与 `graph.go:43`/`types.go:104`/`nodes.go:433` 反复声明的公式 `IsInventive = (Step3: 非显而易见) AND (Step4: 显著进步)` 自相矛盾，在法律结论模块里属于高风险。
2. **【H｜单文件超限】`nodes.go` 共 1020 行**，是"去繁就简"理念与行业惯例（≤500 行）上限的 **2 倍**。该文件混杂了 7 个 Pregel 节点、6 个 schema、5 个解析函数、4 段提示词常量与多个辅助函数。
3. **【M｜架构分层】`tool.go:11` 直接 `import disclosure`**（基础设施层），`NewInventivenessToolFromReport` 依赖 `*disclosure.AnalysisReport` / `[]disclosure.EvidenceChunk` 具体类型，违反 AGENTS.md"Domain 层不得 import Infrastructure 具体实现"。值得肯定的是**核心图引擎路径（graph.go/nodes.go）通过 types.go 镜像类型已正确隔离**，违规仅集中在这一个便捷适配函数。

## 1. 审阅范围

| 维度 | 数据 |
|------|------|
| 文件总数 | 8（6 非测试 + 2 测试） |
| 非测试文件 | `doc.go`(42) · `framework.go`(~432) · `graph.go`(83) · `nodes.go`(**1020**) · `tool.go`(313) · `types.go`(155) ≈ **2045 行** |
| 测试文件 | `inventiveness_test.go`(~1000+) · `integration_test.go`(220) ≈ **1220+ 行** |
| 是否含 TODO/FIXME/panic | 否（全模块 0 处） |
| 覆盖率 | 核心图引擎覆盖良好；但核心 LLM 推理链未用真实 stub 覆盖（见 F-04） |

## 2. 审阅维度执行情况（5 Lens 表格）

| Lens | 执行状态 | 关键结论 |
|------|---------|---------|
| Lens-1 Go 规范 | ⚠️ 部分违规 | 错误 `%w` 包装✓、Happy Path✓、注释规范✓、接口小而精✓；但工具层吞 error（F-05）、测试 `_ =` 忽略错误（F-06） |
| Lens-2 架构分层 | ⚠️ 部分违规 | core 图引擎分层干净✓、Pregel 拓扑正确✓；但 `tool.go` 跨层依赖 disclosure 具体类型（F-03）；"四轮迭代"实为单趟线性图（F-07） |
| Lens-3 安全红线 | ⚠️ 存在高危 | 法条引用准确✓、结论附置信度✓、无绝对化词✓；但 `IsInventive` 逻辑与声明的 AND 公式不一致（F-01）—— 法律结论一致性高危 |
| Lens-4 测试质量 | ⚠️ 核心路径空洞 | 解析/schema/framework 测试充分✓、表格驱动✓；但 mockProvider 返回空响应、无 callCount（F-04），"审查模拟"链路未被真实 JSON 覆盖 |
| Lens-5 核心理念 | ⚠️ 超限 | 无 TODO/FIXME✓、构造器用 error 不 panic✓、无过度抽象✓；但 nodes.go 1020 行严重超限（F-02） |

## 3. 发现清单

| ID | 风险等级 | 类别 | 证据(文件:行) | 规范条款 | 建议 |
|----|---------|------|--------------|---------|------|
| **F-01** | **H** | Lens-3 法律逻辑一致性 | `nodes.go:786-797`（buildResult switch）；对照声明 `graph.go:43`、`types.go:104`、`nodes.go:433` | tone-style-guide §6 结论一致性；A22.3 二要件 AND 关系 | 先计算 `computed := !Step3.TechnicalSuggestion && Step4.HasSignificantProgress`，仅当 cc 与 computed 冲突时记录分歧并降置信度，而非让 cc 单方面否决 |
| **F-02** | **H** | Lens-5 单文件超限 | `nodes.go` = **1020 行** | 核心理念"去繁就简"；行业 ≤500 行惯例 | 拆分：`nodes.go`(节点闭包) / `parse.go`(parseStep* + extractJSON) / `schema.go`(*Schema) / `prompt.go`(提示词常量) |
| **F-03** | **M** | Lens-2 架构分层 | `tool.go:11` import；`tool.go:237` `NewInventivenessToolFromReport(... *disclosure.AnalysisReport, ...)` | AGENTS.md "Domain 层不得 import Infra 具体实现" | 定义 `ReportSource` 接口（消费端定义），或把该便捷函数移至 adapter 包 |
| **F-04** | **M** | Lens-4 核心路径测试空洞 | `inventiveness_test.go:16-28`（mockProvider 返回空 `ProviderResponse{}`，无 callCount） | GO-DEVELOPMENT-STANDARDS §7.3 Provider Stub（callCount + 顺序 responses） | 新增带 callCount + 分步 JSON 的 stub，验证各步 state 传递 + AND 结论逻辑 + 反向教导/跨领域标记透传 |
| **F-05** | **M** | Lens-1 错误处理 | `tool.go:116-148` runInventivenessTool 把所有 error 吞进 `map{"error":…}` 返回 `nil`，丢失 RetryableError/FatalError 分类 | GO-DEVELOPMENT-STANDARDS §4.3 保留错误分类 | map 内增加 `error_type`/`retryable` 字段；或不可恢复时返回 error |
| **F-06** | L | Lens-4 测试忽略错误 | `integration_test.go:90` `_ = runErr`；`integration_test.go:120` `state, _ = compiled.Run(ctx, state)` | §1.4 始终检查错误返回值 | 至少 `t.Logf` 记录，或对预期错误做断言 |
| **F-07** | L | Lens-2 文档/实现不符 | `graph.go:48` `Compile("load_input", 100)` 但图中**无边回流**（7 节点严格线性） | 任务宣称"四轮迭代优化" | 若"迭代优化"是预期则需实现 review loop；否则修订对外描述为"四步线性审查" |
| **F-08** | L | Lens-1 并发/上下文 | `tool.go:296` 用 `context.Background()` + 硬编码 10min，忽略调用方 ctx | §6.1 ctx 管理生命周期 | 函数签名增加 `ctx context.Context` 参数 |
| **F-09** | L | Lens-1 健壮性 | `nodes.go:1011-1019` extractJSON 用首 `{` + 末 `}` 启发式 | — | 已有 parseStep* fallback 兜底，风险可控；可选改用更稳健提取 |

## 4. 已验证合规项

- **无 TODO/FIXME/XXX/HACK/panic**：全模块 grep 0 命中
- **接口设计精简**：仅 1 个接口 `ArticleFrameworkProvider`（1 方法），定义在消费端
- **错误包装规范**：节点错误均用 `%w` 包装（nodes.go:101/198/319/367/509、graph.go:65/77）
- **Happy Path 左对齐**：loadInputNode/stateHasSkip 等均提前返回
- **注释规范**：导出符号注释齐全、中文、首句英文符号名开头
- **构造器用 error 而非 panic**：BuildInventivenessGraph 返回 `(*CompiledPregelGraph, error)`
- **core 图引擎分层干净**：graph.go/nodes.go 仅 import agentcore+graph，types.go 用镜像类型隔离
- **法律依据准确**：创造性→专利法第22条第3款；三步法→审查指南第二部分第四章；"预料不到的技术效果是充分条件而非必要条件"表述正确
- **结论附置信度**：InventivenessResult.Confidence + 各 schema 强制 confidence required；低置信度 medium 兜底
- **无绝对化词污染最终结论**：`绝对/一定/百分百` 0 命中；"必然"仅出现在 prompt 推理规则文本中

## 5. 法律正确性待裁决项（[NEEDS CLARIFICATION]）

| ID | 问题 | 需裁决方 |
|----|------|----------|
| F-10 | `framework.go:288`/`nodes.go:1003` 引用"基于 39,496 份复审/无效决定的元数据分析（样本中 12,798 份涉及创造性）"及各项成功率（54%/95.9%/73.7%/5.7%）。数据集来源、年份口径、统计方法无法核实，直接影响 LLM 置信度校准。**需确认数据出处可追溯、是否取得使用授权** | 专利法务/数据方 |
| F-11 | `framework.go:67` 标注"审查指南（2023 修订）第二部分第四章（含第 84 号局令修订）"。精确版本号、生效日（倾向 2024-01-20）、对应关系需法务确认 | 专利法务 |
| F-12 | **结论优先级口径（关联 F-01）**：当结论节点 LLM 判断与"Step3+Step4 确定性计算"冲突时，现行代码让 conclusion 单方面否决。是"分步计算为终审"还是"结论节点为终审"？需法务/产品明确立场（建议倾向前者，因 Step3/Step4 各有独立 schema 约束，更可审计） | 法务/产品 |

## 6. 建议下一步

1. **【先定性，再改码】裁决 F-01/F-12**：请专利法务确认"分步计算 vs 结论节点"的优先级口径。这是改 F-01 的前提。
2. **拆分 F-02（nodes.go 1020 行）**：纯机械重构、零行为变更、风险最低，可作为首个落地 PR。拆为 nodes.go/parse.go/schema.go/prompt.go。
3. **补 F-04 核心路径测试**：引入带 callCount + 顺序 JSON 的 stub，覆盖 Step1→conclusion 全链路与 AND 结论逻辑。
4. **修 F-03 分层违规**：把 NewInventivenessToolFromReport 改为依赖接口或移至 adapter 包。
5. **清理 F-05/F-06/F-08**：工具层 error 透出分类、测试不再 `_ =`、便捷函数接受 ctx，可合并为一个 polish PR。
6. **核对 F-10/F-11**：补全实证数据出处与法条版本号文档脚注。

> 整体评价：本模块**法理知识体系扎实、core 引擎分层干净、合规度高**。核心待解问题是**法律结论的逻辑一致性（F-01）**与**单文件体量（F-02）**，二者解决后模块可达可审计、可维护的成熟状态。
