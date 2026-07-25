# Evidence 模块全链路接入设计

> 状态: 设计完成 · 2026-07-25

## 背景

Mady 项目拥有两层 evidence 模块：

- **`agentcore/evidence`** — 工具调用证据账本（Receipt/Ledger/EvidenceSpan/ClaimBinding/ConflictDetector），通过 LifecycleHook 自动记录工具调用，已在 `cmd/mady/framework.go` 中注册为 Agent Extension。
- **`domains/evidence`** — 专利证据判断规则引擎（三性审查/类型特定规则/举证责任分配/证明标准评估/可信度），核心入口 `Engine.Judge()`，**目前已完整实现但无任何生产代码调用**。

目标：将 evidence 模块全链路接入 Mady，覆盖 Agent Tool、工作流、HTTP API、CLI 四个层面。

## 整体架构

```
                      ┌──────────────────────────────────────┐
                      │         CLI: mady evidence            │
                      │  triple | burden | standard |         │
                      │  conflict | type-specific             │
                      │  JSON stdin → JSON stdout             │
                      └────────────────┬─────────────────────┘
                                       │
                      ┌────────────────┼─────────────────────┐
                      │   HTTP API     │                      │
                      │  /v1/evidence/*│                      │
                      │  (异步任务模式) │                     │
                      └────────────────┼─────────────────────┘
                                       │
         ┌─────────────────────────────┼─────────────────────────────┐
         │                     Agent Tools                            │
         │  judge_triple | check_burden | assess_standard |           │
         │  detect_conflict | judge_type_specific                     │
         │  (via EvidenceExtension → ToolProvider)                    │
         └─────────────────────────────┬─────────────────────────────┘
                                       │ 全部调用
         ┌─────────────────────────────▼─────────────────────────────┐
         │                  domains/evidence                          │
         │  DefaultEngine.Judge()  —  核心判断入口                     │
         │  三性审查 · 类型规则 · 举证责任 · 证明标准 · 冲突检测          │
         └─────────────────────────────┬─────────────────────────────┘
                                       │ 消费
         ┌─────────────────────────────▼─────────────────────────────┐
         │               agentcore/evidence                           │
         │  EvidenceSpan · ClaimBinding · Ledger · ConflictDetector   │
         └───────────────────────────────────────────────────────────┘

         ┌───────────────────────────────────────────────────────────┐
         │                   工作流接入                                │
         │  invalidation: …→ gather → judge → filter → conflict → … │
         │  infringement: …→ parse  → collect → judge → compare → … │
         └───────────────────────────────────────────────────────────┘
```

## 新增/修改文件清单

| 层 | 文件 | 操作 | 职责 |
|---|------|------|------|
| **Tool** | `domains/evidence/tool_triple.go` | 新增 | `judge_triple` — 三性审查 |
| **Tool** | `domains/evidence/tool_burden.go` | 新增 | `check_burden` — 举证责任查询 |
| **Tool** | `domains/evidence/tool_standard.go` | 新增 | `assess_standard` — 证明标准评估 |
| **Tool** | `domains/evidence/tool_conflict.go` | 新增 | `detect_conflict` — 冲突检测 |
| **Tool** | `domains/evidence/tool_type_specific.go` | 新增 | `judge_type_specific` — 类型特定判断 |
| **Extension** | `domains/evidence/extension.go` | 新增 | 注册所有工具到 Agent |
| **API** | `server/evidence.go` | 新增 | HTTP 端点 + 异步任务管理 |
| **CLI** | `cmd/mady/evidence.go` | 新增 | `mady evidence` 子命令 |
| **Workflow** | `workflows/patent/invalidation.go` | 修改 | 增加 judge/filter/conflict 节点 |
| **Workflow** | `workflows/patent/infringement.go` | 修改 | 增加 collect/judge 节点 |
| **Agent 注册** | `cmd/mady/framework.go` | 修改 | 注册 EvidenceExtension |
| **入口** | `cmd/mady/main.go` | 修改 | 添加 `evidence` 子命令路由 |

---

## 一、Agent Tools 设计

### 设计原则

- 全部 5 个工具均为 `ReadOnly: true`，Plan Mode 下可用
- 遵循 `agentcore.Tool{Name, Description, Parameters, Func, ReadOnly}` 模式
- 通过 `EvidenceExtension.Tools()` 统一注册，与 tasklist 扩展模式一致

### 1.1 `judge_triple` — 证据三性审查

对单条证据的关联性、合法性、真实性逐项评分并给出综合判断。

```json
// 输入
{
  "source_uri": "patent:CN12345678A",
  "snippet": "权利要求1公开了一种...",
  "scenario": "patent_invalidation"
}
// → 输出
{
  "relevance":  {"score": 0.85, "reasoning": "该证据直接涉及权利要求1的技术特征"},
  "legality":   {"score": 0.90, "reasoning": "来源为已授权专利文献，获取方式合法"},
  "authenticity": {"score": 0.95, "reasoning": "可通过官方专利数据库核实"},
  "issues": ["relevance 评分接近阈值"],
  "overall": 0.90
}
```

### 1.2 `check_burden` — 举证责任查询

纯只读，查询特定场景下的举证责任分配规则。

```json
// 输入
{"scenario": "patent_infringement"}
// → 输出
{
  "holder": "专利权人",
  "standard": "高度盖然性",
  "shift_allowed": true,
  "shift_trigger": "专利权人证明侵权基本事实后，举证责任转移至被控侵权人"
}
```

### 1.3 `assess_standard` — 证明标准评估

判断已有证据是否达到指定证明标准。

```json
// 输入
{"standard": "high_probability", "supporting_count": 7, "total_count": 10, "gaps": ["缺少权利要求3的对应证据"]}
// → 输出
{"met": true, "confidence": 0.70, "reasoning": "高度盖然性标准：有效证据占比70%，已达标准"}
```

### 1.4 `detect_conflict` — 证据冲突检测

接收多条证据及其主张方向，检测方向和来源矛盾。

```json
// 输入
{"claims": [
  {"claim_id": "特征A", "supporting": ["ev1"], "contradicting": ["ev2"]},
  {"claim_id": "特征B", "supporting": ["ev3"], "contradicting": []}
]}
// → 输出
{"conflicts": [{"type": "direction", "description": "主张「特征A」同时有支持性和矛盾性证据", "span_ids": ["ev1","ev2"]}]}
```

### 1.5 `judge_type_specific` — 类型特定判断

针对特定证据类型做专门检查。

```json
// 输入
{"source_uri": "https://example.com/product-page", "evidence_type_hint": "internet_public"}
// → 输出
{"evidence_type": "internet_public", "platform_credibility": "medium", "content_integrity": "stable", "public_intent": "public", "four_elements_check": {"time": true, "place": true, "method": true, "accessibility": true}}
```

---

## 二、HTTP API 设计

### 设计原则

- 与现有 `disclosure/analyze` 异步任务模式保持一致
- 判断任务（`judge`/`judge/batch`）采用 POST 提交 → GET 轮询
- 查询类端点（`burden`/`standard`/`conflict`）同步返回

### 端点清单

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/v1/evidence/judge` | 提交证据判断任务（异步） |
| `GET` | `/v1/evidence/judge/{task_id}` | 轮询判断结果 |
| `POST` | `/v1/evidence/judge/batch` | 批量判断（异步） |
| `GET` | `/v1/evidence/burden/{scenario}` | 查询举证责任规则（同步） |
| `GET` | `/v1/evidence/standard/{standard}` | 查询证明标准定义（同步） |
| `POST` | `/v1/evidence/conflict` | 证据冲突检测（同步） |

### 异步任务流

```
POST /v1/evidence/judge
{"source_uri": "...", "snippet": "...", "scenario": "patent_invalidation"}
  → {"task_id": "evj_abc123", "status": "pending"}

GET /v1/evidence/judge/evj_abc123
  → {"task_id": "evj_abc123", "status": "completed", "result": {...}, "error": ""}
```

任务状态：`pending` → `running` → `completed` | `failed`

### 批量判断

```json
// POST /v1/evidence/judge/batch
{"items": [{"source_uri": "...", "snippet": "..."}, {"source_uri": "...", "snippet": "..."}]}
// → {"task_id": "evb_xyz789", "status": "pending"}
```

### 内部实现

```go
// server/evidence.go
type evidenceTask struct {
    ID      string
    Status  string
    Result  *evidence.EvidenceJudgment
    Results []evidence.EvidenceJudgment  // 批量
    Err     error
    mu      sync.RWMutex
    doneCh  chan struct{}
}
```

任务存储复用 Server 的内存 map + 互斥锁模式（与 disclosureTask 一致）。
服务端注册在 `server/server.go` 的 `Handler()` 方法中，与现有端口并列。

---

## 三、工作流接入

### 3.1 无效宣告（`workflows/patent/invalidation.go`）

**改造前**：

```
parse_patent → identify_grounds → [gather_evidence] → analyze_grounds → conclude
                                    ↑ DegradationNotImplemented
```

**改造后**（有 retriever 时）：

```
parse_patent → identify_grounds → gather_evidence → judge_evidence
                                 → filter_evidence → detect_conflict
                                 → analyze_grounds → conclude → __end__
```

#### 新增 Pregel 节点

| 节点 | 函数 | 职责 |
|------|------|------|
| `judge_evidence` | `judgeEvidenceNode` | 对每条检索结果调用 `Engine.Judge()`，产出 `[]EvidenceJudgment` |
| `filter_evidence` | `filterEvidenceNode` | 过滤 overall_score < 0.5 的证据，产出有效证据集 |
| `detect_conflict` | `detectConflictNode` | 调用 `ConflictDetector.Detect()`，产出 `[]Conflict` |

#### 新增 State Keys

```go
const (
    InvStateEvidenceJudgments = "inv_evidence_judgments" // []EvidenceJudgment
    InvStateValidEvidence     = "inv_valid_evidence"     // []EvidenceSpan
    InvStateConflicts         = "inv_conflicts"          // []Conflict
)
```

#### 降级策略

无 retriever 时，三个新节点各自标记 `DegradationNotImplemented` 并透传 state。
`analyze_grounds` 在无证据时回退为纯法律分析。

### 3.2 侵权分析（`workflows/patent/infringement.go`）

**改造前**：

```
parse_claims → parse_product → full_coverage → equivalence → rule_check → conclude
```

**改造后**：

```
parse_claims → parse_product → collect_evidence → judge_evidence
             → full_coverage → equivalence → rule_check → conclude → __end__
```

#### 新增 Pregel 节点

| 节点 | 职责 |
|------|------|
| `collect_evidence` | 从权利要求特征列表构建 `[]EvidenceSpan` |
| `judge_evidence` | 三性审查（复用无效宣告的同名函数） |

#### 新增 State Keys

```go
const (
    InfStateEvidence          = "inf_evidence"           // []EvidenceSpan
    InfStateEvidenceJudgments = "inf_evidence_judgments" // []EvidenceJudgment
)
```

#### 举证责任联动

`full_coverage` 节点读取 `inf_evidence_judgments`，使用 `BurdenScenarioPatentInfringement`（高度盖然性标准）评估证据是否达标，未达标时在结论中标记风险。

---

## 四、CLI 子命令设计

### 设计原则

- 管道友好：JSON stdin → JSON stdout，错误 → stderr
- `--file` flag 优先级高于 stdin

### 命令结构

```bash
mady evidence <action> [--file <path>]
```

### 5 个 action

```bash
# 三性审查
echo '{"source_uri":"...","snippet":"..."}' | mady evidence triple

# 举证责任查询
echo '{"scenario":"patent_infringement"}' | mady evidence burden

# 证明标准评估
echo '{"standard":"high_probability","supporting_count":7,"total_count":10}' | mady evidence standard

# 冲突检测
echo '{"claims":[{"claim_id":"A","supporting":["e1"],"contradicting":["e2"]}]}' | mady evidence conflict

# 类型特定判断
echo '{"source_uri":"https://example.com","evidence_type_hint":"internet_public"}' | mady evidence type-specific

# 从文件批量读取
mady evidence triple --file evidence_list.json
```

### 实现

```go
// cmd/mady/evidence.go
func runEvidenceCLI(args []string) {
    engine := evidence.NewEngine(nil)
    // 读取 stdin 或 --file，解析 JSON → 调用对应引擎方法 → 输出 JSON
}
```

注册到 `cmd/mady/main.go` 顶层命令路由：

```go
case "evidence":
    runEvidenceCLI(args)
```

---

## 五、测试策略

| 层 | 测试文件 | 覆盖内容 |
|---|---------|---------|
| Tool | `domains/evidence/tool_*_test.go` | 每个工具的输入校验、正常流程、边界条件 |
| Extension | `domains/evidence/extension_test.go` | Tools() 返回 5 个工具、LifecycleHook 行为 |
| API | `server/evidence_test.go` | 端点的任务生命周期、错误路径、批量判断 |
| CLI | `cmd/mady/evidence_test.go` | stdin/file 输入、JSON 输出格式、错误退出码 |
| Workflow | `workflows/patent/invalidation_test.go` | 新增节点在完整图中的行为、降级路径 |
| Workflow | `workflows/patent/infringement_test.go` | 新增节点的证据收集和判断链路 |
