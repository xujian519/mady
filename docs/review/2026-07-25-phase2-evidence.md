# domains/evidence/ 专利证据规则引擎审阅报告 — 2026-07-25

> Phase 2 子审阅（R8）｜依据：`Mady 全面审阅计划 v1.0` ｜执行者：AI（Grok）｜Human Owner：[NEEDS CLARIFICATION]
> **本报告颠覆性结论（"整模块死代码"）已由主审阅员 grep 二次核实**

## 摘要

| 维度 | 评级 | 一句话结论 |
|------|------|-----------|
| Lens-1 Go 编码 | 🟡 有债务 | 重复计数逻辑 bug + 两套并行 API + 硬编码阈值 |
| Lens-2 架构分层 | 🔴 **严重** | **整个模块是死代码（零外部引用）**；规则 DSL 被解析但从未执行 |
| Lens-3 安全红线 | 🔴 **严重** | 侵权证明标准三实现互相矛盾 + 混用美国法术语；结论无置信度/无法条引用 |
| Lens-4 测试 | 🟡 有债务 | burden.go / standard.go 零测试；六覆盖域强类型 API 未覆盖 |
| Lens-5 核心理念 | 🟡 有债务 | 过度设计（违反"去繁就简"）；engine.go 770 行、date.go 500 行 |

**总体定性**：模块代码本身工程素养不低（并发正确、无 panic、错误处理规范），但作为"专利证据判断规则引擎"存在三个根本性问题：**(1) 完全未被接入，是孤岛代码（~3400 行）；(2) 举证/证明标准有三套互相矛盾的实现，其中一套混用美国法术语；(3) 名为"规则引擎"但规则 DSL 仅被索引不被执行**。接入生产前必须先解决这三个问题。

## 1. 审阅范围

| 文件 | 行数 | 职责 |
|------|------|------|
| `engine.go` | 770 | DefaultEngine：Judge/BatchJudge/AssessBurdenOfProof/AssessProofStandard + 三性评分 + 互联网公开/使用公开辅助 |
| `date.go` | 500 | 日期确定：DeterminePublicationDate/InternetPublicationDate/PublicUseDate + 文本日期抽取 |
| `types.go` | ~258 | 全部类型定义 + EvidenceJudgmentEngine 接口 |
| `credibility.go` | 183 | 平台可信度 + 电子证据综合评估 |
| `burden.go` | 134 | 举证责任（强类型函数版 DetermineBurden + DefaultBurdenRules） |
| `rule_loader.go` | 114 | RuleIndex：YAML 加载/索引/按类型查询 |
| `standard.go` | 113 | 证明标准（强类型函数版 AssessProofStandard + DetermineStandard） |
| `doc.go` | 3 | 包注释 |
| **合计** | **8 源 + 5 测试 ≈ 2075 源行 + 1358 测试行** | |

## 2. 审阅维度执行情况（5 Lens）

### Lens-1 Go 编码规范

| ID | 等级 | 证据 | 建议 |
|----|------|------|------|
| **F-L1-1** | **M** | `engine.go:165-176` AssessProofStandard 重复计数：低分证据（OverallScore<0.6）进 else 分支 contradicting++，若同时 hasConflict() 再 contradicting++，同一证据被计两次。影响 Met 判定（supporting > contradicting）系统性偏保守 | 冲突只标记不二次计数；补"低分+冲突"测试 |
| F-L1-2 | M | 两套并行 API：举证责任 engine.go:131 方法版 vs burden.go:73 函数版；证明标准 engine.go:157 vs standard.go:17。语义不同（阈值 vs 占比）互不引用 | 统一为单一实现 |
| F-L1-3 | L | 三性评分/证明标准/可信度阈值全硬编码（0.95/0.80/0.50/0.85/0.65） | 可配置化 |
| F-L1-4 | L | computeOverallScore 每次 Judge 全量扫规则 | 缓存权重结果 |

✅ 无 panic；RuleIndex 用 sync.RWMutex 读路径 RLock 返回副本（rule_loader.go:77-97）；构造器走 error；错误用 %w 包裹。

### Lens-2 架构分层与契约

| ID | 等级 | 证据 | 建议 |
|----|------|------|------|
| **F-L2-1** | **🔴 C** | **整模块死代码**：grep `github.com/xujian519/mady/domains/evidence` 全仓**零外部引用**（已二次核实）；EvidenceJudgmentEngine 接口仅 types.go:248 自身定义无消费方；router/workflows/cmd 均不引用 | **接入决策**：接入生产或标记实验/归档 |
| **F-L2-2** | **H** | "规则引擎"名不副实：EvidenceRule.Check/Conditions/Exemptions/PlatformCredibility 构成 DSL，但 Judge（engine.go:26）虽调 GetRulesByType，evaluateTripleAttributes/evaluateTypeSpecific 评分全硬编码，**从不迭代 rules 应用 Check/Conditions**。PlatformCredibility map 在 Judge 路径从未读取。规则唯一被消费处是 computeOverallScore 读 Dimensions.Weight | 要么在 Judge 应用规则，要么删除未用 DSL 字段 |
| F-L2-3 | M | EvidenceJudgmentEngine 接口混合三类职责（判断+程序法+规则管理），LoadRules 把存储耦合进判断接口，违反 ISP | 拆分为多个小接口 |

✅ 依赖方向正确：Domain(evidence) → agentcore，无反向。

### Lens-3 安全红线（法律结论）

| ID | 等级 | 证据 | 建议 |
|----|------|------|------|
| **F-L3-1** | **H** | 侵权证明标准三实现矛盾：engine.go:135 `clear_and_convincing`（美国法）vs burden.go:36 `高度盖然性` vs standard.go:95 StandardHighProbability。"clear and convincing"是美国法标准；中国民事程序依《民诉法解释》第108条适用「高度盖然性」 | 统一为「高度盖然性」，删除美国术语。**[NEEDS CLARIFICATION]** clear_and_convincing 是否针对特定场景有意保留 |
| **F-L3-2** | **H** | BurdenDetermination（types.go:230）**无 Confidence 字段**；AssessBurdenOfProof 的 Reasoning 是无置信度断言。对照 ProofStandardResult 有 Confidence，口径不统一 | 增加 Confidence 字段 |
| F-L3-3 | M | 结论文案无法条引用：EvidenceRule.LegalBasis 字段存在但 AssessBurdenOfProof/AssessProofStandard 输出从不携带法条编号。"举证责任倒置"不引《专利法》第66条、"谁主张谁举证"不引《民诉法》第64条 | BurdenDetermination/ProofStandardResult 增加 LegalBasis 字段。**[NEEDS CLARIFICATION]** 项目以哪版《专利法》为准（2020版方法专利倒置为第66条，2008版第61条） |
| F-L3-4 | M | EvidenceType 枚举（types.go:10-28）与《民诉法》法定证据分类（书证/物证/视听资料/电子数据/证人证言/当事人陈述/鉴定意见/勘验笔录）不对齐，且混入 burden_of_proof/standard_of_proof/prior_art_date 等程序性概念 | 提供与法定分类的映射。**[NEEDS CLARIFICATION]** 现行《民诉法》证据条款为第66条（2023版含电子数据），任务说明引第63条为旧编号 |
| F-L3-5 | M | evaluateLegality（engine.go:362-388）基准分 0.7 仅按 SourceURI/ContentHash 加减，无法判断取证合法性。带 ContentHash 的非法获取证据会被评到 0.9（"high"），法律误导 | 改名 evaluateMetadataCompleteness 或引入取证合法性维度 |
| F-L3-6 | L | standard.go:7 StandardBeyondReasonableDoubt（排除合理怀疑）主要用于刑事，专利（民事/行政）一般不适用 | 注释"专利程序通常不适用"。**[NEEDS CLARIFICATION]** 是否仅为完整性保留 |

✅ "绝对新颖性标准"（engine.go:597）是专利法固定术语（A22 世界范围新颖性），**非禁用绝对化词**，判定合规；拒绝类文案提供替代建议（assessPublicUseChainIntegrity）。

### Lens-4 测试与质量门禁

| ID | 等级 | 证据 | 建议 |
|----|------|------|------|
| **F-L4-1** | **H** | burden.go 零测试：无 burden_test.go；DetermineBurden/DefaultBurdenRules/formatBurdenReasoning/prima_facie 均无用例 | 新建 burden_test.go |
| **F-L4-2** | **H** | standard.go 零测试：无 standard_test.go；包级 AssessProofStandard/DetermineStandard/5 个常量阈值边界无用例 | 新建 standard_test.go |
| F-L4-3 | M | credibility.go 覆盖不足：仅 3 个 PlatformCredibility 用例；AssessElectronicEvidence/CredibilityToScore 未测 | 扩展 credibility_test.go |
| F-L4-4 | M | computeOverallScore 权重覆盖分支（engine.go:221-229）未测 | 补含 dimensions 的规则用例 |
| F-L4-5 | M | F-L1-1 重复计数 bug 未被测试捕获（engine_test.go:148 仅测 Met=true） | 补"低分+冲突"用例 |
| F-L4-6 | L | 三性评分边界值（0.85/0.65/0.45）无单测 | 补边界值断言 |

✅ 六覆盖域：三性✓、类型特定✓、日期✓、规则加载器（rule_loader_test.go 294 行）扎实。

### Lens-5 核心理念落地

| ID | 等级 | 证据 | 建议 |
|----|------|------|------|
| **F-L5-1** | **H** | 过度设计：两套并行 API（F-L1-2）+ 未消费的规则 DSL（F-L2-2）+ 未消费的接口（F-L2-1）。与"去繁就简"冲突 | 接入前先做减法 |
| F-L5-2 | M | engine.go 770 行混合三性评分+类型分派+互联网公开+使用公开+平台分类+URI 清洗；date.go 500 行 | engine.go 拆为 scoring.go/internet_publication.go/public_use.go |
| F-L5-3 | L | date.go:285 extractDateFromText 4 套策略偏重，且零调用 | 视接入决策定去留 |

✅ 无 TODO/FIXME/HACK；构造器无 panic。

## 3. 发现清单（按严重度汇总）

| # | 严重度 | 位置 | 问题 |
|---|--------|------|------|
| F-L2-1 | 🔴 Critical | 整模块 | 零外部引用，整模块死代码（已二次核实） |
| F-L2-2 | 🔴 High | engine.go Judge | 规则 DSL 被解析但从不执行 |
| F-L3-1 | 🔴 High | engine.go:135/burden.go:36/standard.go:95 | 侵权证明标准三实现矛盾+混用美国法术语 |
| F-L3-2 | 🔴 High | types.go:230/engine.go:131 | 法律结论（举证责任）无置信度 |
| F-L4-1 | 🔴 High | burden.go | 零测试 |
| F-L4-2 | 🔴 High | standard.go | 零测试 |
| F-L5-1 | 🔴 High | 整模块 | 过度设计（违反去繁就简） |
| F-L1-1 | 🟡 Medium | engine.go:165-176 | 重复计数 bug |
| F-L1-2 | 🟡 Medium | engine.go/burden.go/standard.go | 两套并行 API |
| F-L2-3 | 🟡 Medium | types.go:248 | 接口违反 ISP |
| F-L3-3 | 🟡 Medium | engine.go:137-148 | 结论无法条引用 |
| F-L3-4 | 🟡 Medium | types.go:10-28 | 证据类型枚举与法定分类不对齐 |
| F-L3-5 | 🟡 Medium | engine.go:362-388 | evaluateLegality 名实不符 |
| F-L4-3/4/5 | 🟡 Medium | credibility/engine weights/test | 覆盖不足 |
| F-L5-2 | 🟡 Medium | engine.go/date.go | 单文件偏大 |
| F-L1-3/4, F-L3-6, F-L4-6, F-L5-3 | 🟢 Low | 多处 | 硬编码/性能/标准适用性/边界测试/启发式偏重 |

## 4. 已验证合规项

- ✅ 无 panic；构造器走 error
- ✅ RuleIndex 并发安全（RWMutex + 副本返回）
- ✅ 依赖方向正确（Domain → agentcore）
- ✅ 无 TODO/FIXME/HACK 残留
- ✅ 规则加载器测试扎实（报错/重置/排序/回退）
- ✅ 拒绝类文案提供替代建议
- ✅ "绝对新颖性"为合法术语，非禁用绝对化词
- ✅ 三性/类型特定/日期 三域测试较全

## 5. 法律正确性待裁决项（[NEEDS CLARIFICATION]）

| # | 待裁决问题 | 关联发现 |
|---|-----------|---------|
| Q-1 | 侵权证明标准应为「高度盖然性」（中国民事标准）还是保留「clear_and_convincing」（美国法）？三处实现需统一 | F-L3-1 |
| Q-2 | 项目以哪版《专利法》为准？方法专利举证责任倒置 2020 版第66条、2008 版第61条（任务说明引第61条，需确认） | F-L3-3 |
| Q-3 | EvidenceType 枚举是否应与《民诉法》法定证据分类对齐？现行《民诉法》证据条款第66条（2023版含电子数据），任务说明引第63条为旧编号 | F-L3-4 |
| Q-4 | StandardBeyondReasonableDoubt（排除合理怀疑）是否应在专利引擎保留？ | F-L3-6 |
| Q-5 | evaluateLegality 仅凭元数据评分合法性，是否需引入取证合法性维度？否则建议改名 | F-L3-5 |

## 6. 建议下一步

1. **接入决策（最高优先）**：裁定 `domains/evidence` 是接入生产还是标记实验/归档。若接入需在 router/workflows/cmd 完成接线并记 AI_CHANGELOG；若暂不接入应明确标注避免维护噪声（F-L2-1）。**这是"去繁就简"的关键决策——~3400 行死代码要么激活要么删除。**
2. **统一举证/证明标准为单一实现**：以强类型版（burden.go/standard.go）为准，废弃 engine 方法版字符串键实现，消除 F-L1-2 与 F-L3-1 术语矛盾。
3. **修复重复计数 bug**（F-L1-1）：engine.go:165-176 改为冲突只标记不二次计数，补测试。
4. **补齐测试**：新建 burden_test.go/standard_test.go（F-L4-1/2）、扩展 credibility_test.go（F-L4-3）。`go test -race ./domains/evidence/`。
5. **法律结论合规化**：BurdenDetermination 增加 Confidence 与 LegalBasis 字段（F-L3-2/3）。法条基准版本 Q-2 裁定后统一。
6. **让规则引擎真正"执行规则"或精简 DSL**（F-L2-2）。
7. **拆分大文件**：engine.go → scoring/internet_publication/public_use（F-L5-2）。
8. **重命名 evaluateLegality**（F-L3-5），消除法律语义误导。

> 关键提醒：本报告未执行 `go test -cover`（只读审阅），覆盖率基于测试文件静态分析；建议本地补跑 `go test -race -cover ./domains/evidence/`。法律待裁决项 Q-1~Q-5 需领域/法务确认。
