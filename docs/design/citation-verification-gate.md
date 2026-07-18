# 法条引用核验 Gate 设计方案（Citation Verification Gate）

> 状态：设计待评审 | 日期：2026-07-17
>
> 问题证据：`docs/evaluation-baseline-v0.8.md` §关键发现 2/4/5
>
> 涉及安全敏感路径对照：`guardrails/levels.go`（不动）、`domains/approval.go`（复用不改）、
> `agentcore/handoff.go`（不涉及）、`tools/path.go`（不涉及）

## 1. 背景与问题定义

v0.8 全量基线暴露了小模型时代最危险的一类错误：**法条编号幻觉**。

- `patent_exam_2008_a31_02`（分案申请专题）：模型回答引用「专利法第47条（分案申请）」
  「实施细则第20/21条」，全为幻觉——分案申请的正确依据是**实施细则第42条**，
  专利法第47条是无效宣告效力条款。该答案分析框架完整、行文流畅，
  **同模型自评 judge 给了 0.9 高分**，唯有 citation_completeness 规则抽取指标（0 分）
  拦住了它。
- L3 实验证明被动装配检索工具不能解决问题：31 题全程零触发，
  模型「不知道自己不知道」，不会主动去查。

这类错误在真实业务中的危害远大于「答案质量差」：**结论说对了方向、
引错了法条，专业用户一旦采信就是代理事故**。现有护栏体系（关键词触发免责声明、
审批 gate）对此完全无感——它按关键词工作，不理解引用内容。

**设计目标**：在答案送达用户之前，对其中引用的每一个法条编号做
「存在性 + 语境相关性」核验，存疑引用按护栏等级处置，全程留痕可审计。

## 2. 目标与非目标

### 目标

1. 拦截**编号错误**（引用了不存在或语境不符的法条），不误伤正常引用
2. 与现有 `guardrails.Level` 三档语义一致：Light 标注 / Standard 标注+留痕 / Strict 挂起待审
3. 核验结果进入评测体系：benchmark 新增 `citation_validity` 指标，
   复跑 P2A 31 题量化收益
4. 全程离线可用（本地知识库 + 规则），**不引入新的网络依赖**

### 非目标（明确排除）

- 不判断法律结论本身的正确性（那是 llm_judge 和专家盲测的职责）
- 不核验对比文件/专利号内容真实性（属于 patent_lookup 检索层问题）
- 不做实时法条时效性追踪（如 2024 细则修订差异），索引版本随知识库更新
- 不改动 `guardrails/levels.go` 等级枚举与 `domains/approval.go` 审批流程

## 3. 核心设计决策

### 决策一：核验什么——存在性不够，必须核验语境相关性

`2008_a31_02` 案例的关键教训：被引的「专利法第47条」**真实存在**，
纯存在性检查（编号 ≤ 法条总数）会通过它。错误在于**语境错配**——
答案声称它规定了分案申请，而它的实际主题是无效宣告效力。

因此核验分两级，缺一不可：

| 级别 | 检查 | 能拦住 | 拦不住 |
|------|------|--------|--------|
| R1 存在性 | 编号是否在法条有效范围内 | 凭空捏造的编号（如"专利法第208条"） | 张冠李戴 |
| R2 语境相关性 | 答案对该条的**用途声明**（括号注、
"根据…规定"从句）与该条的注册主题是否匹配 | 张冠李戴型幻觉（本案例型） | 引用正确但论证错误 |

### 决策二：核验源——三层降级，全部本地

```
┌─────────────────────────────────────────────────────┐
│ S1 内嵌静态主题表（编译进二进制，~60 高频条目）        │
│    专利法/实施细则高频条文 → 主题关键词组               │
│    覆盖考试与实务高频引用的 ~90% 场景                  │
├─────────────────────────────────────────────────────┤
│ S2 知识库法条索引（knowledge.Store，运行时构建）        │
│    从 wiki 拆分法条（专利法-2020-拆分-*、审查指南/*）    │
│    提取 条号→标题/关键词，loader 已有 LawRefs 提取     │
├─────────────────────────────────────────────────────┤
│ S3 语义检索兜底（retrieval 混合检索，P2 阶段）          │
│    用途声明与条文正文的 embedding 相似度 < 阈值 → 存疑   │
└─────────────────────────────────────────────────────┘
```

降级规则：S2/S3 不可用时静默退回 S1；S1 未覆盖的法条（冷门条目）
判「未知」而非「存疑」——**宁可漏报不可误报**（见 §6 误报控制）。

### 决策三：挂在哪——复用 LifecycleHook，不动等级枚举

新 hook `guardrails/citation_gate.go`（新文件，不碰 `levels.go`），
在 `AfterModelCall` 工作，与现有 guardrail 同相位、先执行：

```go
// 调用侧（domains/patent.go、domains/legal.go，各加一行）
cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
    guardrails.NewCitationGate(
        guardrails.WithCitationSource(citeSource),   // S1/S2 复合源
        guardrails.WithCitationLevel(guardrails.LevelStrict), // 与域护栏等级对齐
    ),
)
```

处置动作按等级（语义与现有 guardrail 完全对齐）：

| 等级 | 动作 | 类比现有行为 |
|------|------|-------------|
| Light | 答案末尾追加「引用核验」存疑清单（⚠️ 措辞见 §7） | 免责声明追加 |
| Standard | Light 动作 + 写 `disclosure` 留痕（新增 trigger=`citation_verify`） | RecordDecision 触点 |
| Strict | Standard 动作 + `SuppressPersist = true`，命中即入 ApprovalGate 待审队列 | 审批关键词挂起 |

### 决策四：引用抽取——抽出共享包，evaluate 与 gate 同源

现状：`agentcore/evaluate/metrics.go` 的 `citationPattern` /
`normalizeChineseNumerals` / `extractLawCitations` 是未导出实现，
且只认「第N条」不区分**哪部法**——这是现有指标的天花板
（"实施细则第42条"与"专利法第42条"无法区分）。

新增 `pkg/lawcite`（纯函数包，零依赖）：

```go
// Citation 表示一次法条引用，含所属法律与语境。
type Citation struct {
    Statute  Statute // PatentLaw / ImplementingRules / ExamGuideline / Unknown
    Article  int     // 条
    Paragraph int    // 款（0 = 未指明）
    Item     int     // 项（0 = 未指明）
    Context  string  // 引用点前后 ±40 字（用途声明提取窗口）
    Raw      string  // 原文
}

// Extract 从文本提取全部法条引用。支持：
//   "专利法第22条第3款" / "《专利法实施细则》第四十二条" /
//   "细则第68条"（简称归一）/ 中文数字 / "第22条之一"
// 承接语境规则：未指明法律时，沿用上文最近一次明确的 Statute。
func Extract(text string) []Citation
```

`evaluate/metrics.go` 随后改为调用 `lawcite.Extract`（行为等价重构，
citation_completeness 口径不变，单独一个 PR）——**评测与线上护栏
从此用同一套引用理解**，评测发现的幻觉模式就是护栏拦截的模式。

## 4. 核验流程（AfterModelCall 内）

```
模型输出 content
   │
   ▼
lawcite.Extract(content) ──→ 无引用？──→ 放行（绝大多数闲聊/流程性回复）
   │
   ▼ 逐条 Citation
┌──────────────────────────────────────────┐
│ R1 存在性：source.Lookup(statute, article) │
│   不存在 → 判 Invalid（幻觉编号）            │
│   存在   → R2                              │
│   未知法条（源未覆盖）→ 判 Unknown，放行      │
├──────────────────────────────────────────┤
│ R2 语境相关性：                             │
│   purpose := 提取用途声明（Citation.Context  │
│     中的括号注/"（…）"/"规定了…"从句）        │
│   purpose 为空 → 判 Unverifiable，放行      │
│   purpose 与注册主题关键词交集 = ∅           │
│     → 判 Suspect（张冠李戴）                │
└──────────────────────────────────────────┘
   │
   ▼ 汇总
全部 Valid → 放行
存在 Suspect/Invalid → 按等级处置（§3 决策三），
  处置记录 {caseID?, citations[], verdicts[], action}
  → disclosure（Standard+）
```

**性能预算**：Extract + R1/R2 全程正则 + map 查找，单答案 < 1ms，
无 LLM 调用、无网络——可以进 AfterModelCall 热路径。S3 语义检索（P2）
才引入 embedding，届时放异步路径，不阻塞输出。

## 5. 静态主题表（S1）示例与维护

```go
// guardrails/citation_table.go（生成文件，DO NOT EDIT）
var patentLawTopics = map[int][]string{
    22: {"新颖性", "创造性", "实用性", "现有技术"},
    26: {"说明书", "充分公开", "支持", "清楚"},
    31: {"单一性", "合案申请", "同样的发明构思"},
    33: {"修改", "超范围", "原说明书和权利要求书记载的范围"},
    45: {"无效宣告", "请求", "国务院专利行政部门"},
    47: {"无效宣告", "视为自始不存在", "效力"},  // ← 本案例题眼
    // ...约 40 条高频
}
var implementingRulesTopics = map[int][]string{
    42: {"分案申请", "原申请", "提出期限"},
    // ...约 20 条高频
}
```

- 主题词从知识库 wiki 拆分法条的标题/`核心要点` 半自动生成
  （`scripts/gen_citation_table.go`，人工核对后入库），
  **不是手写拍脑袋**；每次知识库更新可重新生成 diff 供审查
- 收录原则：只收主题无争议的条目；一词多义的条（如细则第20条
  新旧版含义不同）标注版本或剔除——漏报优于误报

## 6. 误报控制（本设计的成败点）

法律写作引用习惯多样，误报会直接摧毁专业用户信任。四条防线：

1. **Unknown/Unverifiable 一律放行**：源未覆盖、用途声明缺失时不做判断。
   预期拦截面只覆盖「明确声明了用途 + 用途明确不匹配」的高置信场景
2. **主题交集而非语义判决**：R2 用关键词交集（确定性），
   不用 LLM 判断（引入新幻觉源）；S3 语义相似度仅作 P2 增强信号
3. **措辞是「存疑」不是「错误」**（对照 `docs/tone-style-guide.md`）：
   输出标注统一为
   「⚠️ 引用核验提示：文中「专利法第47条」的用途描述（分案申请）与该条
   注册主题（无效宣告效力）不一致，请人工核对」——
   给出依据、把判断权留给专业人
4. **注入测试卡误报率**：benchmark 新增对抗用例（见 §8），
   正确引用必须 100% 放行，误报率 >0 即合入阻塞

## 7. 与现有契约的关系

| 契约 | 关系 |
|------|------|
| `guardrails.Level` | 复用三档语义，**不新增等级、不改枚举**（`levels.go` 零改动） |
| `domains.ApprovalGate` | Strict 档命中时复用其待审机制（`SuppressPersist` + 人工确认），零改动 |
| `disclosure` / `RecordDecision` | 新增 trigger 类别 `citation_verify`，schema 复用（P3 盲测的「证据引用错误」拒绝分类由此自动统计） |
| `knowledge.Store` / wiki loader | 只读消费；法条索引构建放 loader 完成钩子里 |
| `agentcore/evaluate` | `metrics.go` 重构改用 `pkg/lawcite`（口径不变）；新增 `CitationValidity` 指标 |
| Manifest 校验 | 不动；Gate 随域装配，非 manifest 声明 |

## 8. 评测方案

**新增指标 `citation_validity`**（`agentcore/evaluate/metrics.go`）：
对每题答案跑同一核验源，得分 = Valid 引用数 ÷ 总引用数
（Unknown/Unverifiable 不计入分母）。

**三层验证**：

1. **单元测试**：lawcite.Extract 覆盖（中文数字/简称/承接语境/之一）；
   R1/R2 判定矩阵；误报对抗集（50 条正确引用必须全放行）
2. **回放验证**：用 v0.8 缓存的 93 条真实答案（L0/L1/L3 各 31，
   已在 `$TMPDIR/mady_*_eval.json`）离线重放 Gate——
   预期：`2008_a31_02` 三层答案全部命中 Suspect（≥3 条），
   其余答案误报 = 0。**这是合入的硬性验收标准**
3. **P2A 复跑**：Gate 开启后全量 31 题三层复跑，
   观察 `citation_validity` 基线与通过率变化（Strict 挂起在
   跑批模式下自动降级为 Standard 标注，避免无人值守阻塞）

## 9. 分阶段实施

| 阶段 | 内容 | 文件炸弹半径 | 验收 |
|------|------|:---:|------|
| **P1a** | `pkg/lawcite` 抽取包 + 单测 | 2 新文件 | 单测全绿 |
| **P1b** | `guardrails/citation_gate.go` + S1 静态表 + Light/Standard 处置 + patent/legal 域接线 | 3 新 2 改 | 回放验证（§8.2）达标 |
| **P1c** | `evaluate/metrics.go` 改用 lawcite + `citation_validity` 指标 + P2A 复跑入 v0.9 报告 | 2 改 1 文档 | 指标口径等价 |
| **P2** | S2 知识库法条索引（loader 钩子）+ Strict 档 ApprovalGate 联动 + disclosure trigger | 3 改 | 端到端人工待审演练 |
| **P3** | S3 语义检索兜底（异步）+ 主题表自动生成脚本 | 2 新 | 存疑召回率复测 |

P1 三刀各自独立可合入；P2 之后才在 TUI/Server 暴露人工待审 UX。

## 10. 风险与开放问题

1. **静态表覆盖面**：S1 约 60 条能覆盖考试高频引用，但真实案件可能引
   冷门条目 → 全部落 Unknown 放行，Gate 在真实场景初期拦截率低，
   靠 P2 知识库索引补齐。**这是已接受的阶段性局限**
2. **承接语境归一错误**：长答案中"同上""该法"等指代可能归错 Statute →
   宁可判 Unverifiable 也不猜
3. **跑批口径**：Gate 上线后 v0.8 基线与新基线的 citation_completeness
   仍可比（同一 lawcite），但答案本身会因 Gate 标注而变化，
   v0.9 报告需重述可比性声明
4. **开放问题**：审查指南引用（"指南第二部分第八章 2.2 节"）是章节式
   而非条号式，R1/R2 模型不直接适配，P2 单独设计——
   本文暂不展开，标记 `[NEEDS CLARIFICATION]` 待评审时定夺
