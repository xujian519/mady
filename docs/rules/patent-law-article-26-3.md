# 专利法第26条第3款 — 说明书充分公开规则

> 自动生成于 2026-07-22，来源为 Mady 项目知识系统全量检索。
> 覆盖：S1 静态引用表、YAML 规则引擎、审查指南索引、OA 解析器、法律意图分类器、引文核验门、评测基准。

---

## 1. 法条原文与核心标准

**《中华人民共和国专利法》（2020 年修正）第 26 条第 3 款：**

> 说明书应当对发明或者实用新型作出清楚、完整的说明，以所属技术领域的技术人员能够实现为准。

### 核心要素

| 要素 | 含义 |
|------|------|
| **清楚** | 技术术语含义明确，无歧义，主题明确，表述准确 |
| **完整** | 包含理解发明、实现发明所需的全部必要技术内容 |
| **能够实现** | 所属技术领域的技术人员根据说明书记载无需创造性劳动即可实施 |
| **所属技术领域的技术人员** | 知晓申请日（或优先权日）前本领域普通技术知识，能够获知所有现有技术，具有常规实验能力 |

---

## 2. 来源：S1 静态引用主题表

**文件：** `guardrails/citation_table.go:37`

```go
26: {"清楚", "完整", "支持", "摘要", "充分公开"},
```

- 第 26 条被列入 **无效宣告理由**（`invalidationGrounds`，`citation_table.go:155`）
- 专利法存在性上限：82 条（`maxPatentLawArticle = 82`）

### 引用核验规则（`guardrails/citation_gate.go`）

对"专利法第26条第3款"的引用执行双级核验：

| 级别 | 核验内容 | 不通过判定 |
|------|----------|-----------|
| R1 存在性 | 条号 26 ≤ 82 | `VerdictInvalid`（幻觉编号） |
| R2 语境相关性 | 引用语境词与注册主题词匹配 | `VerdictSuspect`（张冠李戴疑点） |

---

## 3. 来源：YAML 规则引擎 — 充分公开核查

**文件：** `domains/rules/data/rules/patent-core.yaml:115-142`

```yaml
- ruleId: "patent-a26.3-disclosure"
  name: "说明书充分公开核查"
  description: >
    根据专利法第26条第3款，核查说明书是否对发明作出清楚、完整的说明，
    以所属技术领域的技术人员能够实现为准。
  legalBasis: "专利法第26条第3款"
  domain: patent
  severity: critical
  action: block
  check:
    type: patent_disclosure
    method: check_sufficient_disclosure
    principles:
      - "以所属技术领域的技术人员能够实现为准"
      - "清楚：技术术语含义明确，无歧义"
      - "完整：包含理解发明、实现发明所需的全部必要技术内容"
      - "能够实现：技术人员根据说明书记载无需创造性劳动即可实施"
    requirements:
      - "技术领域"
      - "背景技术"
      - "发明内容（要解决的技术问题、技术方案、有益效果）"
      - "附图说明（如有附图）"
      - "具体实施方式（至少一个实施例）"
    rules:
      - "缺少关键技术手段的说明 → 公开不充分"
      - "技术手段含糊不清 → 公开不充分"
      - "仅给出任务/设想，未给出具体技术手段 → 公开不充分"
      - "实验数据不足以证明技术效果 → 公开不充分"
```

### 法条判断框架工具

**文件：** `domains/rules/engine.go:248`

工具 `get_article_framework` 支持 `A26.3`（充分公开）的法条判断框架查询，返回步骤、输入输出、结论模式。

---

## 4. 来源：确定性规则引擎

**文件：** `workflows/patent/rule_engine.go:396-406`

```go
{
    ID:              "DISCLOSURE-SUFFICIENCY",
    Name:            "充分公开审查",
    Description:     "说明书应充分公开发明，使本领域技术人员能够实现",
    Level:           LevelShould,      // 级别1：重要（单次失败即阻断）
    Severity:        SeverityMajor,    // 严重度：major
    Message:         "充分公开分析不完整",
    CheckType:       CheckDisclosure,
    RequiredAspects: []string{"充分公开", "能够实现"},
    Domain:          "patent_disclosure",
    FixSuggestion:   "确认说明书是否提供足够的技术细节使本领域技术人员能够实现该发明",
}
```

### 同义词扩展（`rule_engine.go:271`）

```
"充分公开" → {"公开充分", "能够实现", "enablement"}
```

### 否决模式（`rule_engine.go:278-289`）

以下语义词出现时不计入肯定匹配：

```
不具有、不构成、无法证明、缺少、未发现、没有公开、不满足、不符合、难以看出、不能证明
```

---

## 5. 来源：审查指南索引

**文件：** `guardrails/guideline_source.go:33`

```go
20201: {"说明书", "充分公开", "能够实现", "清楚完整"},
```

对应 **《专利审查指南》第二部分第二章第 2.1 节**（编码 `20201`），是审查指南中关于说明书的"充分公开"最核心的章节：

- **编码规则**：`PPPCCSS`（Part=2, Chapter=2, Section=1）
- **主题关键词**：说明书、充分公开、能够实现、清楚完整
- **引用核验**：当模型输出引用"审查指南第二部分第二章第 2.1 节"时，通过 `GuidelineSource.Topics()` 匹配上述关键词验证引用正确性

---

## 6. 来源：OA 审查意见解析器

**文件：** `domains/rules/oa_parser.go:48`

```go
{OaDisclosure, []string{"公开不充分", "26条第3款", "无法实现"}},
```

驳回类型枚举（`oa_parser.go:15`）：

```go
OaDisclosure OaRejectionType = "disclosure"  // 公开不充分
```

**案件分类器**（`domains/case_extractor.go:276`）：

```go
{"disclosure", []string{"公开不充分", "26条第3款"}},
```

---

## 7. 来源：法律意图分类器

**文件：** `domains/legal_intent.go:82-85`

```go
{
    keywords: []string{"充分公开", "公开不充分", "A26.3"},
    caseType: reasoning.CaseInvalidation,  // 归类为无效宣告案件
    mode:     ModeJudgment,                // 判定模式
},
```

- 当用户输入中包含"充分公开"、"公开不充分"、"A26.3"时，Router 自动识别为 **无效宣告类案件**，采用 **判定模式**（有别于咨询/分析模式）

---

## 8. 来源：无效宣告工作流

**文件：** `domains/reasoning/manifest.go:459`

在无效宣告分析的第二步中，明确将"公开充分（第26条第3款）"列为逐项分析的无效理由之一：

```
逐项分析无效理由：新颖性（第22条第2款，单独对比）、
创造性（第22条第3款，三步法）、公开充分（第26条第3款）、
修改超范围（第33条）
```

---

## 9. 来源：法律引用解析器

**文件：** `pkg/lawcite/lawcite.go`

`lawcite.Extract()` 对流式文本中的法条引用进行结构化抽取：

- 输入文本中的 `"专利法第26条第3款"` 被解析为：
  ```go
  Citation{
      Statute:   StatutePatentLaw,  // 专利法
      Article:   26,                // 第26条
      Paragraph: 3,                 // 第3款
      Item:      0,                 // 无项
      Suffix:    0,                 // 无之N
  }
  ```
- 支持中文数字归一化：`"专利法第二十六条第三款"` → 同上述结果

---

## 10. 来源：评测基准

**文件：** `evaluate/benchmark/patent_exam_real_a26.go`

包含基于历年专利代理人资格考试真题的 A26 相关评测用例：

| 用例 ID | 来源 | 考点 |
|---------|------|------|
| `patent_exam_2008_a26_03` | 2008 年实务 | A26.4 支持问题（油炸食品案例） |
| `patent_exam_2013_a26_01` | 2013 年实务 | A26.4 不清楚/不支持（垃圾箱案例） |
| `patent_exam_2017_a26_01` | 2017 年实务 | A26.4 不清楚/不支持（起钉锤案例） |

> 注：当前评测基准中 A26 相关用例主要覆盖第 26 条第 4 款（清楚/支持），第 3 款（充分公开）的专项评测用例待补充。

---

## 11. 知识图谱中的关联关系

```
专利法第26条
├── 第1款：申请文件要求（请求书、说明书、权利要求书、摘要、附图）
├── 第2款：摘要要求
├── 第3款：说明书充分公开 ★ (本条规则)
│   ├── 清楚 — 技术术语含义明确，无歧义
│   ├── 完整 — 含理解发明和实现发明的全部必要技术内容
│   └── 能够实现 — 技术人员无需创造性劳动即可实施
├── 第4款：权利要求清楚、简要、以说明书为依据
└── 第5款：依赖遗传资源的来源披露
```

### 邻近法条引用关系

| 法条 | 主题 | 关联场景 |
|------|------|----------|
| 第 22 条第 2/3 款 | 新颖性/创造性 | 三性判断通常与充分公开并行审查 |
| 第 26 条第 4 款 | 权利要求清楚/支持 | 与第 3 款为同条不同款，OA 中常一并引用 |
| 第 33 条 | 修改不超范围 | 无效宣告中常与第 26 条第 3 款组合作为无效理由 |
| 审查指南 P2C2S1 | 说明书充分公开 | 第 26 条第 3 款的审查指南具体规定 |

---

## 12. 规则应用矩阵

| 应用场景 | 触发方式 | 严重度 | 处置动作 |
|----------|----------|--------|----------|
| 引用核验（R1 存在性） | 模型输出含"第26条" | — | 26→放行；≥83→提示幻觉 |
| 引用核验（R2 相关性） | 模型输出含"第26条第3款"+用途声明 | — | 匹配"清楚/完整/充分公开/能够实现"→放行；否则→存疑提示 |
| 规则引擎检查 | `CheckDisclosure` 规则 | Major | 缺少"充分公开"或"能够实现"关键词→阻断(LevelShould) |
| YAML 规则检查 | `patent-a26.3-disclosure` | Critical | 公开不充分证据确凿→ block |
| OA 解析 | 审查意见文本 | — | 匹配"公开不充分/26条第3款/无法实现"→识别为 disclosure 驳回 |
| 意图路由 | 用户输入含"充分公开/公开不充分/A26.3" | — | 路由至 CaseInvalidation + ModeJudgment |
| 无效宣告分析 | `Stage4 Step2` | — | 自动纳入逐项分析的无效理由之一 |

---

## 附录 A：相关源文件索引

| 文件 | 内容 |
|------|------|
| `guardrails/citation_table.go` | S1 静态主题表（第26条关键词） |
| `guardrails/citation_gate.go` | 引用核验门（双级核验逻辑） |
| `guardrails/guideline_source.go` | 审查指南主题索引（20201 充分公开） |
| `domains/rules/data/rules/patent-core.yaml` | YAML 规则（patent-a26.3-disclosure） |
| `domains/rules/engine.go` | 法条判断框架工具（A26.3） |
| `domains/rules/oa_parser.go` | OA 驳回类型解析（OaDisclosure） |
| `domains/legal_intent.go` | 法律意图分类（充分公开→无效宣告） |
| `domains/case_extractor.go` | 案件要素提取（公开不充分） |
| `domains/reasoning/manifest.go` | 无效宣告编排（含第26条第3款） |
| `workflows/patent/rule_engine.go` | 确定性规则引擎（DISCLOSURE-SUFFICIENCY） |
| `pkg/lawcite/lawcite.go` | 法条引用结构化抽取 |
| `retrieval/citation.go` | 引用追溯与格式化 |
| `evaluate/benchmark/patent_exam_real_a26.go` | A26 评测基准用例 |

## 附录 B：变更记录

| 日期 | 变更 |
|------|------|
| 2026-07-22 | 初始版本，从 Mady 知识系统全量检索并整理 |
