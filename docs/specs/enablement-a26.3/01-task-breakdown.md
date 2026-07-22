# 专利法第26条第3款判断模块 — 任务分解与验证清单

> 基于调研结论（`docs/rules/patent-law-article-26-3.md` / `patent-law-26-3-cases.md`）
> 和设计方案（`.claude/plans/tidy-mapping-zebra.md`）。
>
> 最后更新：2026-07-22

---

## 一、总体进度

| 阶段 | 任务数 | 已完成 | 待完成 |
|------|:------:|:------:|:------:|
| P1. 核心数据结构 | 3 | 3 | 0 |
| P2. Pregel 图引擎 | 3 | 3 | 0 |
| P3. 集成与触发 | 4 | 4 | 0 |
| P4. 工具与评测 | 5 | 3 | 2 |
| P5. 补全与加固 | 6 | 0 | 6 |
| **合计** | **21** | **13** | **8** |

---

## 二、可执行任务清单

### P1. 核心数据结构 ✅ 已完成

| # | 任务 | 产出 | 验收标准 | 状态 |
|---|------|------|----------|:----:|
| 1.1 | 创建 `domains/enablement/doc.go` | 包文档，标注 26.3/A26.3/充分公开 关键词 | 文件中含 "26.3" "充分公开" "enablement" 三个关键词 | ✅ |
| 1.2 | 创建 `domains/enablement/types.go` | EnablementInput/Result/CompletenessResult/ClarityResult/EnablementJudgment | 5 个 struct 均含 json tag；RequiredSectionCount()=5 | ✅ |
| 1.3 | 创建 `domains/rules/data/articles/patent-law-a26.3.yaml` | A26.3 法条判断框架 | get_article_framework("A26.3") 返回非 nil | ✅ |

### P2. Pregel 图引擎 ✅ 已完成

| # | 任务 | 产出 | 验收标准 | 状态 |
|---|------|------|----------|:----:|
| 2.1 | 创建 `domains/enablement/nodes.go` | 5 个 Pregel 节点实现 | 每个节点函数签名匹配 PregelNode | ✅ |
| 2.2 | 创建 `domains/enablement/graph.go` | BuildEnablementGraph() | 图编译成功（5 节点线性链） | ✅ |
| 2.3 | 创建 `domains/enablement/framework.go` | Framework + 降级默认框架 | go build 编译通过 | ✅ |

### P3. 集成与触发 ✅ 已完成

| # | 任务 | 产出 | 验收标准 | 状态 |
|---|------|------|----------|:----:|
| 3.1 | 创建 `server/enablement_events.go` | EnablementTrigger | 结构对表 InventivenessTrigger | ✅ |
| 3.2 | 修改 `cmd/mady/server.go` | 注册 EnablementTrigger | `go build` 编译通过 | ✅ |
| 3.3 | 修改 `server/server.go` | enablementResults + Set/Get 方法 | import 使用 enablement 包 | ✅ |
| 3.4 | 修改 `server/disclosure.go` | Enablement 字段 + REST/SSE 返回 | 两个路径均包含 enablement | ✅ |

### P4. 工具与评测（部分完成）

| # | 任务 | 产出 | 验收标准 | 状态 |
|---|------|------|----------|:----:|
| 4.1 | 创建 `domains/enablement/tool.go` | NewEnablementTool | 编译通过 | ✅ |
| 4.2 | 修改 `domains/patent.go` | 注册 evaluate_enablement 工具 | PatentAgentConfig 包含该工具 | ✅ |
| 4.3 | 创建 `evaluate/benchmark/patent_exam_real_a26_3.go` | 3 个 26.3 评测用例 | suite.go 注册了该用例集 | ✅ |
| 4.4 | 创建 `domains/enablement/enablement_test.go` | 单元测试（13 个用例） | `go test -count=1` 通过 | ✅ |
| 4.5 | 创建 `domains/enablement/testdata/enablement_cases.json` | 测试 fixture 数据 | 文件非空，含至少 2 个测试案例 | ❌ |

### P5. 补全与加固（待完成）

| # | 任务 | 产出 | 验收标准 | 状态 |
|---|------|------|----------|:----:|
| 5.1 | **补充 testdata/enablement_cases.json** | 结构化测试案例 | 含 2+ 案例，每案例含 features/pfe_triples/expected | ❌ |
| 5.2 | **增加 buildEnablementInput 单元测试** | 验证 disclosure→enablement 数据转换 | 覆盖完整/部分/空输入的转换路径 | ❌ |
| 5.3 | **增加 tool.go 单元测试** | 测试 parseEnablementArgs 和 NewEnablementToolFromReport | 正常 JSON / 格式错误 JSON / nil provider | ❌ |
| 5.4 | **验证 ArticleFramework 加载** | 端到端验证 rules.Engine 加载 A26.3 YAML | `engine.Article("patent-law-a26.3")` 返回完整框架 | ❌ |
| 5.5 | **编写 enablement SKILL.md** | 为智能体提供领域技能定义 | 文件含 name/description/domain/mode/护栏级别 | ❌ |
| 5.6 | **集成测试：disclosure → enablement 自动触发** | 端到端测试文档 | 提交交底书 → disclosure 完成 → enablement 自动运行 → API 返回结果 | ❌ |

---

## 三、验证检查清单（Checklist）

### A. 编译与静态检查

```
☐ go build ./...                               # 全项目编译零错误
☐ go vet ./domains/enablement/...               # 静态分析零警告
☐ go test ./domains/enablement/... -count=1     # 单元测试全部通过
☐ grep -r "TODO\|FIXME\|HACK" domains/enablement/  # 零未解决的 TODO
```

### B. 数据结构完整性

```
☐ EnablementInput 包含: Features / PFETriples / Problems / Effects / DocSections / HasDrawings / EvidenceCoverage
☐ EnablementResult 包含: Assessed / Skipped / Completeness / Clarity / Enablement / IsSufficient / Confidence / Deficiencies
☐ CompletenessResult 包含: MissingSections / Score / Notes
☐ ClarityResult 包含: IsClear / AmbiguousTerms / OrphanFeatures / OrphanEffects / Notes
☐ EnablementJudgment 包含: CanImplement / MissingKeyMeans / VagueMeans / OnlyTaskNoMeans / InsufficientData / Notes
☐ RequiredSectionCount() 返回 5
☐ SectionLabel(0) 非空 / SectionLabel(999) 为空
```

### C. Pregel 图正确性

```
☐ 5 个节点均已注册: load_input / step1_completeness / step2_clarity / step3_enablement / generate_conclusion
☐ 边链完整: load_input→step1→step2→step3→conclusion→__end__
☐ 跳过逻辑: 空输入→Skipped=true; nil 输入→Skipped=true; 0特征+0三元组→Skipped=true
☐ 有效输入: 3+ 特征正常传递到各步骤
☐ stateHasSkip() 正确检测 Skipped 状态
☐ JSON 解析: extractJSON 正确处理各种输入（嵌套/无JSON/空）
☐ 截断工具: truncateText 正确处理 ASCII 和 Unicode
```

### D. 集成点验证

```
☐ server.go: enablementResults 已初始化（csync.NewMap）
☐ server.go: SetEnablementResult / GetEnablementResult 方法已实现
☐ cmd/mady/server.go: EnablementTrigger 在 inventiveness 之后注册并 Start()
☐ disclosure.go: DisclosureTaskStatus 含 Enablement 字段（json tag）
☐ disclosure.go: REST 路径（GET /v1/disclosure/analyze/{task_id}）附加 enablement
☐ disclosure.go: SSE 路径附加 enablement
☐ patent.go: evaluate_enablement 工具已注册到 PatentAgentConfig
☐ benchmark/suite.go: PatentExamRealA26_3Cases 已注册
```

### E. 知识源验证

```
☐ ArticleFramework YAML 存在: domains/rules/data/articles/patent-law-a26.3.yaml
☐ YAML 包含 3 个步骤 + conclusionSchema + applicableTo
☐ 规则文档存在: docs/rules/patent-law-article-26-3.md (290行)
☐ 案例文档存在: docs/rules/patent-law-26-3-cases.md (253行)
☐ 案例文档含 13件无效决定 + 3道考试真题
☐ 案例文档含 4条裁判规则提炼
☐ 案例文档含 4个智能体使用场景指南
```

### F. 评测基准验证

```
☐ 3 个 26.3 评测用例已创建:
   ☐ patent_exam_2011_a26_3_chemical (化学催化剂-公开不充分成立)
   ☐ patent_exam_2012_a26_3_mechanical (机械滤网-公开充分成立)
   ☐ invalidation_a26_3_disclosure (CMOS传感器-无效理由被驳回)
☐ 每个用例包含: ID / Domain / Input / Expected / RequiredCitations
☐ RequiredCitations 包含 "专利法第26条第3款"
☐ 用例已注册到 suite.go 的 patentCases()
```

### G. 重复性检查（预防回归）

```
☐ go build ./... 在每次提交前通过
☐ go test ./domains/enablement/... 在每次提交前通过
☐ 未修改 disclosure/ 管线代码（松耦合）
☐ 未修改 inventiveness/ 模块代码
☐ 新增文件均在项目根模块内（无独立 go.mod）
☐ 无循环依赖（domains/enablement → agentcore + graph 是单向的）
```

---

## 四、待完成任务详情

### 5.1 testdata/enablement_cases.json

**目标**：为单元测试和集成测试提供结构化 fixture 数据。

**内容**：
```json
[
  {
    "id": "case_chemical_insufficient",
    "description": "化学案例-公开不充分",
    "input": {
      "features": [{"id":"f1","description":"取原料A和原料B","category":"method","function":"制备催化剂","importance":"high"}],
      "pfe_triples": [{"id":"t1","problem":"提高催化活性","feature_ids":["f1"],"effect":"活性提高30%"}],
      "problems": ["提高催化活性"],
      "effects": ["活性提高30%"],
      "doc_sections": {"technical_field":"催化剂技术领域","background":"现有催化剂活性低","content":"通过优化工艺参数提高活性","embodiments":"取原料A和B在高温下反应"},
      "has_drawings": false
    },
    "expected": {
      "is_sufficient": false,
      "confidence": "high",
      "deficiencies_min": 2
    }
  },
  {
    "id": "case_mechanical_sufficient",
    "description": "机械案例-充分公开成立",
    "input": {
      "features": [
        {"id":"f1","description":"壳体","category":"structure","function":"容纳组件","importance":"high"},
        {"id":"f2","description":"滤网","category":"structure","function":"过滤","importance":"high"},
        {"id":"f3","description":"电机驱动刮板","category":"method","function":"自动清洁","importance":"high"}
      ],
      "pfe_triples": [
        {"id":"t1","problem":"人工清洗费时费力","feature_ids":["f1","f2","f3"],"effect":"实现自动清洁"}
      ],
      "problems": ["人工清洗费时费力"],
      "effects": ["实现自动清洁"],
      "doc_sections": {"technical_field":"过滤设备","background":"现有滤网需人工拆卸","content":"通过机械刮板和反冲洗实现自动清洁","embodiments":"如图1所示，电机驱动刮板往复运动","drawings":"图1为装置结构示意图"},
      "has_drawings": true
    },
    "expected": {
      "is_sufficient": true,
      "confidence": "high",
      "deficiencies_min": 0
    }
  }
]
```

**验收**：`go test` 使用该 fixture 验证 JSON 解析和预期结果匹配。

### 5.2 buildEnablementInput 单元测试

**文件**：`domains/enablement/enablement_test.go`（追加）

**测试用例**：
- `TestBuildEnablementInput_FromDisclosureReport_Full` — 完整 disclosure.AnalysisReport 转换
- `TestBuildEnablementInput_FromDisclosureReport_Partial` — 只有 Extraction 无 Document
- `TestBuildEnablementInput_FromDisclosureReport_Nil` — nil report 处理
- `TestBuildEnablementInput_EvidenceCoverage` — 验证 EvidenceCoverage 自动提升逻辑

### 5.3 tool.go 单元测试

**文件**：`domains/enablement/tool_test.go`（新建）

**测试用例**：
- `TestParseEnablementArgs_Valid` — 正常 JSON 输入
- `TestParseEnablementArgs_Invalid` — 格式错误 JSON
- `TestParseEnablementArgs_Empty` — 空对象
- `TestNewEnablementTool_NoProvider` — 无 provider 时的错误处理
- `TestNewEnablementToolFromReport_Nil` — nil provider/report

### 5.4 ArticleFramework 加载验证

**验证方式**：
```go
// 在 test 中手工加载 engine 并验证 A26.3 框架
engine := rules.LoadFromDir("domains/rules/data/")
af := engine.Article("patent-law-a26.3")
assert.NotNil(t, af)
assert.Equal(t, 3, len(af.Steps))  // 三步判断
assert.Equal(t, "patent-law-a26.3", af.ArticleID)
```

### 5.5 enablement SKILL.md

**文件**：`skills/enablement/SKILL.md`（新建）

**内容模板**：
```yaml
---
name: enablement-evaluation
description: 专利法第26条第3款（充分公开/可实现性）评估
domain: patent
mode: patent
guardrail: strict
approval: true
mady:
  keywords: ["26.3", "A26.3", "充分公开", "公开不充分", "能够实现", "enablement"]
  article: "patent-law-a26.3"
  tool: "evaluate_enablement"
---
```

### 5.6 集成测试

**测试流程**：
1. 启动 Mady Server（带 provider + EventBus）
2. POST `/v1/disclosure/analyze` 提交测试交底书
3. 轮询 `GET /v1/disclosure/analyze/{task_id}` 直到 status=completed
4. 验证 response.enablement 非空
5. 验证 response.enablement.is_sufficient 符合预期
6. 验证 response.inventiveness 同时非空（两个触发器并行工作）

---

## 五、执行顺序建议

```
P5.1 (testdata fixture) → 依赖 4.4 (existing tests)
  ↓
P5.2 (buildEnablementInput test) → 依赖 3.1 (enablement_events.go)
P5.3 (tool.go test) → 依赖 4.1 (tool.go)
  ↓ (可并行)
P5.4 (ArticleFramework 验证) → 依赖 1.3 (YAML)
P5.5 (SKILL.md) → 独立任务
  ↓
P5.6 (集成测试) → 依赖 P5.1-5.5 全部完成
```

**推荐批次**：
- **第 1 批**（并行）：P5.1 + P5.5（无依赖关系）
- **第 2 批**（并行）：P5.2 + P5.3 + P5.4（各依赖已有代码）
- **第 3 批**：P5.6（依赖前面全部）
