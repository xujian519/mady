# 01 — 提案：实用新型（及发明）驳回复审请求书工作流

- **功能名**：reexamination-request
- **Human Owner**：[NEEDS CLARIFICATION: 待指派]
- **提案日期**：2026-07-22
- **状态**：待人工 Sign-off

---

## 1. 背景

### 1.1 现状：项目无复审请求能力

Mady 的专利文书自动化目前只覆盖**审查意见答复（OA Response）**——即审查员发出"审查意见通知书"后申请人的"意见陈述书"。这条线在 `workflows/patent/oa_response.go` 已完整实现（Pregel 图 + `draft_oa_response` 工具 + `doc-templates/oa-response/` 模板）。

但**"驳回复审请求"是另一条独立的法律程序**，项目目前没有任何对应工作流。全仓检索 `驳回复审|reexamination|复审请求` 仅命中两处知识/规划层引用（`guardrails/guideline_source.go` 的法条索引、`docs/plans/` 的计划字段），无任何可执行代码。

### 1.2 业务差异：复审请求 ≠ OA 答复

这两个程序处于不同阶段、针对不同文书、受理部门也不同：

| 维度 | OA 审查意见答复（已实现） | 驳回复审请求（本提案） |
|------|--------------------------|------------------------|
| 触发文书 | 审查意见通知书（含初步驳回理由） | **驳回决定**（正式驳回） |
| 程序阶段 | 实质/初步审查过程中 | 驳回后的救济程序 |
| 受理部门 | 审查员本人 | 复审和无效审理部门（合议庭） |
| 法律依据 | 《专利法》第 22/26/33 条 | 《专利法》**第 41 条** + 实施细则第 60-62 条 |
| 产出文书 | 意见陈述书 | **复审请求书**（含固定栏目 + 修改替换页） |
| 期限 | 一通 4 个月 / 二通 2 个月 | 收到驳回决定之日起 **3 个月**内（可延 1 个月） |

因此无法简单复用 OA 工作流，需要独立的驳回决定解析器和复审请求书模板。

### 1.3 已有的可复用资产（重要）

项目在**案件管理 / 意图识别**层已埋下复审相关的概念，只是缺少文书生成工作流：

| 已有能力 | 位置 | 复用方式 |
|----------|------|----------|
| 驳回决定文档分类 | `domains/case_index.go` `DocRejection = "rejection"` | 复审工作流可直接消费已分类的驳回决定文档 |
| 案件驳回状态 | `domains/case_index.go` `CaseStatusRejected = "rejected"` | 触发"该案件可提复审"的判定依据 |
| 复审意图识别 | `domains/legal_intent.go` `reasoning.CaseReexamination` | Router 已能识别"复审请求"意图 |
| 期限计算 | `domains/deadline_calculator.go` | [NEEDS CLARIFICATION: 是否已含复审 3 个月期限规则] |
| OA 工作流范式 | `workflows/patent/oa_response.go` | Pregel 图结构 + 工具封装模式直接参照 |
| 驳回理由类型 | `workflows/patent/oa_types.go` `OaRejectionType` | 类型枚举可扩展复用 |
| 文档分类器 | `domains/case_classifier*.go` | 已能识别"驳回决定.pdf" |

### 1.4 为什么现在做

1. OA 答复工作流已成熟稳定，复审请求是其**同族下游场景**，架构范式可平移，开发成本低
2. 案件索引库（`CaseIndex`）已能追踪案件到"驳回"状态，形成"驳回 → 复审"的业务闭环是自然延伸
3. 复审是驳回后唯一的行政救济途径，对专利代理人/律师是高频刚需

---

## 2. 目标

### 2.1 总目标

为 Patent Agent 增加 `draft_reexamination_request` 工具：输入**驳回决定书**全文，自动解析驳回决定要素（文号/日期/驳回理由/对比文件），生成结构化**复审请求书**骨架，经人工审阅后定稿。

### 2.2 阶段目标

| 阶段 | 目标 | 一句话验收 |
|------|------|-----------|
| **阶段 1：MVP（本期）** | 驳回决定解析器 + 复审请求书模板 + 确定性 Pregel 图 + 工具注册 | 喂入驳回决定书文本，输出含固定栏目的复审请求书骨架 |
| **阶段 2：实用新型特化** | 实用新型客体抗辩模板 + 实用新型驳回理由特化处理 | 实用新型驳回决定（客体问题）能生成针对性的客体抗辩论证 |
| **阶段 3：LLM 增强**（可选） | 注入 Provider，生成实质论证段落 | 骨架基础上自动产出有说服力的复审理由 |
| **阶段 4：案件闭环**（可选） | 复审请求书回写 `CaseDocument`，自动判定驳回案件并提示期限 | 驳回案件自动提示"3 个月内提复审" |

> 本提案聚焦**阶段 1 + 阶段 2**（实用新型场景覆盖），阶段 3/4 作为后续迭代。

### 2.3 非目标（本期不做）

- 复审委后续审查阶段（前置审查、口头审理）的模拟
- 复审决定结果的预测
- 复审请求书的在线提交/电子申请接口
- 与 eFiling / CPC 客户端的对接

---

## 3. 成功标准

### 3.1 功能验收

| 编号 | 标准 | 验证方式 |
|------|------|----------|
| AC-1 | 喂入驳回决定书文本，调用 `draft_reexamination_request`，输出结构化复审请求书（含请求人/申请信息/驳回决定信息/复审理由/免责声明） | 单元测试 + 手动 |
| AC-2 | 解析器正确提取驳回决定文号、决定日期、申请号、驳回理由的法律依据（条/款） | 单元测试 |
| AC-3 | 实用新型驳回决定（含 A2.2 客体问题）能识别并触发"实用新型客体抗辩"论证模板 | 单元测试（阶段 2） |
| AC-4 | 工具在 `domains/patent.go` 注册后，Patent Agent 可在 TUI 中调用 | 手动冒烟 |
| AC-5 | 输出附带人工审核提醒与免责声明（遵循 `docs/tone-style-guide.md`） | 代码检查 |
| AC-6 | 期限提示（3 个月）出现在输出中（若驳回决定日期可解析） | 单元测试 |

### 3.2 质量验收

- `go build ./...` / `go vet ./...` / `go test -race ./...` 全绿
- `golangci-lint run` 零 issue
- 新增代码符合分层架构（领域层 `domains/` 不 import `workflows/` 的实现细节，通过工具接口注入）
- 遵循 `MADY_HOME` 统一路径解析，不新增 cwd 相对路径

### 3.3 回归红线

- 现有 `oa_response.go` / `oa_response_tool.go` 公开 API 不做破坏性变更
- `OaRejectionType` 枚举只新增不修改（向后兼容）
- 复审工作流作为独立工具注册，不影响现有 `draft_oa_response` 行为

---

## 4. 关键约束

1. **实用新型客体特化**：实用新型驳回的高频理由是"不属于实用新型保护客体"（《专利法》第 2 条第 3 款），这是发明复审**不存在**的理由类型，必须特化处理
2. **实用新型不审查创造性**：实用新型只做初步审查，其驳回理由类型与发明不同——本工作流的理由类型枚举需据此裁剪（详见 §7）
3. **安全敏感路径**：`domains/patent.go` 在敏感路径表内（涉及动态 WorkingDir），注册工具的改动需人工审阅（L3）
4. **措辞规范**：复审理由与免责声明遵循 `docs/tone-style-guide.md`，不使用绝对化表述，结论附置信度

---

## 5. 设计方案

### 5.1 数据模型

参照 `ParsedOfficeAction`，新增驳回决定解析结果：

```go
// ParsedRejectionDecision 表示解析后的驳回决定书。
type ParsedRejectionDecision struct {
    DecisionNumber    string           // 驳回决定文号（如"国知局驳回决定 第2026XXXX号"）
    DecisionDate      string           // 决定作出日期
    ApplicationNumber string           // 专利申请号
    PatentTitle       string           // 发明创造名称
    Applicant         string           // 申请人
    Examiner          string           // 审查员
    PatentType        string           // 申请类型（发明/实用新型/外观）—— 决定特化分支
    RejectionGrounds  []RejectionGround // 驳回理由列表
    CitedReferences   []CitedReference  // 引用对比文件（复用 oa_types.CitedReference）
}

// RejectionGround 单条驳回理由。
type RejectionGround struct {
    ClaimNumbers []int   // 受影响的权利要求
    LegalBasis   string  // 法律依据（如"第2条第3款""第26条第4款""第22条第2款"）
    GroundType   string  // 理由类型枚举（见下）
    Summary      string  // 驳回理由要点摘要
}
```

### 5.2 驳回理由类型枚举

扩展自 `OaRejectionType`，新增复审/实用新型特有类型：

```go
type RejectionGroundType string

const (
    GroundUMSubject   RejectionGroundType = "um_subject"   // 实用新型客体（A2.2）—— 实用新型特有
    GroundNovelty     RejectionGroundType = "novelty"      // 新颖性（A22.2）
    GroundClarity     RejectionGroundType = "clarity"      // 清楚（A26.4）
    GroundSupport     RejectionGroundType = "support"      // 支持（A26.4）
    GroundDisclosure  RejectionGroundType = "disclosure"   // 公开充分（A26.3）
    GroundScope       RejectionGroundType = "scope"        // 修改超范围（A33）
    GroundFormal      RejectionGroundType = "formal"       // 形式缺陷
)
```

> **注意**：实用新型初步审查**不主动审查创造性**（A22.3），故枚举不含 `inventiveness`。若驳回决定中出现创造性理由（罕见，通常在复审阶段由请求人主动争辩），归入 `combined` 兜底。

### 5.3 Pregel 图结构

完全参照 OA 图的范式，节点职责针对复审场景调整：

```
parse_decision → classify_grounds → analyze_claims → draft_request → [llm_enhance] → approval_gate → __end__
```

| 节点 | 职责 | 是否用 LLM |
|------|------|-----------|
| `parse_decision` | 确定性规则解析驳回决定书（文号/日期/理由/对比文件/申请类型） | 否 |
| `classify_grounds` | 按理由类型选择答复策略 + 复审请求书模板（含实用新型客体分支） | 否 |
| `analyze_claims` | 权利要求级分析 + 修改建议对照表（复审常含修改替换页） | 否 |
| `draft_request` | 组装复审请求书骨架（固定栏目 + 复审理由 + 期限提示 + 免责声明） | 否 |
| `llm_enhance`（可选） | 阶段 3：在骨架上生成实质论证段落 | 是 |
| `approval_gate` | 标记需人工审阅，暂停管线 | 否 |

### 5.4 工具签名

参照 `NewOAResponseTool`：

```go
// NewReexaminationRequestTool 创建 draft_reexamination_request 工具。
// 输入驳回决定书文本，运行复审请求 Pregel 图，输出结构化复审请求书骨架。
func NewReexaminationRequestTool(opts ...ReexaminationGraphOption) *agentcore.Tool
```

工具参数：
- `decision_text`（必填）：驳回决定书全文
- `claim_text`（可选）：当前权利要求书文本，用于更精确的权利要求分析
- `case_id`（可选）：关联案件 ID，用于回写 `CaseDocument`（阶段 4）

### 5.5 文档模板结构

新增 `doc-templates/reexamination/` 目录：

```
doc-templates/reexamination/
├── request-header.md        # 复审请求书固定栏目（请求人/申请信息/驳回决定信息/法律依据第41条）
├── um-subject-defense.md    # 实用新型客体抗辩（A2.2）—— 实用新型特化
└── general-grounds.md       # 通用驳回理由抗辩（新颖性/清楚/支持/公开/超范围）
```

复审请求书固定栏目（遵循国知局标准表格）：

1. **请求人信息**（姓名/名称、地址、代表人、代理机构）
2. **专利申请信息**（申请号、申请日、发明创造名称）
3. **驳回决定信息**（决定文号、决定日期、审查员）
4. **复审请求理由**（逐条反驳驳回决定）—— 由 `classify_grounds` 选择的模板填充
5. **修改说明**（如有修改替换页，对照说明）
6. **法律依据**（专利法第 41 条 + 实施细则第 60-62 条 + 期限提示）
7. **签字盖章** + **人工审核提醒 + 免责声明**

---

## 6. 文件清单（阶段 1+2）

控制在 5 个文件内，符合 AGENTS.md 任务粒度约定：

| 文件 | 职责 | 改动类型 |
|------|------|----------|
| `workflows/patent/reexamination.go` | 数据模型 + 驳回决定解析器 + Pregel 图节点 + 图构建器 | 新增 |
| `workflows/patent/reexamination_tool.go` | `draft_reexamination_request` 工具封装 | 新增 |
| `doc-templates/reexamination/request-header.md` | 复审请求书固定栏目模板 | 新增 |
| `doc-templates/reexamination/um-subject-defense.md` | 实用新型客体抗辩模板 | 新增 |
| `domains/patent.go` | 注册 `NewReexaminationRequestTool()`（1 行，敏感路径需审阅） | 修改 |

配套测试（不计入主文件数）：
- `workflows/patent/reexamination_test.go` — 解析器 + 图构建 + 实用新型分支单元测试

> 阶段 3（LLM 增强）可复用 `oa_response.go` 的 `WithOAProvider` 模式，新增 `WithReexaminationProvider` 选项，改动仅限 `reexamination.go` 内部。

---

## 7. 实用新型特化考量

实用新型与发明的审查程序差异决定了复审请求的论证重点不同，这是本提案相对 OA 工作流的核心增量：

| 维度 | 发明 | 实用新型 |
|------|------|----------|
| 审查程序 | 初步审查 + **实质审查** | 仅**初步审查** |
| 高频驳回理由 | 创造性（A22.3）、新颖性（A22.2） | **实用新型客体**（A2.2）、清楚/支持（A26.4）、新颖性（A22.2） |
| 客体问题 | 少见 | **高频**——必须是产品的形状、构造或其结合 |
| 创造性审查 | 主动审查 | **不主动审查** |

因此：
- `um-subject-defense.md` 模板需专门论证"技术方案属于产品的形状、构造或其结合"，这是实用新型复审的**特有抗辩模块**
- `classify_grounds` 节点检测到 `PatentType == "实用新型"` 且存在 `GroundUMSubject` 时，优先选用该模板
- 通用模板（`general-grounds.md`）处理新颖性/清楚/支持等，可部分复用 OA 模板的论证框架

---

## 8. 决策摘要

| 决策点 | 选择 | 备选 | 理由 |
|--------|------|------|------|
| 图引擎 | Pregel（复用现有 graph 包） | 全新工作流框架 | 与 OA 工作流一致，复用编译/执行机制 |
| 解析方式 | 确定性规则（regex/关键词） | LLM 解析 | 与 `ParseOA` 一致，可测试、可复现 |
| 工具注册 | `domains/patent.go` ExtraTools | 新建独立 domain | 复审属专利域，与 OA 工具同族 |
| 模板组织 | 独立 `doc-templates/reexamination/` | 复用 `oa-response/` | 文书栏目与论证结构本质不同 |
| 实用新型特化 | 独立客体抗辩模板 + 类型分支 | 通用模板统一处理 | 客体问题是实用新型独有高频理由，特化质量更高 |
| LLM 增强 | 阶段 3 可选注入 | 默认开启 | MVP 先保确定性，质量验收后再开 |

---

## 9. 风险

| 风险 | 等级 | 缓解 |
|------|------|------|
| 驳回决定书格式不统一，确定性解析漏提取 | 中 | 规则解析 + 兜底分支；解析失败时降级为通用模板并提示人工补充 |
| 实用新型客体判断需要技术理解，规则难以覆盖 | 中 | 模板提供论证框架（占位符 + 法律依据），实质判断留给人工审阅 + 阶段 3 LLM |
| 复审期限计算依赖决定日期，日期解析失败 | 低 | 日期缺失时跳过期限计算，提示"请核实驳回决定日期以确认复审期限" |
| `domains/patent.go` 属敏感路径，工具注册需审阅 | 低 | 改动仅 1 行注册，L3 审阅；不触碰 WorkingDir/沙箱逻辑 |

---

## 10. 下一步

人工 Sign-off 本提案后：

1. 确认 `[NEEDS CLARIFICATION]` 项（Human Owner、`deadline_calculator` 是否已含复审期限）
2. 进入 `02-spec.md`（补充驳回决定书的解析规则细节、模板占位符清单、测试用例样本）
3. 阶段 1 实现控制在 5 文件内，按 `04-tasks.md` 拆解逐步推进
4. 完成后在 `docs/decisions/AI_CHANGELOG.md` 记录决策
