# domains/enablement/ 审阅报告 — 2026-07-25

> Phase 2 子审阅（R7）｜依据：`Mady 全面审阅计划 v1.0` ｜执行者：AI（Grok）｜Human Owner：[NEEDS CLARIFICATION]
> 法律依据：《专利法》第26条第3款 + 《审查指南》第二部分第二章 §2.1.3

## 摘要（3 条最关键发现）

1. **【F-01｜高】法律判断逻辑零回归测试覆盖**：`testdata/enablement_cases.json` 定义了 9 个带 `expected` 字段的法律案例（折叠椅违背物理原理、马库什缺实验数据、生物材料未保藏等），但**没有任何测试真正运行图并断言 `expected`**。所有跑图测试使用返回空响应的 mockProvider，9 个精心设计的法律案例的 expected 字段形同虚设。涉及法律结论的模块却无可重复回归测试，是本模块最大风险。
2. **【F-02/F-03/F-04｜高·法律正确性】prompt 混用中美法系 + "六种情形"内部不一致 + 禁用词"必然"**：step3 prompt 引入美国判例 *In re Wands* 的 8 因素到中文《审查指南》26.3 判断中；types.go 注释称"五种 + 实验数据"而 nodes.go/framework.go 均称"六种情形"；结论 prompt 直接写"公开不充分**必然**导致不支持"（"必然"为禁用词，且该司法解释原文措辞待核实）。
3. **【F-05/F-07｜中】错误处理与确定性两处违反"克制"理念**：`NewEnablementToolFromReport` 在 `provider==nil` 时返回 `(nil, nil)` 吞掉错误；`framework.go` 的 `formatArticleData` 遍历 `map[string]string` 顺序非确定，削弱 Temperature=0.2 的可复现性。

## 1. 审阅范围

| 文件 | 行数 | 性质 |
|------|------|------|
| `doc.go` | 25 | 包文档 |
| `types.go` | 161 | 输入/输出值类型 + KnowledgeRetriever 接口 |
| `framework.go` | 138 | 法条框架查询（接口降级） |
| `graph.go` | 65 | Pregel 子图构建 |
| `nodes.go` | ~784 | 五节点实现 + prompt 模板 + JSON 解析 |
| `domain_rules.go` | 294 | 领域检测 + 6 领域自适应规则 |
| `tool.go` | 322 | 工具注册 + 便捷函数 + 知识增强 |
| `enablement_test.go` | ~1180 | 单元/fixture 测试 |
| `integration_test.go` | 235 | 端到端数据流测试 |
| `testdata/enablement_cases.json` | 372 | 9 个法律案例 fixture |

## 2. 审阅维度执行情况（5 Lens 表格）

| Lens | 覆盖度 | 结论 |
|------|--------|------|
| Lens-1 Go 编码 | 充分 | `%w` 规范✓、无生产 panic✓、接口注入良好✓；但 F-05/F-06/F-08 三处错误处理与 nil 防御缺陷 |
| Lens-2 架构分层 | 充分 | graph 用法与 inventiveness 同构✓、未违反分层；disclosure 导入是项目既有约定（patent/inventiveness/specdrafting 同样） |
| Lens-3 安全红线 | 充分 | 三要素（清楚/完整/能够实现）完整✓、置信度强制✓、有 legal_note 免责✓；但 F-02/F-03/F-04 法律引用准确性与措辞存在 3 处 [NEEDS CLARIFICATION] |
| Lens-4 测试 | 充分 | 表格驱动部分到位✓、Provider Stub 完备✓、边界覆盖好✓；但 **F-01 法律判断核心路径无任何断言测试——致命缺口** |
| Lens-5 核心理念 | 充分 | 无 TODO/FIXME✓、构造器无 panic✓、函数式选项克制✓；F-07（map 遍历非确定）与 F-09（784 行）轻微偏离 |

## 3. 发现清单

| ID | 风险等级 | 类别 | 证据(文件:行) | 规范条款 | 建议 |
|----|---------|------|--------------|---------|------|
| **F-01** | **H** | Lens-4 测试 | `testdata/enablement_cases.json` 9 案例 expected 字段；`enablement_test.go` 无加载 fixture→跑图→断言 expected 的代码；`integration_test.go:211` mockProvider 返回空响应 | AGENTS.md"目标驱动，先写测试再修复"；Lens-4 | 引入可注入 Provider 录制/回放 stub，让 9 fixture 真正跑图断言 expected；或至少用固定 JSON 驱动 buildResult 断言 |
| **F-02** | **H** | Lens-3 法律 | `nodes.go:242` step3 prompt 引入美国判例 *In re Wands* 的 8 因素到中文 26.3 判断 | AGENTS.md 安全红线"法律依据引用准确性" | 移除 *In re Wands*，替换为《审查指南》§2.1.3 原文表述。**[NEEDS CLARIFICATION]** 需法律顾问确认混合是否有意 |
| **F-03** | **H** | Lens-3 法律 | `types.go:98` 注释"五种 + 实验数据" vs `nodes.go:212`/`framework.go:88` prompt 称"六种情形" | Lens-3 法律准确性 | 核对《审查指南（2023修订）》§2.1.3 原文列举几种，统一注释与 prompt。**[NEEDS CLARIFICATION]** |
| **F-04** | **M** | Lens-3 法律/措辞 | `nodes.go:312/320` 结论 prompt "公开不充分**必然**导致不支持"，引法释〔2020〕8号第六条第2款 | Claude.md"不使用绝对化表述"；tone-style-guide 禁用词 | 核实法释〔2020〕8号第六条原文措辞；若原文为"必然"则加引号标出处，否则改"通常会导致"并附置信度。**[NEEDS CLARIFICATION]** |
| **F-05** | **M** | Lens-1 错误处理 | `tool.go:261-263` `if provider == nil { return nil, nil }` 吞错误 | AGENTS.md"错误必须修复"；Go 惯例 | 返回 `(nil, errors.New("enablement: provider 未配置"))` |
| **F-06** | **M** | Lens-1 错误处理 | `tool.go:112-148` runEnablementTool 所有错误路径返回 `(map{...,"error":...}, nil)`，error 永远 nil；且把 `err.Error()` 拼进返回可能泄露内部节点名 | Lens-1；一致性 | 保持工具路径软返回但统一两入口契约；对 err.Error() 拼接脱敏/截断 |
| **F-07** | **M** | Lens-5 确定性 | `framework.go:120-123/131-133` 遍历 map[string]string，顺序随机，削弱 Temperature=0.2 可复现性 | Lens-5 | 对 map key 排序后再遍历 |
| **F-08** | **M** | Lens-1 健壮性 | `nodes.go:134` step2ClarityNode 未判 nil（extractInput 失败返回 nil），依赖上游保证 | Lens-1 panic 防御 | `if input == nil { return state, nil }`（与 step1 L70-73 模式一致） |
| F-09 | L | Lens-5 单文件 | `nodes.go` ~784 行混合五节点 prompt + schema + parse | "去繁就简" | 可选拆至 nodes_parse.go + prompts.go |
| F-10 | L | Lens-1 ctx | `tool.go:316` 便捷函数用 context.Background()+10min 无 ctx 参数 | §6.1 | 增加 ctx context.Context 参数 |
| F-11 | L | Lens-2 类型安全 | `graph.go:31` 裸 PregelState + 字符串 key + 类型断言，未启用 StateSchema | — | 与 inventiveness 一致，非违规；提示放弃编译期类型安全 |
| F-12 | L | Lens-3 语义残缺 | `types.go:38` EvidenceCoverage 注释 "full/partial/none" 但 none 永不产生 | — | 实现 none 分支或从注释移除 |

## 4. 已验证合规项

| 维度 | 合规点 | 证据 |
|------|--------|------|
| 依赖倒置 | KnowledgeRetriever 接口注入，nil 降级 | types.go:118-128、tool.go:31、EnrichInput nil 守卫 |
| 依赖倒置 | ArticleFrameworkProvider 接口避免 import domains/rules | framework.go:11-17 注释"避免 transitive build 依赖" |
| graph 契约 | 图拓扑线性正确、entry/edges/end 设置正确 | graph.go:40-61 |
| 分层 | disclosure 导入是项目既有约定（6 处同模式） | grep 对比 |
| 法律三要素 | 清楚/完整/能够实现三要件完整覆盖 | step1(完整)/step2(清楚)/step3(能够实现) |
| 置信度 | confidence 为 schema required，枚举 high/medium/low | nodes.go:463-465、parseConclusion 兜底 medium |
| 免责声明 | legal_note "不构成正式法律意见" 强制 | nodes.go:330 |
| 工具安全 | ReadOnly: true | tool.go:102 |
| 错误 wrap | 全部用 `fmt.Errorf("...: %w", err)` | graph.go:49/57、nodes.go:119/178/283/341 |
| 无生产 panic | panic( 仅测试 helper jsonMarshal（注释"tests only"） | grep 1 处 enablement_test.go:595 |
| 表格驱动 | TestStateHasSkip/TestExtractJSON/TestTruncateText 规范 | enablement_test.go:144/168/196 |
| 边界覆盖 | nil input/report/extraction/空特征 均有测试 | integration_test.go:130/143/156 |
| 知识库联动 | mock retriever 测了成功/失败/nil 三路径 | enablement_test.go:1030/1044/1050/1093 |

## 5. 法律正确性待裁决项（[NEEDS CLARIFICATION]）

| ID | 问题 | 需裁决方 | 位置 |
|----|------|----------|------|
| F-02 | 美国 *In re Wands* 8 因素是否应出现在中文 26.3 判断 prompt？是否导致 LLM 产出非中国法口径结论？ | 专利法律顾问 | nodes.go:242 |
| F-03 | 《审查指南（2023修订）》§2.1.3 原文实际列举几种"不能实现"情形？注释（5+1）与 prompt（6）以哪个为准？ | 专利法律顾问 | types.go:98 vs nodes.go:212/framework.go:88 |
| F-04 | 法释〔2020〕8号第六条第2款原文是否用"必然"二字？措辞须与原文一致 | 专利法律顾问 | nodes.go:312/320 |
| F-04 衍生 | "公开不充分必然导致不支持"会被 LLM 写入面向用户 reasoning，是否违反禁用绝对化词？法条原文优先还是文案规范优先？ | 产品/法律 | nodes.go:312 |
| 补充 | domain_rules.go 引用的具体案号（"2014行提字第8号 阿托伐他汀""(2015)知行字第352号""无效宣告第34992/19367/13248/73780号"）是否真实存在、引用准确？ | 专利法律顾问 | domain_rules.go:142/166、testdata/*.json source |

## 6. 建议下一步

1. **【立即·高】补 F-01 核心路径回归测试**：设计可注入 Provider 录制/回放机制，让 9 fixture 真正跑图断言 expected。**无回归测试的法律判断不可交付**。
2. **【立即·高】裁决 F-02/F-03/F-04 法律正确性**：交专利法律顾问核对审查指南原文、司法解释原文、案号真实性；统一"五种/六种"表述；决定 *In re Wands* 去留。
3. **【本轮·中】修 F-05/F-07/F-08**：NewEnablementToolFromReport 返回真实 error；formatArticleData map key 排序；step2 加 nil 防御。3-5 文件范围内。
4. **【次轮·低】F-06/F-09/F-10/F-11/F-12**：统一两入口错误契约、拆分 nodes.go、便捷函数加 ctx、评估 StateSchema、清理 EvidenceCoverage。
5. **【流程】**：本模块不在敏感路径表内，但产出涉及法律结论，建议将结论生成节点（generateConclusionNode）纳入敏感路径评审，与 disclosure/report.go 同级管理。

> 审阅基于静态代码分析。建议采纳修复前先 `make verify` 确认基线，修复后对 F-01 补测试再回归。
