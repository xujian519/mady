# 知识库增强落地实施计划

> 规范性子系统 + 案例性子系统 + 方法论性子系统 + Skill 蒸馏 四维协同增强

---

## 总体架构

```
                              ┌──────────────────┐
                              │   Skill 蒸馏系统   │
                              │  (Phase 4)        │
                              │  从优秀案例中提取  │
                              │  写作模式/技巧     │
                              └───────┬──────────┘
                                      │ 注入模式
          ┌───────────────────────────┼───────────────────┐
          │                           │                   │
          ▼                           ▼                   ▼
┌─────────────────┐   ┌─────────────────────┐   ┌─────────────────┐
│ 审查指南全文索引  │   │ 判决文书结构化解析器   │   │ 主动风险提示引擎  │
│ (Phase 1)       │   │ (Phase 2)            │   │ (Phase 3)       │
│ FTS5+Vector双索 │   │ 案号/法院/争议焦点/   │   │ 特征组合→相似    │
│ 引+GuidelineRule│   │ 裁判理由/结论结构化    │   │ 案例→风险评分    │
│ 图节点           │   │ +KG节点+SIMILAR_TO边  │   │ →主动预警        │
└────────┬────────┘   └──────────┬──────────┘   └────────┬────────┘
         │                      │                        │
         ▼                      ▼                        ▼
┌─────────────────────────────────────────────────────────────────┐
│                      已有基础设施层                               │
│  knowledge/graph(图谱) + knowledge/sqlite(FTS5+Vector)           │
│  + retrieval/(RRF混合检索) + domains/reasoning/(五步法推理)       │
│  + guardrails/(引用核验Gate)                                      │
└─────────────────────────────────────────────────────────────────┘
```

### 依赖关系

```
Phase 1 (审查指南索引) ──独立── 仅依赖已有基础设施
Phase 2 (文书解析器)   ──独立── 仅依赖已有基础设施
Phase 3 (风险提示)     ──依赖── Phase 1 (语义检索) + Phase 2 (结构化案例)
Phase 4 (Skill蒸馏)   ──依赖── Phase 2 (案例原料) + 已有 retrieval/ + skill/
```

**执行策略**：Phase 1 与 Phase 2 并行推进；Phase 3 需前两者完成后启动；Phase 4 与 Phase 3 可部分并行（Phase 2 完成后即可开始）。

---

## Phase 1：审查指南全文索引构建（预计 3-4 天）

### 1.1 要解决的问题

当前 `LawArticleIndex` 仅索引专利法 2020 拆分文件（82 条），《专利审查指南（2023修订）》全文——包括第二部分（实质审查）、第三部分（PCT）、第四部分（复审与无效）等——没有任何原文索引。用户询问"审查指南关于创造性三步法怎么规定的"，Agent 只能通过规则引擎的 `patent-core.yaml` 返回摘要信息，无法返回审查指南原文段落。

### 1.2 设计

#### 新增文件

| 文件 | 职责 |
|------|------|
| `knowledge/loader/guideline_loader.go` | 审查指南 Markdown 遍历、章节结构解析、分块导入 |
| `knowledge/loader/guideline_loader_test.go` | 解析和导入测试 |

#### 数据结构

```go
// GuidelineSection 表示审查指南中的一个章节单元（如 "第二部分第三章第3.2.1节"）
type GuidelineSection struct {
    DocID       string   // 文档ID（如 "guideline_2023_part2_ch3_3.2.1"）
    Title       string   // 章节标题
    Part        string   // 第几部分
    Chapter     string   // 第几章
    Section     string   // 节号（如 "3.2.1"）
    Content     string   // 章节正文
    Keywords    []string // 自动提取的关键词
    RefArticles []string // 引用的法条列表
}
```

#### 复用模式

- **目录遍历**: `WikiLoader` 的 `ImportWiki()` 模式（入口遍历 → 过滤 → 解析 → 入库）
- **分块**: `retrieval.ChunkDocument()`（按章节边界分块，MaxChars=2000, Overlap=200）
- **FTS 索引**: `knowledge/sqlite/writable.go` 的 `AddDocument()` 模式
- **知识图谱**: `knowledge/graph/builder.go` 的 `Build()` 方法，node type=`GuidelineRule`

#### 流程图

```
审查指南2023修订版全文（按部分/章/节拆分的Markdown文件）
  → guideline_loader.go
     ├─ 正则解析："第X部分"、"第X章"、"[节号]" 标题层级
     ├─ GuidelineSection 结构化提取
     └─ Store.AddDocument()（分块→FTS5索引→向量化→入库）
  → GraphBuilder.Build()
     ├─ 创建 GuidelineRule 节点（Content=200字摘要, AuthorityWeight=0.8）
     ├─ APPLIES 边：GuidelineRule → 引用的 LawArticle 节点
     └─ HAS_PRECEDENT 边：GuidelineRule → 依据其做出的 Case/Judgment
```

### 1.3 任务清单

- [ ] 1.1 `guideline_loader.go`：实现审查指南 Markdown 的章节层级解析（正则提取 Part/Chapter/Section）
- [ ] 1.2 `guideline_loader.go`：实现 `LoadGuideline(dir) ([]Document, error)` 批量导入
- [ ] 1.3 `guideline_loader.go`：自动提取每章节的 `RefArticles`（引用的法条号）和 `Keywords`
- [ ] 1.4 集成测试：加载示例审查指南片段，验证 FTS5 可检索到具体段落
- [ ] 1.5 图节点集成：验证 GuidelineRule 节点正确创建，APPLIES 边连接到对应 LawArticle
- [ ] 1.6 补充 `knowledge/loader/law_index.go`：将索引范围从仅 `专利法-2020-拆分-` 扩展到 `审查指南-` 文件
- [ ] 1.7 更新 `factTypeStrategies`（在 `domains/reasoning/walker.go`）：在 legal 事实类型策略中添加 `GuidelineRule` 目标节点类型

### 1.4 验证检查清单

- ☐ `go test ./knowledge/loader/...` 通过
- ☐ 运行 `guideline_loader_test.go` 的 TestLoadGuideline，确认章节解析正确（Part/Chapter/Section 全部到位）
- ☐ 通过 `search_knowledge "三步法 区别特征 技术问题"` 可检索到审查指南原文段落
- ☐ 知识图谱中存在 `GuidelineRule` 类型节点
- ☐ 审查指南节点通过 `APPLIES` 边连接到对应的专利法条节点（如"审查指南第二部分第三章"→"专利法第22条第2款"）

---

## Phase 2：判决文书结构化解析器（预计 3-4 天）

### 2.1 要解决的问题

当前 `legal_loader.go`（2KB）和 `patent_loader.go`（2.5KB）过于简陋，复审无效决定书和判决文书通过通用流程导入，丢失结构化信息。无法从判决文书中提取裁判要点、争议焦点、法律适用等结构化字段。无法支持"找到争议焦点与原案相同的判例"或"引用与本案相同的法条组合"等精确查询。

### 2.2 设计

#### 新增文件

| 文件 | 职责 |
|------|------|
| `knowledge/loader/judgment_parser.go` | 判决文书/复审无效决定书结构化解析 |
| `knowledge/loader/judgment_parser_test.go` | 解析器测试 |
| `knowledge/loader/judgment_loader.go` | 批量导入管线（parse→chunk→embed→store→graph） |
| `knowledge/loader/judgment_loader_test.go` | 导入管线测试 |

#### 数据结构

```go
// Judgment 表示一份结构化判决文书或复审无效决定书
type Judgment struct {
    DocID         string                // 文档ID
    CaseNumber    string                // 案号（如 "（2023）最高法知民终XXX号"）
    DecisionType  string                // 决定类型：judgment(判决) / reexamination(复审) / invalidation(无效)
    Court         string                // 法院/机构名称
    DecisionDate  string                // 决定日期
    Parties       []Party               // 当事人
    DisputedIssues []DisputedIssue      // 争议焦点
    LegalBasis    []string              // 适用法条
    Reasoning     string                // 裁判理由（全文）
    Conclusion    string                // 裁判结论/决定结论
    PrevInstances []string              // 前审案号
    CitedCases    []string              // 引用案例
}

type Party struct {
    Name   string // 当事人名称
    Role   string // 专利权人/请求人/原告/被告
}

type DisputedIssue struct {
    IssueID   int    // 序号
    Title     string // 争议焦点标题
    Claim     string // 请求方主张
    Defense   string // 答辩方主张
    Finding   string // 法院/合议组认定
    LegalRefs []string // 引用的法条/审查指南条款
}
```

#### 复用模式

- **文档模型**: `knowledge.Document` + `ParsedMetadata`（已有 `CaseNumber/Court/DecisionNum` 字段可直接复用）
- **分块**: `retrieval.ChunkDocument()`，但对争议焦点每个评述段落独立分块（保留结构边界）
- **重排序**: `retrieval/rerank.go` 的 `PatentReranker`（已有 `doc_type_rank["复审无效决定"]=65`）
- **图谱构建**: `GraphBuilder` 自动建立 `SIMILAR_TO` 边（共享法条引用的文书互相关联）
- **采集器**: `collector/documents.go` 的模式（解析→结构化→FactBlackboard 注入）

#### 流程图

```
复审无效决定书/判决文书（目录）
  → judgment_loader.go
     ├─ 文件遍历（按目录分类：reexam/ /专利侵权/ /专利判决/）
     ├─ 文件类型检测（PDF → textract, 纯文本 → 直接解析）
     ├─ JudgmentParser.Parse() 结构化解析
     │   ├─ 案号提取（正则 "(（\d{4}）[^（]+号)"）
     │   ├─ 争议焦点提取（"本案争议焦点为：" 等模式标记）
     │   ├─ 法条引用提取（全文中出现的法条号）
     │   └─ 裁判理由分段（按 "本院认为" / "合议组认为" 分段）
     └─ Store.AddDocument()（每段分块→FTS5→向量化→入库）
  → GraphBuilder.Build()
     ├─ 创建 Judgment/Case 节点
     ├─ CITES 边：Judgment → 引用的 LawArticle / GuidelineRule
     ├─ CITED_BY 边：Judgment → 被案例引用
     ├─ HAS_PRECEDENT 边：上级法院 → 下级法院（先例链）
     └─ SIMILAR_TO 边：共享 >=2 个法条引用 → 自动关联
```

### 2.3 任务清单

- [ ] 2.1 `judgment_parser.go`：实现案号正则提取（支持中文括号和数字）
- [ ] 2.2 `judgment_parser.go`：实现争议焦点分段提取（"争议焦点"、"本院认为"、"合议组认为"等标记）
- [ ] 2.3 `judgment_parser.go`：实现法条引用提取（全文扫描法条模式）
- [ ] 2.4 `judgment_parser.go`：实现当事人信息和裁判结论提取
- [ ] 2.5 `judgment_loader.go`：批量导入管线（目录扫描→类型检测→解析→分块→染色→入库）
- [ ] 2.6 图谱集成：在 `GraphBuilder` 中添加对 Judgment 节点的 CITES/CITED_BY/HAS_PRECEDENT 边构建
- [ ] 2.7 收集器集成：在 `collector/documents.go` 中添加对 Judgment 类型的结构化提取支持
- [ ] 2.8 更新 `factTypeStrategies`：在 precedent 事实类型策略中添加 `Judgment` 和 `Case` 目标节点

### 2.4 验证检查清单

- ☐ `go test ./knowledge/loader/...` 通过
- ☐ 解析一份样本文书：案号、法院、当事人、争议焦点、法条引用、结论全部正确提取
- ☐ 导入后，知识图谱中存在 Judgment 类型节点
- ☐ 通过 `search_knowledge "争议焦点 创造性 三步法"` 可检索到文书中的争议焦点段落
- ☐ `QueryCitationChain("专利法第22条第3款")` 返回引用该法条的案例列表
- ☐ 共享同一法条引用（如同时引用 A22.2 + A22.3）的文书之间建立了 SIMILAR_TO 边

---

## Phase 3：主动风险提示引擎（预计 4-5 天）

### 3.1 要解决的问题

现有知识图谱能被动响应查询（用户问→搜→答），但缺少主动扫描和预警能力。用户在撰写权利要求或分析专利时，系统不会主动提示"这种特征组合在历史上曾被无效宣告过"——需要用户自己想到去查。

### 3.2 设计

#### 新增文件

| 文件 | 职责 |
|------|------|
| `knowledge/risk/risk_scanner.go` | 特征组合风险扫描引擎 |
| `knowledge/risk/risk_scanner_test.go` | 扫描引擎测试 |
| `knowledge/risk/risk_types.go` | 风险数据类型定义 |
| `knowledge/risk/risk_extension.go` | agentcore Extension（注入 Agent 行为） |
| `knowledge/risk/risk_renderer.go` | 风险报告格式化输出 |

#### 数据结构

```go
// RiskSignal 是一个风险信号
type RiskSignal struct {
    ID             string    `json:"id"`
    Type           RiskType  `json:"type"`           // feature_combination / claim_scope / term_clarity
    Severity       Severity  `json:"severity"`       // high/medium/low
    Title          string    `json:"title"`          // "涉及功能性限定的无效风险"
    Description    string    `json:"description"`    // 风险描述
    RelatedCases   []string  `json:"related_cases"`  // 相关案例案号
    CaseCount      int       `json:"case_count"`     // 历史案例数
    InvalidRate    float64   `json:"invalid_rate"`   // 历史无效率 (0-1)
    FeatureTags    []string  `json:"feature_tags"`   // 特征标签
    Recommendation string   `json:"recommendation"` // 建议
    Authoritative  bool      `json:"authoritative"`  // 是否有权威依据
}

type RiskType string
const (
    RiskFeatureCombination RiskType = "feature_combination" // 特征组合风险
    RiskClaimScope         RiskType = "claim_scope"         // 保护范围风险
    RiskTermAmbiguity      RiskType = "term_ambiguity"      // 术语不清楚
    RiskSupport            RiskType = "support"             // 得不到说明书支持
)
```

#### 复用模式

- **图谱查询**: `knowledge/graph/query.go` 的 `QuerySimilar()` 和 `QueryNeighbors()`
- **图增强**: `knowledge/graph/retrieval_enhancer.go` 的 `GraphEnhancer.Enhance()`
- **语义检索**: `retrieval/` 的 FTS5+Vector+RRF 混合检索
- **Agent 集成**: `agentcore.LifecycleHook`（AfterModelCall 触发），类似 `guardrails/citation_gate.go`
- **护栏注入**: `guardrails/` 的写法（system prompt 注入风险提示块）

#### 流程图

```
用户描述的技术特征 / 权利要求
  → risk_scanner.go
     ├─ Step 1: 特征分解（LLM提取特征关键词，如 "功能性限定+参数限定"）
     ├─ Step 2: 组合检索（FTS5 OR 组合条件 → 历史案例）
     │   ├─ FTS5: ("功能性限定" AND "参数限定") → 命中 N 条案例
     │   ├─ KG: QuerySimilar(特征节点) → 关联案例 M 条
     │   └─ RRF融合 → 按相关度排序
     ├─ Step 3: 统计计算
     │   ├─ 历史案例总数：N+M
     │   ├─ 被无效案例数：count(结论=="宣告无效")
     │   └─ 无效率 = count(无效) / (N+M)
     ├─ Step 4: 生成 RiskSignal（严重度判定：无效率≥50%→high, ≥20%→medium, else→low）
     └─ Step 5: 格式化风险报告
  → RiskExtension (agentcore.LifecycleHook)
     ├─ AfterModelCall: 检查 Agent 输出是否涉及权利要求分析
     ├─ 如果是，触发风险扫描（异步，不阻塞）
     └─ 将扫描结果注入 system prompt 末尾的 "⚠️ 风险提示" 区块
```

#### 触发时机

1. **主动触发**: 用户在上下文中提到权利要求分析、专利撰写、无效请求时
2. **被动触发**: `AfterModelCall` 钩子检测到 Agent 输出中包含 "权利要求"、"特征"、"无效" 等关键词
3. **批处理**: 手动调用 `/risk-scan [文件/特征]` 命令

### 3.3 任务清单

- [ ] 3.1 `risk_types.go`：定义 RiskSignal、RiskType、Severity 等类型和常量
- [ ] 3.2 `risk_scanner.go`：实现 `analyzeFeatureCombination(features []string) ([]RiskSignal, error)`
  - 基于 FTS5 多词 OR 检索历史案例
  - 融合 KG 相似案例
  - 计算统计指标（案例数/无效率）
- [ ] 3.3 `risk_scanner.go`：实现 `ScanClaims(claimsText string) ([]RiskSignal, error)`
  - 调用 LLM 从权利要求文本提取特征组合
  - 调用 `analyzeFeatureCombination` 扫描
- [ ] 3.4 `risk_renderer.go`：实现风险报告格式化（markdown + 置信度标注 + 案例来源）
- [ ] 3.5 `risk_extension.go`：agentcore Extension 集成
  - `SystemPromptSuffix()`：注入风险提示参考指令
  - 注册工具：`risk_scan`、`risk_history`
  - `LifecycleHook`：AfterModelCall 关键词触发扫描
- [ ] 3.6 护栏集成：在 `guardrails/` 末尾添加风险提示区块模板

### 3.4 验证检查清单

- ☐ `go test ./knowledge/risk/...` 通过
- ☐ 输入特征组合 "功能性限定+参数限定" → 返回风险信号（含关联案例数+无效率）
- ☐ 风险报告中包含关联案例案号和来源链接
- ☐ 报告格式包含严重度标签（high/medium/low）和建议
- ☐ 当 Agent 输出涉及权利要求分析时，System Prompt 末尾自动注入风险提示区块
- ☐ `risk_scan` 工具可被 Agent 正常调用并返回结构化结果

---

## Phase 4：Skill 蒸馏机制（预计 6-8 天——核心工程）

### 4.1 要解决的问题

当前 `SKILL.md` 是静态手写的，描述了"做什么"（五步法流程/边界/审批节点），缺少"怎么写得好"的具体写作技巧。Styles 定义了 anti-patterns（什么不能写），但没有告诉 Agent"应该怎么写得专业、有说服力"。

### 4.1b 素材来源确认

已在 `/Users/xujian/工作/01_专利申请/` 目录发现大量可直接转化为 WritingPattern 的素材：

| 素材类型 | 文件示例 | 可提炼的模式 |
|---------|---------|-------------|
| **撰写规则映射** | `山东大齐4件/W02_撰写规则映射.md` | 完整的实用新型撰写模式（法条映射→权利要求结构→说明书五部分→禁止事项→IPC分类），天然就是 WritingPattern 的雏形 |
| **规则探索器输出** | `山东大齐4件/W02_rule_explorer_output.md` | 规则引擎生成的撰写约束（按域分类：patent_claims/patent_disclosure/patent_general），含生成式 Prompt |
| **专利说明书** | `孙俊霞1件/专利说明书_盐碱地苜蓿幼苗保护罩.md` | 背景技术结构（问题层次化→对比表→急需解决）、发明内容三要素 |
| **项目工作记录** | `新华医疗/xiaonuo.md` | Agent 行为纪律、决策记录模式（可提炼为 agent 写作规范） |
| **OA 答复目录** | `曹新乐_20251217/06_审查答复/` | OA 答复的目录结构和文件组织规范 |

**提取策略**：W02 文件是最佳起点——它们已经是"结构化的写作规则集"，只需从"案卷特定"泛化为"通用模式"。

### 4.2 设计

#### 整体架构

```
优秀案例语料库（OA答复/权利要求/无效请求书/审查意见）
  │
  ▼
Pattern Extractor（LLM辅助提取）
  ├─ 输入：案例文本 + 案例质量评分（已打标签的高质量样本）
  ├─ 分析维度：
  │   ├─ 论证结构：如何使用三段论/三步法
  │   ├─ 说理技巧：如何引证、如何反驳
  │   ├─ 措辞选择：专业术语使用、语气把控
  │   └─ 结构组织：段落分布、逻辑过渡
  └─ 输出：WritingPattern 结构化条目
  │
  ▼
WritingPatternStore（知识图谱+语义库持久化）
  ├─ 通过 AddDocument() 持久化到 knowledge.db
  ├─ FTS5 索引（可检索）
  ├─ 向量索引（语义搜索）
  └─ 知识图谱节点（PatternNode + APPLIES_TO 边）
  │
  ▼
Pattern Retriever（检索匹配）
  ├─ 输入：当前案件特征（CaseType + 特征描述）
  ├─ 匹配：FTS5 + Vector + 规则匹配
  └─ 输出：Top-K 相关 WritingPattern
  │
  ▼
Skill Compiler（技能编译器）
  ├─ 输入：匹配到的 WritingPattern
  ├─ 编译为：动态 SKILL.md 格式（<writing_skills> XML 块）
  └─ 注入：Agent System Prompt
  │
  ▼
Quality Evaluator（质量评估反馈）
  ├─ 输入：Agent 输出文本
  ├─ 与已有 WritingPattern 对比评估
  ├─ 评分：结构完整性/论证力度/措辞精确性
  └─ 输出：质量报告 + 改进建议
```

#### 新增文件

| 文件 | 职责 |
|------|------|
| `domains/writing/pattern_types.go` | WritingPattern 核心类型定义 |
| `domains/writing/pattern_store.go` | 模式存储/检索（FTS5+Vector） |
| `domains/writing/pattern_extractor.go` | 从案例提取写作模式（LLM辅助） |
| `domains/writing/skill_compiler.go` | 将模式编译为 Agent 可注入的指令块 |
| `domains/writing/quality_evaluator.go` | 输出质量评估（与已有模式对比） |
| `domains/writing/writing_extension.go` | agentcore Extension 集成 |
| `domains/writing/pattern_types_test.go` | 类型和存储测试 |
| `domains/writing/pattern_extractor_test.go` | 提取器测试 |
| `domains/writing/skill_compiler_test.go` | 编译器测试 |

#### 核心数据结构

```go
// WritingPattern 是一个可应用的写作模式/技巧
type WritingPattern struct {
    ID          string    `json:"id"`          // "wp-oa-inventiveness-3step-defend"
    Name        string    `json:"name"`        // "创造性三步法答辩框架"
    Category    string    `json:"category"`    // "oa_response" / "claim_writing" / "invalidation"
    SubCategory string    `json:"sub_category"`// "inventiveness_defense"
    Summary     string    `json:"summary"`     // 模式一句话说明
    Context     string    `json:"context"`     // 适用场景
    Steps       []Step    `json:"steps"`       // 步骤框架
    Examples    []Example `json:"examples"`    // 示例片段
    Dos         []Principle `json:"dos"`       // 应该遵循的原则
    Donts       []Principle `json:"donts"`     // 应该避免的错误
    SourceRef   string    `json:"source_ref"`  // 来源案例引用
    Quality     float64   `json:"quality"`     // 模式质量分 (0-1)
    Version     int       `json:"version"`     // 版本号（支持迭代优化）
}

type Step struct {
    Order       int    `json:"order"`
    Name        string `json:"name"`        // "确定最接近的现有技术"
    Instruction string `json:"instruction"` // 怎么写这一步的指导
    Example     string `json:"example"`     // 示例文本
}

type Example struct {
    Context string `json:"context"` // 示例上下文
    Text    string `json:"text"`    // 示例文本
    Note    string `json:"note"`    // 为什么这样写
}

type Principle struct {
    Rule    string `json:"rule"`    // 原则描述
    Example string `json:"example"` // 好/坏的对比示例
}
```

#### 三阶段蒸馏流程

**第一阶段：手工 Crafting（启动期）**

```
步骤: 人工选择 5-10 个高质量案例 → 编写 WritingPattern
方法: 人写 YAML → 注册到 pattern_store
目的: 快速建立 seed patterns，让系统先跑起来
产出: ~10 个精心编纂的 WritingPattern
```

**第二阶段：LLM 辅助提取（核心能力）**

```
步骤: PatternExtractor 分析案例 → 建议 WritingPattern
方法:
  1. 输入案例全文 + 质量标签（通过 quality_evaluator 评分预筛选）
  2. LLM 分析：论证结构/说理技巧/措辞风格
  3. 提取可复用的写作模式
  4. 人审核 → 确认/修改/拒绝 → 入库
目的: 规模化提取
产出: ~50-100 个模式
```

**第三阶段：自动匹配与反馈（闭环）**

```
步骤: 系统自动匹配模式 → Agent 应用 → 质量评估 → 反馈
方法:
  1. Agent 输出时，QualityEvaluator 自动评分
  2. 高分输出：提取新模式建议
  3. 低分输出：检查哪些模式被忽略了？提示改进
目的: 持续闭环改进
产出: 动态更新的模式库 + 质量趋势
```

#### 注入机制

**设计决策（用户确认）**：第一阶段以 **tool-queryable 资源** 形式提供，Agent 在需要写作指导时主动调用 `query_writing_patterns` 工具。待模式库成熟、用户反馈确认质量后，再逐步将高频匹配的模式注入 System Prompt。

```
                             ┌──────────────┐
                             │ Agent 写作任务  │
                             └──────┬───────┘
                                    │ 需要写作指导？
                                    │
                              ┌─────▼──────┐
                              │  是/不确定   │
                              └─────┬──────┘
                                    │ 调用 query_writing_patterns
                                    ▼
                     ┌──────────────────────────┐
                     │ WritingPatternStore       │
                     │ → MatchPatterns(case,     │
                     │   feature) → Top-3 模式   │
                     └──────────────────────────┘
                                    │
                                    ▼
                     ┌──────────────────────────┐
                     │ Agent 阅读模式并应用      │
                     │ （不作为强制约束）         │
                     └──────────────────────────┘
                                    │
                     （用户评价阶段）
                                    │
                                    ▼
                     ┌──────────────────────────┐
                     │ quality_evaluator 评分    │
                     │ + 用户反馈 → ground truth │
                     │ → 模式库迭代优化          │
                     └──────────────────────────┘
```

**成熟度门槛**（达到后才注入 System Prompt）：
1. 模式库 ≥ 30 个模式
2. 用户反馈 ≥ 50 条（其中 excellent/good ≥ 40 条）
3. 自动评分与用户评分的一致性 ≥ 80%

```xml
<!-- Skill Compiler 生成的写作指令块，注入到 Agent System Prompt -->
<writing_skills>...</writing_skills>```

#### 与已有机制的集成

| 已有机制 | 集成方式 |
|---------|---------|
| `skill/skill.go` | Skill Compiler 的输出作为动态 Skill，通过 `ActivePrompt()` 注入 |
| `styles/*.yaml` | WritingPattern 中提取的 Dos/Donts 可以作为风格指南的补充成分 |
| `domains/reasoning/syllogism.go` | WritingPattern 可以指导 Agent 如何构建三段论的每一个前提 |
| `domains/reasoning/five_step_runner.go` | WritingPattern 在 Stage 4（执行）阶段注入 |
| `prompt-templates/*.json` | WritingPattern 可以作为模板中的 writing_guidance 字段 |
| `knowledge/graph/` | WritingPattern 节点 + APPLIES_TO 边连接到适用案例 |

### 4.3 任务清单

#### 第一阶段：基础设施搭建

- [ ] 4.1 `pattern_types.go`：定义 WritingPattern、Step、Example、Principle 等核心类型
- [ ] 4.2 `pattern_store.go`：实现基于 knowledge.db 的模式存储（FTS5 + Vector + 语义检索）
  - 复用 `knowledge/sqlite/writable.go` 的 `AddDocument` 模式
  - `StorePattern(p WritingPattern) error`
  - `SearchPatterns(query string, category string, topK int) ([]WritingPattern, error)`
- [ ] 4.3 知识图谱集成：在 `knowledge/graph/types.go` 中添加 `NodeWritingPattern = "WritingPattern"` 和 `RelAppliesTo = "APPLIES_TO"` 关系类型

#### 第二阶段：Seed 模式编纂（手工 + LLM辅助）

- [ ] 4.4 编写首批 10 个种子 WritingPattern（YAML 格式，人工编纂）
  - 文件位置：`domains/writing/seed-patterns/`
  - **素材来源**：`/Users/xujian/工作/01_专利申请/` 目录
    - `山东大齐4件/W02_撰写规则映射.md` → 泛化为"实用新型撰写规则映射"模式
    - `山东大齐4件/W02_rule_explorer_output.md` → "规则引擎输出驱动的撰写约束"模式
    - `孙俊霞1件/专利说明书_盐碱地苜蓿幼苗保护罩.md` → "背景技术撰写（问题层次化+对比表）"模式
  - 覆盖场景（10 个种子）：
    1. **实用新型独立权利要求撰写** — 前序+特征两段式结构（从 W02 提炼）
    2. **从属权利要求分层策略** — 参数范围→控制细节→材料选择三层（从 W02 提炼）
    3. **说明书背景技术撰写** — 问题层次化+现有技术对比表+总结急需（从孙俊霞提炼）
    4. **发明内容三要素结构** — 技术问题+技术方案+有益效果
    5. **创造性三步法 OA 答复** — 最接近现有技术→区别特征→非显而易见性
    6. **新颖性单独对比 OA 答复** — 单独对比原则+特征逐项比对
    7. **不清楚/不支持 OA 答复** — 功能性限定的支持问题
    8. **技术交底书撰写** — PFE 三元组提取+九段式结构
    9. **IPC 分类与撰写策略关联** — 分类号驱动的撰写约束（从 W02 rule_explorer 提炼）
    10. **说明书具体实施方式撰写** — 至少一个实施例+具体参数
- [ ] 4.5 `pattern_extractor.go`：实现 LLM 辅助的模式提取
  - 输入：案例文本 + 质量评分
  - 输出：建议的 WritingPattern（需人工审核确认）
  - `ExtractPattern(caseText string, qualityScore float64) (*WritingPattern, error)`

#### 第三阶段：编译器与注入

- [ ] 4.6 `skill_compiler.go`：实现将 WritingPattern 编译为 Agent 可用指令
  - `CompileSkills(patterns []WritingPattern) string` → 输出 `<writing_skills>` XML 块
  - 按 Category 分组，注入标签
- [ ] 4.7 `skill_compiler.go`：实现 `MatchPatterns(caseType string, features []string) ([]WritingPattern, error)`
  - 当前案件的特征 → 检索匹配的 WritingPattern
  - FTS5 + 向量 + category 过滤 三路检索
- [ ] 4.8 `writing_extension.go`：agentcore Extension 集成
  - `SystemPromptSuffix()`：将匹配的 WritingPattern 注入 `<writing_skills>` 块
  - 注册工具：`list_patterns`、`get_pattern`、`extract_pattern`
  - 以 tool 形式提供，注册工具：`query_writing_patterns`（Agent 根据需要主动调用），`list_patterns`、`get_pattern`
  - 成熟后才注入 System Prompt（门槛：≥30模式 + ≥50用户反馈 + 评分一致率≥80%）

#### 第四阶段：质量评估闭环（以用户真实评价为最高准则）

**评估架构**：4 维度自动评分作为"建议值"，用户评价覆盖并修正自动评分。

```
Agent 输出 → QualityEvaluator 自动评分（建议值）
                  ↓
          用户审阅 → 给出 5 级评价（excellent/good/acceptable/needs_improvement/poor）
                  ↓
          用户评价写入 feedback store → 作为 ground truth
                  ↓
          自动评分和用户评分差异 > 阈值 → 调整 evaluator 参数
                  ↓
          用户评价为 excellent 的输出 → 建议提取新的 WritingPattern
```

- [ ] 4.9 `quality_evaluator.go`：实现输出质量自动评估
  - 初始评估维度（建议值，不标记为权威）：
    - 结构完整性（是否有完整的论证结构）
    - 引用精确度（法条和案例引用是否正确）
    - 论证力度（是否有实质说理还是空泛断言）
    - 专业术语（术语使用是否准确）
  - `Evaluate(output string, patterns []WritingPattern) (*QualityReport, error)`
- [ ] 4.10 `quality_evaluator.go`：实现用户反馈采集
  - `FeedbackStore` 结构：记录 { output_id, user_rating, user_comment, auto_score, timestamp, applied_patterns }
  - `CollectFeedback(outputID string, rating Rating, comment string) error`
  - 自动评分与用户评分的差异分析 → 定期生成校准报告
- [ ] 4.11 `quality_evaluator.go`：实现低分反馈
  - 低分输出 → 识别缺失的模式 → 提示改进方向
  - 用户评分为 poor 的输出 → 强制要求用户补充反馈（原因归类）

### 4.4 验证检查清单

- ☐ `go test ./domains/writing/...` 通过
- ☐ `StorePattern` 存储一个模式后，`SearchPatterns("三步法 答辩")` 可检索到
- ☐ 知识图谱中存在 `WritingPattern` 类型节点，并通过 `APPLIES_TO` 边连接到适用案例
- ☐ `skill_compiler.CompileSkills()` 输出的 XML 块语法正确，包含 `writing_skills` 标签
- ☐ `MatchPatterns("invalidation", ["功能性限定"])` 返回 Top-3 相关 WritingPattern
- ☐ 当 Agent 处理 OA 答复任务时，System Prompt 末尾自动注入创造性答辩 WritingPattern
- ☐ `quality_evaluator.Evaluate()` 对高质量 OA 答复输出建议评分 ≥ 80% （满分100%）
- ☐ `quality_evaluator.Evaluate()` 对低质量（空泛断言）输出建议评分 < 50%
- ☐ `CollectFeedback()` 写入 user_feedback 库，持久化（含 output_id + user_rating + timestamp）
- ☐ 用户评分 poor 的输出触发了强制补充反馈提示
- ☐ 自动评分与用户评分的一致性报告可正常生成
- ☐ 手工编纂的 5-10 个种子 WritingPattern 全部加载通过

---

## 四阶段迭代执行策略

```
第1周    第2周    第3周    第4周
├Phase1──┤
├Phase2──┤
         ├────Phase3────┤
         ├────Phase4-1────┤
                         ├Phase4-2──┤
                         ├Phase4-3──┤
```

| 周次 | 并行工作流 |
|------|-----------|
| 第1周 | Phase 1（审查指南索引）+ Phase 2（文书解析器）+ Phase 4-1（基础设施搭建） |
| 第2周 | Phase 1验证 + Phase 2验证 + Phase 3（风险提示引擎）+ Phase 4-1继续 |
| 第3周 | Phase 3继续 + Phase 4-2（Seed编纂+提取器）+ Phase 4-3（编译器） |
| 第4周 | Phase 3验证修复 + Phase 4-3、4-4（质量评估+集成） + 全量集成测试 |

### 关键里程碑

| 里程碑 | 时间 | 验证标准 |
|--------|------|---------|
| M1: 语料就绪 | 第1周末 | 审查指南全文可检索 + 判决文书结构化解析通过测试 |
| M2: 风险扫描 | 第2周末 | 特征组合风险扫描可返回 > 80% 准确率 |
| M3: 模式种子 | 第3周末 | 10 个种子 WritingPattern + 编译器输出正确 XML |
| M4: 全链路闭环 | 第4周末 | Agent 输出自动注入写作模式 + 质量评估反馈通道建立 |

---

## 统合验证总清单

### 集成测试清单

- ☐ `go test ./...` 全部通过（无编译错误、无竞态）
- ☐ `go test -race ./knowledge/...` 竞态检测通过
- ☐ Phase 1 + Phase 2 + Phase 3 + Phase 4 全数据流端到端测试：
  1. 审查指南可检索原文段落
  2. 判决文书结构化导入并建立图谱关联
  3. 特征组合输入 → 风险信号输出
  4. 案件特征输入 → 匹配 WritingPattern → Agent 行为变化

### 回归安全清单

- ☐ 已有 `domains/rules/engine.go` 的 search_rules 工具不受影响
- ☐ 已有 `guardrails/citation_gate.go` 的双级核验不受影响
- ☐ 已有 `knowledge/` 的文档存储和检索不受影响
- ☐ 已有 `domains/reasoning/five_step_runner.go` 的五步工作法编排不受影响
- ☐ 已有 `skill/skill.go` 的 SKILL.md 解析不受影响

### 边界情况

- ☐ 审查指南文件格式不符合预期（非标准 Markdown）→ 提供错误信息而非 panic
- ☐ 判决文书无争议焦点 → 跳过争议焦点提取，保留全文
- ☐ 风险扫描无历史案例命中 → 返回空列表而非错误
- ☐ WritingPattern 注入过多导致 context 溢出 → 限制注入模式数量 ≤ 5
- ☐ 低质量案例写入 pattern_store → QualityScore < 0.3 的模式不参与自动匹配
- ☐ 单个导入任务失败不影响其他批次继续导入

---

## 与现有知识体系的关系图

```
                         ┌────────────────────────┐
                         │     SKILL.md + MadyExt  │
                         │  (mode/guardrail/       │
                         │   capabilities/handoff) │
                         └────────┬───────────────┘
                                  │ 注入
          ┌───────────────────────┼───────────────────────┐
          │                       │                       │
          ▼                       ▼                       ▼
┌──────────────────┐  ┌────────────────────┐  ┌────────────────────┐
│ 规则引擎          │  │ 知识图谱           │  │ Skill 蒸馏         │
│ (domains/rules/) │  │ (knowledge/graph/) │  │ (domains/writing/) │
│                  │  │                    │  │                    │
│ patent-core.yaml │  │ LawArticle 节点    │  │ WritingPattern     │
│ ArticleFramework │  │ Case/Judgment 节点 │──┤ APPLIES_TO 边      │
│ CitationGate     │  │ GuidelineRule 节点  │  │ QualityEvaluator   │
│                  │  │ SIMILAR_TO 边      │  │ Skill Compiler     │
│ 【放之四海皆准】  │  │ CITES/APPLIES 边   │  │ 【怎么写得好】      │
└──────────────────┘  └────────┬───────────┘  └────────────────────┘
                               │
                               ▼
                    ┌────────────────────┐
                    │ 语义库             │
                    │ (knowledge/sqlite/ │
                    │  + retrieval/)     │
                    │                    │
                    │ FTS5 + Vector +    │
                    │ RRF 混合检索       │
                    │ ChunkDocument     │
                    │ Embedding         │
                    │ 【原文片段检索】    │
                    └────────────────────┘
```

---

## 各文件修改清单

### 新增文件（共 18 个）

| 文件 | 所属 Phases |
|------|------------|
| `knowledge/loader/guideline_loader.go` | P1 |
| `knowledge/loader/guideline_loader_test.go` | P1 |
| `knowledge/loader/judgment_parser.go` | P2 |
| `knowledge/loader/judgment_parser_test.go` | P2 |
| `knowledge/loader/judgment_loader.go` | P2 |
| `knowledge/loader/judgment_loader_test.go` | P2 |
| `knowledge/risk/risk_types.go` | P3 |
| `knowledge/risk/risk_scanner.go` | P3 |
| `knowledge/risk/risk_scanner_test.go` | P3 |
| `knowledge/risk/risk_renderer.go` | P3 |
| `knowledge/risk/risk_extension.go` | P3 |
| `domains/writing/pattern_types.go` | P4 |
| `domains/writing/pattern_store.go` | P4 |
| `domains/writing/pattern_extractor.go` | P4 |
| `domains/writing/skill_compiler.go` | P4 |
| `domains/writing/quality_evaluator.go` | P4 |
| `domains/writing/writing_extension.go` | P4 |
| `domains/writing/seed-patterns/*.yaml` | P4 |

### 修改文件（共 8 个）

| 文件 | 修改内容 | 所属 |
|------|---------|------|
| `knowledge/graph/types.go` | 添加 `NodeWritingPattern` 和 `RelAppliesTo` 常量 | P4 |
| `knowledge/graph/builder.go` | 添加对 Judgment 节点的 CITES/CITED_BY 边构建 | P2 |
| `knowledge/loader/law_index.go` | 扩展文件前缀匹配支持审查指南 | P1 |
| `domains/reasoning/walker.go` | `factTypeStrategies` 添加 GuidelineRule/Judgment/Case 目标类型 | P1+P2 |
| `domains/reasoning/wiring/vector_rule_store.go` | 增加审查指南检索通道 | P1 |
| `domains/reasoning/collector/documents.go` | 添加 Judgment/DisputedIssue 结构化提取 | P2 |
| `knowledge/extension.go` | 注册 risk 和 writing 扩展 | P3+P4 |
| `cmd/mady/main.go` | 初始化 risk/writing 扩展 | P3+P4 |
