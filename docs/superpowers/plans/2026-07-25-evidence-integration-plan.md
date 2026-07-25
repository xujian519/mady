# Evidence 模块全链路接入实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `domains/evidence` 证据判断引擎全链路接入 Mady 项目——5 个 Agent Tool、HTTP API 异步服务、CLI 子命令、无效宣告和侵权分析工作流集成。

**Architecture:** 以 `domains/evidence.DefaultEngine` 为核心，向上构建 Tool 层（5 个只读 Agent 工具）→ Extension 注册 → HTTP API 异步任务 → CLI 管道友好子命令 → 工作流 Pregel 节点。遵循现有 tasklist extension、disclosure API、patent workflow 的代码模式。

**Tech Stack:** Go 1.26, agentcore.Tool/Extension, graph.Pregel, net/http (Go 1.22+ 路由)

**Design Doc:** [docs/superpowers/specs/2026-07-25-evidence-integration-design.md](../specs/2026-07-25-evidence-integration-design.md)

---

## 文件结构

| 文件 | 操作 | 职责 |
|------|------|------|
| `domains/evidence/extension.go` | 新增 | EvidenceDomainExtension — 注册 5 个工具 |
| `domains/evidence/tool_triple.go` | 新增 | `judge_triple` — 三性审查 |
| `domains/evidence/tool_burden.go` | 新增 | `check_burden` — 举证责任查询 |
| `domains/evidence/tool_standard.go` | 新增 | `assess_standard` — 证明标准评估 |
| `domains/evidence/tool_conflict.go` | 新增 | `detect_conflict` — 冲突检测 |
| `domains/evidence/tool_type_specific.go` | 新增 | `judge_type_specific` — 类型特定判断 |
| `domains/evidence/tools_test.go` | 新增 | 全部 5 个工具 + extension 测试 |
| `cmd/mady/evidence.go` | 新增 | CLI 5 个子命令 |
| `cmd/mady/evidence_test.go` | 新增 | CLI 测试 |
| `server/evidence.go` | 新增 | HTTP 异步任务 API |
| `server/evidence_test.go` | 新增 | API 测试 |
| `cmd/mady/framework.go` | 修改 | 注册 EvidenceDomainExtension |
| `cmd/mady/main.go` | 修改 | 路由 `evidence` 子命令 |
| `server/server.go` | 修改 | 注册 `/v1/evidence/*` 端点 |
| `workflows/patent/invalidation.go` | 修改 | 新增 judge/filter/conflict 节点 |
| `workflows/patent/infringement.go` | 修改 | 新增 collect/judge 节点 |

---

### Task 1: EvidenceDomainExtension — 基础设施

**Files:**
- Create: `domains/evidence/extension.go`
- Create: `domains/evidence/tools_test.go` (extension 部分)

- [ ] **Step 1: 编写 extension 测试**

```go
// domains/evidence/tools_test.go
package evidence

import (
	"context"
	"testing"
)

func TestEvidenceDomainExtension_Name(t *testing.T) {
	ext := NewDomainExtension(nil)
	if ext.Name() != ExtensionNameDomain {
		t.Errorf("expected %q, got %q", ExtensionNameDomain, ext.Name())
	}
}

func TestEvidenceDomainExtension_InitDispose(t *testing.T) {
	ext := NewDomainExtension(nil)
	if err := ext.Init(context.Background(), nil); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := ext.Dispose(); err != nil {
		t.Fatalf("Dispose: %v", err)
	}
}

func TestEvidenceDomainExtension_Tools(t *testing.T) {
	ext := NewDomainExtension(nil)
	tools := ext.Tools()
	if len(tools) != 5 {
		t.Fatalf("expected 5 tools, got %d", len(tools))
	}
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"judge_triple", "check_burden", "assess_standard", "detect_conflict", "judge_type_specific"} {
		if !names[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
cd /Users/xujian/projects/Mady && go test ./domains/evidence/... -run TestEvidenceDomainExtension -count=1
```
Expected: 编译错误 `undefined: NewDomainExtension`

- [ ] **Step 3: 实现 EvidenceDomainExtension**

```go
// domains/evidence/extension.go
package evidence

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionNameDomain 是领域证据判断扩展的注册名称。
// 与 agentcore/evidence.Extension（"evidence"）不同，此扩展名为 "evidence_judge"，
// 用于注入证据判断类工具。
const ExtensionNameDomain = "evidence_judge"

// EvidenceDomainExtension 将专利证据判断工具集注入 Agent。
//
// 通过 ToolProvider 贡献 5 个只读工具：
//   - judge_triple: 证据三性审查
//   - check_burden: 举证责任查询
//   - assess_standard: 证明标准评估
//   - detect_conflict: 证据冲突检测
//   - judge_type_specific: 类型特定判断
//
// 与 tasklist/planmode/filecheckpoint 等扩展使用相同的装配机制。
type EvidenceDomainExtension struct {
	engine *DefaultEngine
}

var (
	_ agentcore.Extension    = (*EvidenceDomainExtension)(nil)
	_ agentcore.ToolProvider = (*EvidenceDomainExtension)(nil)
)

// NewDomainExtension 创建领域证据判断扩展。
// 传入 nil 时使用默认规则索引。
func NewDomainExtension(index *RuleIndex) *EvidenceDomainExtension {
	return &EvidenceDomainExtension{engine: NewEngine(index)}
}

// Name 实现 agentcore.Extension。
func (e *EvidenceDomainExtension) Name() string { return ExtensionNameDomain }

// Init 实现 agentcore.Extension。
func (e *EvidenceDomainExtension) Init(_ context.Context, _ *agentcore.Agent) error { return nil }

// Dispose 实现 agentcore.Extension。
func (e *EvidenceDomainExtension) Dispose() error { return nil }

// Tools 实现 agentcore.ToolProvider，返回 5 个证据判断工具。
func (e *EvidenceDomainExtension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		newTripleTool(e.engine),
		newBurdenTool(),
		newStandardTool(),
		newConflictTool(e.engine),
		newTypeSpecificTool(e.engine),
	}
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./domains/evidence/... -run TestEvidenceDomainExtension -count=1 -v
```
Expected: 3 个测试 PASS（编译通过后 Tool 函数名尚未定义，会因链接错误失败；此时先确认 extension 自身逻辑正确。Tool 函数在后续 Task 中逐一实现后测试将全部通过）

> 注意：此步骤工具函数（`newTripleTool` 等）尚未实现，extension_test 可编译但链接失败是预期的。在 Task 2-6 逐个实现工具后，此测试将自动通过。如需独立验证 extension 结构，可在本 task 中为 5 个工具函数添加返回 nil 的占位实现。

- [ ] **Step 5: 添加占位工具函数以便 extension 测试通过**

```go
// domains/evidence/tool_stubs_placeholder.go (临时文件，后续 task 中删除)
package evidence

import "github.com/xujian519/mady/agentcore"

func newTripleTool(engine *DefaultEngine) *agentcore.Tool       { return &agentcore.Tool{Name: "judge_triple"} }
func newBurdenTool() *agentcore.Tool                            { return &agentcore.Tool{Name: "check_burden"} }
func newStandardTool() *agentcore.Tool                          { return &agentcore.Tool{Name: "assess_standard"} }
func newConflictTool(engine *DefaultEngine) *agentcore.Tool     { return &agentcore.Tool{Name: "detect_conflict"} }
func newTypeSpecificTool(engine *DefaultEngine) *agentcore.Tool { return &agentcore.Tool{Name: "judge_type_specific"} }
```

- [ ] **Step 6: 运行 extension 测试确认通过**

```bash
go test ./domains/evidence/... -run TestEvidenceDomainExtension -count=1 -v
```
Expected: 3 PASS

- [ ] **Step 7: Commit**

```bash
git add domains/evidence/extension.go domains/evidence/tool_stubs_placeholder.go domains/evidence/tools_test.go
git commit -m "feat(evidence): add EvidenceDomainExtension with 5 tool stubs"
```

---

### Task 2: `judge_triple` — 证据三性审查工具

**Files:**
- Create: `domains/evidence/tool_triple.go`
- Modify: `domains/evidence/tools_test.go` (追加测试)

- [ ] **Step 1: 编写 judge_triple 测试**

在 `domains/evidence/tools_test.go` 末尾追加：

```go
func TestJudgeTripleTool_Success(t *testing.T) {
	engine := NewEngine(nil)
	tool := newTripleTool(engine)

	args := `{"source_uri":"patent:CN12345678A","snippet":"权利要求1公开了一种图像识别方法"}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	for _, key := range []string{"relevance", "legality", "authenticity", "overall", "reasoning"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing key %q in result", key)
		}
	}
	// 验证 overall 在 0-1 范围内
	if overall, ok := m["overall"].(float64); ok {
		if overall < 0 || overall > 1 {
			t.Errorf("overall %f outside [0,1]", overall)
		}
	}
}

func TestJudgeTripleTool_MissingSnippet(t *testing.T) {
	engine := NewEngine(nil)
	tool := newTripleTool(engine)

	args := `{"source_uri":"patent:CN12345678A"}`
	_, err := tool.Func(context.Background(), json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for missing snippet")
	}
}

func TestJudgeTripleTool_InvalidJSON(t *testing.T) {
	engine := NewEngine(nil)
	tool := newTripleTool(engine)

	_, err := tool.Func(context.Background(), json.RawMessage(`{bad json}`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
```

需要在文件头部添加 import：

```go
import (
	"context"
	"encoding/json"
	"testing"
)
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./domains/evidence/... -run TestJudgeTriple -count=1 -v
```
Expected: `undefined: newTripleTool`（占位函数在 tool_stubs_placeholder.go 中定义了返回 stub，需先更新为真正实现）

- [ ] **Step 3: 实现 judge_triple**

```go
// domains/evidence/tool_triple.go
package evidence

import (
	"context"
	"encoding/json"
	"fmt"

	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
	"github.com/xujian519/mady/agentcore"
)

const TripleToolName = "judge_triple"

const tripleToolDesc = `对单条证据进行三性审查（关联性、合法性、真实性），返回逐项评分和综合判断。

适用场景：
- 评估现有技术文献是否可以作为无效宣告证据
- 判断互联网公开内容的法律效力
- 核实域外证据的可采性

返回逐项评分（0-1）及推理说明，综合评分低于 0.5 的证据不建议使用。`

type tripleTool struct {
	engine *DefaultEngine
}

func newTripleTool(engine *DefaultEngine) *agentcore.Tool {
	t := &tripleTool{engine: engine}
	return &agentcore.Tool{
		Name:        TripleToolName,
		Description: tripleToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source_uri": map[string]any{
					"type":        "string",
					"description": "证据来源 URI，如 patent:CN12345678A 或 https://...",
				},
				"snippet": map[string]any{
					"type":        "string",
					"description": "证据原文摘录",
				},
				"scenario": map[string]any{
					"type":        "string",
					"description": "使用场景：patent_invalidation / patent_infringement / novelty_challenge / inventiveness / disclosure / priority",
				},
			},
			"required":             []string{"snippet"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type tripleArgs struct {
	SourceURI string `json:"source_uri"`
	Snippet   string `json:"snippet"`
	Scenario  string `json:"scenario"`
}

func (t *tripleTool) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p tripleArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if p.Snippet == "" {
		return nil, fmt.Errorf("缺少必填字段 'snippet'")
	}

	span := agentcore_evidence.EvidenceSpan{
		ID:        "tool_input",
		SourceURI: p.SourceURI,
		Snippet:   p.Snippet,
	}

	judgment, err := t.engine.Judge(span)
	if err != nil {
		return nil, fmt.Errorf("证据判断失败: %w", err)
	}

	return judgmentToMap(judgment), nil
}

// judgmentToMap 将 EvidenceJudgment 转换为便于 LLM 消费的 map。
func judgmentToMap(j *EvidenceJudgment) map[string]any {
	m := map[string]any{
		"overall":   j.OverallScore,
		"confidence": j.Confidence,
		"reasoning":  j.Reasoning,
	}

	if j.RelevanceJudgment != nil {
		m["relevance"] = map[string]any{
			"score":     j.RelevanceJudgment.Score,
			"level":     j.RelevanceJudgment.Level,
			"reasoning": j.RelevanceJudgment.Reasoning,
		}
	}
	if j.LegalityJudgment != nil {
		m["legality"] = map[string]any{
			"score":     j.LegalityJudgment.Score,
			"level":     j.LegalityJudgment.Level,
			"reasoning": j.LegalityJudgment.Reasoning,
		}
	}
	if j.AuthenticityJudgment != nil {
		m["authenticity"] = map[string]any{
			"score":     j.AuthenticityJudgment.Score,
			"level":     j.AuthenticityJudgment.Level,
			"reasoning": j.AuthenticityJudgment.Reasoning,
		}
	}

	var issues []map[string]string
	for _, issue := range j.FlaggedIssues {
		issues = append(issues, map[string]string{
			"type":        issue.Type,
			"description": issue.Description,
			"severity":    issue.Severity,
		})
	}
	if len(issues) > 0 {
		m["issues"] = issues
	}
	return m
}
```

- [ ] **Step 4: 删除 tool_stubs_placeholder.go 中的 newTripleTool 占位**

编辑 `domains/evidence/tool_stubs_placeholder.go`，删除 `newTripleTool` 那一行。

- [ ] **Step 5: 运行测试验证通过**

```bash
go test ./domains/evidence/... -run TestJudgeTriple -count=1 -v
```
Expected: 3 PASS

- [ ] **Step 6: Commit**

```bash
git add domains/evidence/tool_triple.go domains/evidence/tool_stubs_placeholder.go domains/evidence/tools_test.go
git commit -m "feat(evidence): add judge_triple tool for evidence triple-attribute review"
```

---

### Task 3: `check_burden` — 举证责任查询工具

**Files:**
- Create: `domains/evidence/tool_burden.go`
- Modify: `domains/evidence/tools_test.go` (追加测试)

- [ ] **Step 1: 编写 check_burden 测试**

在 `domains/evidence/tools_test.go` 末尾追加：

```go
func TestCheckBurdenTool_ValidScenario(t *testing.T) {
	tool := newBurdenTool()

	args := `{"scenario":"patent_invalidation"}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if m["holder"] != "请求人" {
		t.Errorf("expected holder 请求人, got %v", m["holder"])
	}
	if m["standard"] != "优势证据" {
		t.Errorf("expected standard 优势证据, got %v", m["standard"])
	}
}

func TestCheckBurdenTool_InvalidScenario(t *testing.T) {
	tool := newBurdenTool()

	args := `{"scenario":"nonexistent"}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["holder"] != "主张方" {
		t.Errorf("expected fallback holder 主张方, got %v", m["holder"])
	}
}

func TestCheckBurdenTool_MissingScenario(t *testing.T) {
	tool := newBurdenTool()

	args := `{}`
	_, err := tool.Func(context.Background(), json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for missing scenario")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./domains/evidence/... -run TestCheckBurden -count=1 -v
```
Expected: `undefined: newBurdenTool`（或 stub 返回空结果导致测试失败）

- [ ] **Step 3: 实现 check_burden**

```go
// domains/evidence/tool_burden.go
package evidence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
)

const BurdenToolName = "check_burden"

const burdenToolDesc = `查询特定场景下的举证责任分配规则。

支持场景：
- patent_invalidation: 专利无效宣告
- patent_infringement: 专利侵权
- novelty_challenge: 新颖性质疑
- inventiveness: 创造性评估
- disclosure: 充分公开
- priority: 优先权核实

返回举证责任方、适用证明标准、转移条件等信息。`

type burdenTool struct{}

func newBurdenTool() *agentcore.Tool {
	t := &burdenTool{}
	return &agentcore.Tool{
		Name:        BurdenToolName,
		Description: burdenToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenario": map[string]any{
					"type":        "string",
					"description": "查询场景",
					"enum":        []string{"patent_invalidation", "patent_infringement", "novelty_challenge", "inventiveness", "disclosure", "priority"},
				},
			},
			"required":             []string{"scenario"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type burdenArgs struct {
	Scenario string `json:"scenario"`
}

func (t *burdenTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var p burdenArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if p.Scenario == "" {
		return nil, fmt.Errorf("缺少必填字段 'scenario'")
	}

	result := DetermineBurden(BurdenScenario(p.Scenario), nil)

	return map[string]any{
		"holder":        result.BurdenHolder,
		"standard":      result.Standard,
		"has_shifted":   result.HasShifted,
		"shift_reason":  result.ShiftReason,
		"reasoning":     result.Reasoning,
	}, nil
}
```

- [ ] **Step 4: 删除 tool_stubs_placeholder.go 中的 newBurdenTool 占位**

- [ ] **Step 5: 运行测试验证通过**

```bash
go test ./domains/evidence/... -run TestCheckBurden -count=1 -v
```
Expected: 3 PASS

- [ ] **Step 6: Commit**

```bash
git add domains/evidence/tool_burden.go domains/evidence/tool_stubs_placeholder.go domains/evidence/tools_test.go
git commit -m "feat(evidence): add check_burden tool for burden of proof query"
```

---

### Task 4: `assess_standard` — 证明标准评估工具

**Files:**
- Create: `domains/evidence/tool_standard.go`
- Modify: `domains/evidence/tools_test.go` (追加测试)

- [ ] **Step 1: 编写 assess_standard 测试**

在 `domains/evidence/tools_test.go` 末尾追加：

```go
func TestAssessStandardTool_Met(t *testing.T) {
	tool := newStandardTool()

	args := `{"standard":"preponderance","supporting_count":7,"total_count":10}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["met"] != true {
		t.Errorf("expected met=true, got %v", m["met"])
	}
}

func TestAssessStandardTool_NotMet(t *testing.T) {
	tool := newStandardTool()

	args := `{"standard":"high_probability","supporting_count":3,"total_count":10}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["met"] != false {
		t.Errorf("expected met=false, got %v", m["met"])
	}
}

func TestAssessStandardTool_EmptyTotal(t *testing.T) {
	tool := newStandardTool()

	args := `{"standard":"high_probability","supporting_count":0,"total_count":0}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["met"] != false {
		t.Errorf("expected met=false for zero total, got %v", m["met"])
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./domains/evidence/... -run TestAssessStandard -count=1 -v
```

- [ ] **Step 3: 实现 assess_standard**

```go
// domains/evidence/tool_standard.go
package evidence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
)

const StandardToolName = "assess_standard"

const standardToolDesc = `评估已有证据是否达到指定证明标准。

证明标准等级：
- beyond_reasonable_doubt: 排除合理怀疑（>= 95% + 至少3条证据）
- high_probability: 高度盖然性（>= 80% + 至少2条证据）
- preponderance: 优势证据（> 50%）
- substantial_evidence: 实质性证据（至少1条有效直接证据）
- prima_facie: 初步证据（至少1条表面有效证据）

返回是否达标、置信度及推理说明。`

type standardTool struct{}

func newStandardTool() *agentcore.Tool {
	t := &standardTool{}
	return &agentcore.Tool{
		Name:        StandardToolName,
		Description: standardToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"standard": map[string]any{
					"type": "string",
					"enum": []string{"beyond_reasonable_doubt", "high_probability", "preponderance", "substantial_evidence", "prima_facie"},
				},
				"supporting_count": map[string]any{
					"type":        "integer",
					"description": "有效支持性证据数量",
				},
				"total_count": map[string]any{
					"type":        "integer",
					"description": "证据总数",
				},
				"gaps": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "证据缺口描述列表",
				},
			},
			"required":             []string{"standard", "supporting_count", "total_count"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type standardArgs struct {
	Standard        string   `json:"standard"`
	SupportingCount int      `json:"supporting_count"`
	TotalCount      int      `json:"total_count"`
	Gaps            []string `json:"gaps"`
}

func (t *standardTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var p standardArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if p.Standard == "" {
		return nil, fmt.Errorf("缺少必填字段 'standard'")
	}

	result := AssessProofStandard(StandardOfProof(p.Standard), p.SupportingCount, p.TotalCount, p.Gaps)

	return map[string]any{
		"met":                result.Met,
		"standard":           result.Standard,
		"confidence":         result.Confidence,
		"supporting_count":   result.SupportingCount,
		"contradicting_count": result.ContradictingCount,
		"reasoning":          result.Reasoning,
		"gaps":               result.Gaps,
	}, nil
}
```

- [ ] **Step 4: 删除 tool_stubs_placeholder.go 中的 newStandardTool 占位**

- [ ] **Step 5: 运行测试验证通过**

```bash
go test ./domains/evidence/... -run TestAssessStandard -count=1 -v
```
Expected: 3 PASS

- [ ] **Step 6: Commit**

```bash
git add domains/evidence/tool_standard.go domains/evidence/tool_stubs_placeholder.go domains/evidence/tools_test.go
git commit -m "feat(evidence): add assess_standard tool for proof standard evaluation"
```

---

### Task 5: `detect_conflict` — 证据冲突检测工具

**Files:**
- Create: `domains/evidence/tool_conflict.go`
- Modify: `domains/evidence/tools_test.go` (追加测试)

- [ ] **Step 1: 编写 detect_conflict 测试**

在 `domains/evidence/tools_test.go` 末尾追加：

```go
func TestDetectConflictTool_DirectionConflict(t *testing.T) {
	tool := newConflictTool(nil)

	args := `{"claims":[{"claim_id":"特征A","supporting":["ev1"],"contradicting":["ev2"]},{"claim_id":"特征B","supporting":["ev3"],"contradicting":[]}]}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	conflicts, ok := m["conflicts"].([]any)
	if !ok {
		t.Fatalf("expected conflicts array, got %T", m["conflicts"])
	}
	if len(conflicts) < 1 {
		t.Fatal("expected at least 1 conflict")
	}
}

func TestDetectConflictTool_NoConflict(t *testing.T) {
	tool := newConflictTool(nil)

	args := `{"claims":[{"claim_id":"特征C","supporting":["ev4"],"contradicting":[]}]}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	conflicts := m["conflicts"].([]any)
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts, got %d", len(conflicts))
	}
}

func TestDetectConflictTool_EmptyClaims(t *testing.T) {
	tool := newConflictTool(nil)

	args := `{"claims":[]}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	conflicts := m["conflicts"].([]any)
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts, got %d", len(conflicts))
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./domains/evidence/... -run TestDetectConflict -count=1 -v
```

- [ ] **Step 3: 实现 detect_conflict**

```go
// domains/evidence/tool_conflict.go
package evidence

import (
	"context"
	"encoding/json"
	"fmt"

	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
	"github.com/xujian519/mady/agentcore"
)

const ConflictToolName = "detect_conflict"

const conflictToolDesc = `检测多条证据之间的冲突关系，包括方向冲突（同一主张同时有支持和反对证据）和来源冲突（同一来源内含矛盾内容）。

输入为每条主张及其支持/反对证据的 span ID 列表，返回冲突类型、描述和涉及的证据 ID。`

type conflictTool struct {
	engine *DefaultEngine
}

func newConflictTool(engine *DefaultEngine) *agentcore.Tool {
	t := &conflictTool{engine: engine}
	return &agentcore.Tool{
		Name:        ConflictToolName,
		Description: conflictToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"claims": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"claim_id":      map[string]any{"type": "string"},
							"supporting":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
							"contradicting": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
						"required": []string{"claim_id"},
					},
				},
			},
			"required":             []string{"claims"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type conflictClaimInput struct {
	ClaimID       string   `json:"claim_id"`
	Supporting    []string `json:"supporting"`
	Contradicting []string `json:"contradicting"`
}

type conflictArgs struct {
	Claims []conflictClaimInput `json:"claims"`
}

func (t *conflictTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var p conflictArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if len(p.Claims) == 0 {
		return map[string]any{"conflicts": []any{}}, nil
	}

	cb := agentcore_evidence.NewClaimBinding()
	for _, c := range p.Claims {
		for _, sid := range c.Supporting {
			span := agentcore_evidence.EvidenceSpan{
				ID:        sid,
				Direction: agentcore_evidence.DirectionSupporting,
				ClaimRefs: []string{c.ClaimID},
			}
			cb.RegisterSpan(span)
		}
		for _, sid := range c.Contradicting {
			span := agentcore_evidence.EvidenceSpan{
				ID:        sid,
				Direction: agentcore_evidence.DirectionContradicting,
				ClaimRefs: []string{c.ClaimID},
			}
			cb.RegisterSpan(span)
		}
	}

	detector := agentcore_evidence.NewConflictDetector(cb)
	conflicts := detector.Detect()

	var out []map[string]any
	for _, c := range conflicts {
		out = append(out, map[string]any{
			"type":        string(c.Type),
			"description": c.Description,
			"span_ids":    c.SpanIDs,
		})
	}
	return map[string]any{"conflicts": out}, nil
}
```

- [ ] **Step 4: 删除 tool_stubs_placeholder.go 中的 newConflictTool 占位**

- [ ] **Step 5: 运行测试验证通过**

```bash
go test ./domains/evidence/... -run TestDetectConflict -count=1 -v
```
Expected: 3 PASS

- [ ] **Step 6: 运行全部 tool 测试确认无回归**

```bash
go test ./domains/evidence/... -run "TestJudgeTriple|TestCheckBurden|TestAssessStandard|TestDetectConflict|TestEvidenceDomainExtension" -count=1 -v
```
Expected: 全部 PASS

- [ ] **Step 7: Commit**

```bash
git add domains/evidence/tool_conflict.go domains/evidence/tool_stubs_placeholder.go domains/evidence/tools_test.go
git commit -m "feat(evidence): add detect_conflict tool for evidence conflict detection"
```

---

### Task 6: `judge_type_specific` — 类型特定判断工具

**Files:**
- Create: `domains/evidence/tool_type_specific.go`
- Modify: `domains/evidence/tools_test.go` (追加测试)
- Modify: `domains/evidence/tool_stubs_placeholder.go` (删除最后一行占位，然后删除此文件)

- [ ] **Step 1: 编写 judge_type_specific 测试**

在 `domains/evidence/tools_test.go` 末尾追加：

```go
func TestJudgeTypeSpecificTool_InternetPublic(t *testing.T) {
	engine := NewEngine(nil)
	tool := newTypeSpecificTool(engine)

	args := `{"source_uri":"https://example.com/product-page"}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["evidence_type"] != "internet_publication" && m["evidence_type"] != "general" {
		t.Errorf("unexpected evidence_type: %v", m["evidence_type"])
	}
	// 互联网公开应有内容完整性字段
	if _, ok := m["content_integrity"]; !ok {
		t.Error("missing content_integrity field for internet evidence")
	}
}

func TestJudgeTypeSpecificTool_Patent(t *testing.T) {
	engine := NewEngine(nil)
	tool := newTypeSpecificTool(engine)

	args := `{"source_uri":"patent:CN12345678A"}`
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	// 专利文献应有可信度评估
	if _, ok := m["evidence_type"]; !ok {
		t.Error("missing evidence_type")
	}
}

func TestJudgeTypeSpecificTool_EmptyURI(t *testing.T) {
	engine := NewEngine(nil)
	tool := newTypeSpecificTool(engine)

	args := `{"source_uri":""}`
	_, err := tool.Func(context.Background(), json.RawMessage(args))
	if err == nil {
		t.Fatal("expected error for empty source_uri")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./domains/evidence/... -run TestJudgeTypeSpecific -count=1 -v
```

- [ ] **Step 3: 实现 judge_type_specific**

```go
// domains/evidence/tool_type_specific.go
package evidence

import (
	"context"
	"encoding/json"
	"fmt"

	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
	"github.com/xujian519/mady/agentcore"
)

const TypeSpecificToolName = "judge_type_specific"

const typeSpecificToolDesc = `根据证据类型进行专门检查，包括：

- internet_publication: 平台可信度、内容完整性、公开意图
- public_use: 四要件检查（时间/地点/方式/公众可获取性）
- foreign_language: 翻译状态
- overseas: 公证认证状态
- electronic: 平台可信度
- notarial_certificate: 公证状态

输入证据来源 URI，自动推断证据类型并执行相应检查。`

type typeSpecificTool struct {
	engine *DefaultEngine
}

func newTypeSpecificTool(engine *DefaultEngine) *agentcore.Tool {
	t := &typeSpecificTool{engine: engine}
	return &agentcore.Tool{
		Name:        TypeSpecificToolName,
		Description: typeSpecificToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source_uri": map[string]any{
					"type":        "string",
					"description": "证据来源 URI，用于自动推断证据类型",
				},
				"evidence_type_hint": map[string]any{
					"type":        "string",
					"description": "手动指定证据类型（可选），不指定则根据 URI 自动推断",
				},
			},
			"required":             []string{"source_uri"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type typeSpecificArgs struct {
	SourceURI       string `json:"source_uri"`
	EvidenceTypeHint string `json:"evidence_type_hint"`
}

func (t *typeSpecificTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var p typeSpecificArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if p.SourceURI == "" {
		return nil, fmt.Errorf("缺少必填字段 'source_uri'")
	}

	span := agentcore_evidence.EvidenceSpan{
		ID:        "type_specific_input",
		SourceURI: p.SourceURI,
	}

	judgment, err := t.engine.Judge(span)
	if err != nil {
		return nil, fmt.Errorf("证据判断失败: %w", err)
	}

	if judgment.TypeSpecificJudgment == nil {
		return map[string]any{
			"evidence_type": inferEvidenceType(p.SourceURI),
			"note":          "无类型特定判断结果",
		}, nil
	}

	ts := judgment.TypeSpecificJudgment
	m := map[string]any{
		"evidence_type":   string(ts.EvidenceType),
		"platform_category": ts.PlatformCategory,
	}

	if ts.PlatformCredibility != nil {
		m["platform_credibility"] = string(*ts.PlatformCredibility)
	}
	if ts.ContentIntegrity != "" {
		m["content_integrity"] = string(ts.ContentIntegrity)
	}
	if ts.PublicIntent != "" {
		m["public_intent"] = string(ts.PublicIntent)
	}
	if ts.TranslationStatus != "" {
		m["translation_status"] = ts.TranslationStatus
	}
	if ts.NotarizationStatus != "" {
		m["notarization_status"] = ts.NotarizationStatus
	}
	if ts.FourElementsCheck != nil {
		m["four_elements_check"] = map[string]any{
			"time":          map[string]any{"met": ts.FourElementsCheck.TimeElement.Met, "score": ts.FourElementsCheck.TimeElement.Score, "detail": ts.FourElementsCheck.TimeElement.Detail},
			"place":         map[string]any{"met": ts.FourElementsCheck.PlaceElement.Met, "score": ts.FourElementsCheck.PlaceElement.Score, "detail": ts.FourElementsCheck.PlaceElement.Detail},
			"method":        map[string]any{"met": ts.FourElementsCheck.MethodElement.Met, "score": ts.FourElementsCheck.MethodElement.Score, "detail": ts.FourElementsCheck.MethodElement.Detail},
			"accessibility": map[string]any{"met": ts.FourElementsCheck.Accessibility.Met, "score": ts.FourElementsCheck.Accessibility.Score, "detail": ts.FourElementsCheck.Accessibility.Detail},
		}
	}
	return m, nil
}
```

- [ ] **Step 4: 删除 tool_stubs_placeholder.go（全部 5 个函数已实现）**

```bash
rm domains/evidence/tool_stubs_placeholder.go
```

- [ ] **Step 5: 运行全部工具测试**

```bash
go test ./domains/evidence/... -run "TestJudgeTriple|TestCheckBurden|TestAssessStandard|TestDetectConflict|TestJudgeTypeSpecific|TestEvidenceDomainExtension" -count=1 -v
```
Expected: 全部 PASS

- [ ] **Step 6: Commit**

```bash
git add domains/evidence/tool_type_specific.go
git rm domains/evidence/tool_stubs_placeholder.go
git add domains/evidence/tools_test.go
git commit -m "feat(evidence): add judge_type_specific tool and complete all 5 tools"
```

---

### Task 7: 注册 EvidenceDomainExtension 到 Agent 框架

**Files:**
- Modify: `cmd/mady/framework.go`

- [ ] **Step 1: 找到 evidence.NewExtension() 注册位置**

在 `cmd/mady/framework.go:501` 处，`evidence.NewExtension()` 是 agentcore/evidence 的扩展。在其后添加领域证据扩展。

- [ ] **Step 2: 添加 import 和注册代码**

在 `cmd/mady/framework.go`：

import 段添加：
```go
domainEvidence "github.com/xujian519/mady/domains/evidence"
```

在 `fc.BaseConfig.Extensions` append 块中（`evidence.NewExtension()` 之后）添加：
```go
fc.BaseConfig.Extensions = append(fc.BaseConfig.Extensions,
    fc.PlanModeExt,
    evidence.NewExtension(),
    domainEvidence.NewDomainExtension(nil),  // 新增：领域证据判断工具
)
```

- [ ] **Step 3: 编译验证**

```bash
go build ./cmd/mady/...
```
Expected: 编译成功

- [ ] **Step 4: 验证工具可见性**

```bash
go test ./cmd/mady/... -run TestFramework -count=1 2>&1 | head -5
```

- [ ] **Step 5: Commit**

```bash
git add cmd/mady/framework.go
git commit -m "feat(evidence): register EvidenceDomainExtension in agent framework"
```

---

### Task 8: CLI 子命令

**Files:**
- Create: `cmd/mady/evidence.go`
- Create: `cmd/mady/evidence_test.go`
- Modify: `cmd/mady/main.go` (添加路由)

- [ ] **Step 1: 编写 CLI 测试**

```go
// cmd/mady/evidence_test.go
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestRunEvidenceCLI_Triple(t *testing.T) {
	input := `{"source_uri":"patent:CN12345678A","snippet":"权利要求1公开了一种图像识别方法"}`
	r := strings.NewReader(input)
	var buf bytes.Buffer

	exitCode := runEvidenceAction("triple", r, &buf, os.Stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, buf.String())
	}
	if _, ok := result["overall"]; !ok {
		t.Error("missing 'overall' in output")
	}
}

func TestRunEvidenceCLI_Burden(t *testing.T) {
	input := `{"scenario":"patent_infringement"}`
	r := strings.NewReader(input)
	var buf bytes.Buffer

	exitCode := runEvidenceAction("burden", r, &buf, os.Stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["holder"] != "专利权人" {
		t.Errorf("expected holder 专利权人, got %v", result["holder"])
	}
}

func TestRunEvidenceCLI_InvalidAction(t *testing.T) {
	r := strings.NewReader("{}")
	var buf bytes.Buffer

	exitCode := runEvidenceAction("invalid_action", r, &buf, os.Stderr)
	if exitCode != 1 {
		t.Errorf("expected exit 1 for invalid action, got %d", exitCode)
	}
}

func TestRunEvidenceCLI_InvalidJSON(t *testing.T) {
	r := strings.NewReader("not json")
	var buf bytes.Buffer

	exitCode := runEvidenceAction("triple", r, &buf, os.Stderr)
	if exitCode != 1 {
		t.Errorf("expected exit 1 for invalid JSON, got %d", exitCode)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./cmd/mady/... -run TestRunEvidenceCLI -count=1 -v
```
Expected: `undefined: runEvidenceAction`

- [ ] **Step 3: 实现 CLI**

```go
// cmd/mady/evidence.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/xujian519/mady/domains/evidence"
	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
)

const evidenceUsage = `用法: mady evidence <action> [--file <path>]

Actions:
  triple        证据三性审查（关联性/合法性/真实性）
  burden        举证责任查询
  standard      证明标准评估
  conflict      证据冲突检测
  type-specific 类型特定判断

输入方式:
  echo '{"source_uri":"...","snippet":"..."}' | mady evidence triple
  mady evidence triple --file evidence.json

所有命令从 stdin 读取 JSON（或通过 --file 从文件读取），结果输出 JSON 到 stdout，
错误输出到 stderr。
`

func runEvidenceCLI(args []string) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, evidenceUsage)
		os.Exit(2)
	}

	action := args[0]
	var filePath string

	// 解析 --file flag
	remaining := args[1:]
	for i := 0; i < len(remaining); i++ {
		if remaining[i] == "--file" && i+1 < len(remaining) {
			filePath = remaining[i+1]
			i++
		}
	}

	var input io.Reader
	if filePath != "" {
		f, err := os.Open(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "无法打开文件: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		input = f
	} else {
		input = os.Stdin
	}

	os.Exit(runEvidenceAction(action, input, os.Stdout, os.Stderr))
}

func runEvidenceAction(action string, input io.Reader, stdout, stderr io.Writer) int {
	data, err := io.ReadAll(input)
	if err != nil {
		fmt.Fprintf(stderr, "读取输入失败: %v\n", err)
		return 1
	}
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		fmt.Fprintf(stderr, "输入为空\n")
		return 1
	}

	engine := evidence.NewEngine(nil)

	switch action {
	case "triple":
		return execTriple(engine, data, stdout, stderr)
	case "burden":
		return execBurden(stdout, stderr, data)
	case "standard":
		return execStandard(stdout, stderr, data)
	case "conflict":
		return execConflict(stdout, stderr, data)
	case "type-specific":
		return execTypeSpecific(engine, data, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "未知 action: %s\n可用: triple, burden, standard, conflict, type-specific\n", action)
		return 1
	}
}

func execTriple(engine *evidence.DefaultEngine, data []byte, stdout, stderr io.Writer) int {
	var args struct {
		SourceURI string `json:"source_uri"`
		Snippet   string `json:"snippet"`
	}
	if err := json.Unmarshal(data, &args); err != nil {
		fmt.Fprintf(stderr, "JSON 解析失败: %v\n", err)
		return 1
	}
	if args.Snippet == "" {
		fmt.Fprintln(stderr, "缺少必填字段: snippet")
		return 1
	}

	span := agentcore_evidence.EvidenceSpan{
		ID:        "cli_input",
		SourceURI: args.SourceURI,
		Snippet:   args.Snippet,
	}
	judgment, err := engine.Judge(span)
	if err != nil {
		fmt.Fprintf(stderr, "判断失败: %v\n", err)
		return 1
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	enc.Encode(evidenceJudgmentToMap(judgment))
	return 0
}

func evidenceJudgmentToMap(j *evidence.EvidenceJudgment) map[string]any {
	// 此函数需要访问 EvidenceJudgment 的导出字段。
	// 我们直接构造 map，因为 EvidenceJudgment 所有字段都是导出的。
	m := map[string]any{
		"overall":   j.OverallScore,
		"confidence": j.Confidence,
		"reasoning":  j.Reasoning,
	}
	if j.RelevanceJudgment != nil {
		m["relevance"] = map[string]any{
			"score": j.RelevanceJudgment.Score, "level": j.RelevanceJudgment.Level,
			"reasoning": j.RelevanceJudgment.Reasoning,
		}
	}
	if j.LegalityJudgment != nil {
		m["legality"] = map[string]any{
			"score": j.LegalityJudgment.Score, "level": j.LegalityJudgment.Level,
			"reasoning": j.LegalityJudgment.Reasoning,
		}
	}
	if j.AuthenticityJudgment != nil {
		m["authenticity"] = map[string]any{
			"score": j.AuthenticityJudgment.Score, "level": j.AuthenticityJudgment.Level,
			"reasoning": j.AuthenticityJudgment.Reasoning,
		}
	}
	var issues []map[string]string
	for _, issue := range j.FlaggedIssues {
		issues = append(issues, map[string]string{
			"type": issue.Type, "description": issue.Description, "severity": issue.Severity,
		})
	}
	if len(issues) > 0 {
		m["issues"] = issues
	}
	return m
}

func execBurden(stdout, stderr io.Writer, data []byte) int {
	var args struct {
		Scenario string `json:"scenario"`
	}
	if err := json.Unmarshal(data, &args); err != nil {
		fmt.Fprintf(stderr, "JSON 解析失败: %v\n", err)
		return 1
	}
	if args.Scenario == "" {
		fmt.Fprintln(stderr, "缺少必填字段: scenario")
		return 1
	}
	result := evidence.DetermineBurden(evidence.BurdenScenario(args.Scenario), nil)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]any{
		"holder":       result.BurdenHolder,
		"standard":     result.Standard,
		"has_shifted":  result.HasShifted,
		"shift_reason": result.ShiftReason,
		"reasoning":    result.Reasoning,
	})
	return 0
}

func execStandard(stdout, stderr io.Writer, data []byte) int {
	var args struct {
		Standard        string   `json:"standard"`
		SupportingCount int      `json:"supporting_count"`
		TotalCount      int      `json:"total_count"`
		Gaps            []string `json:"gaps"`
	}
	if err := json.Unmarshal(data, &args); err != nil {
		fmt.Fprintf(stderr, "JSON 解析失败: %v\n", err)
		return 1
	}
	if args.Standard == "" {
		fmt.Fprintln(stderr, "缺少必填字段: standard")
		return 1
	}
	result := evidence.AssessProofStandard(evidence.StandardOfProof(args.Standard), args.SupportingCount, args.TotalCount, args.Gaps)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]any{
		"met":                 result.Met,
		"standard":            result.Standard,
		"confidence":          result.Confidence,
		"supporting_count":    result.SupportingCount,
		"contradicting_count": result.ContradictingCount,
		"reasoning":           result.Reasoning,
		"gaps":                result.Gaps,
	})
	return 0
}

func execConflict(stdout, stderr io.Writer, data []byte) int {
	var args struct {
		Claims []struct {
			ClaimID       string   `json:"claim_id"`
			Supporting    []string `json:"supporting"`
			Contradicting []string `json:"contradicting"`
		} `json:"claims"`
	}
	if err := json.Unmarshal(data, &args); err != nil {
		fmt.Fprintf(stderr, "JSON 解析失败: %v\n", err)
		return 1
	}

	cb := agentcore_evidence.NewClaimBinding()
	for _, c := range args.Claims {
		for _, sid := range c.Supporting {
			cb.RegisterSpan(agentcore_evidence.EvidenceSpan{
				ID:        sid,
				Direction: agentcore_evidence.DirectionSupporting,
				ClaimRefs: []string{c.ClaimID},
			})
		}
		for _, sid := range c.Contradicting {
			cb.RegisterSpan(agentcore_evidence.EvidenceSpan{
				ID:        sid,
				Direction: agentcore_evidence.DirectionContradicting,
				ClaimRefs: []string{c.ClaimID},
			})
		}
	}

	detector := agentcore_evidence.NewConflictDetector(cb)
	conflicts := detector.Detect()

	var out []map[string]any
	for _, c := range conflicts {
		out = append(out, map[string]any{
			"type":        string(c.Type),
			"description": c.Description,
			"span_ids":    c.SpanIDs,
		})
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]any{"conflicts": out})
	return 0
}

func execTypeSpecific(engine *evidence.DefaultEngine, data []byte, stdout, stderr io.Writer) int {
	var args struct {
		SourceURI string `json:"source_uri"`
	}
	if err := json.Unmarshal(data, &args); err != nil {
		fmt.Fprintf(stderr, "JSON 解析失败: %v\n", err)
		return 1
	}
	if args.SourceURI == "" {
		fmt.Fprintln(stderr, "缺少必填字段: source_uri")
		return 1
	}

	span := agentcore_evidence.EvidenceSpan{ID: "cli_type_input", SourceURI: args.SourceURI}
	judgment, err := engine.Judge(span)
	if err != nil {
		fmt.Fprintf(stderr, "判断失败: %v\n", err)
		return 1
	}

	ts := judgment.TypeSpecificJudgment
	if ts == nil {
		enc := json.NewEncoder(stdout)
		enc.Encode(map[string]any{"note": "无类型特定判断结果"})
		return 0
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	enc.Encode(typeSpecificToMap(ts))
	return 0
}

func typeSpecificToMap(ts *evidence.TypeSpecificJudgment) map[string]any {
	m := map[string]any{"evidence_type": string(ts.EvidenceType)}
	if ts.PlatformCredibility != nil {
		m["platform_credibility"] = string(*ts.PlatformCredibility)
	}
	if ts.ContentIntegrity != "" {
		m["content_integrity"] = string(ts.ContentIntegrity)
	}
	if ts.PublicIntent != "" {
		m["public_intent"] = string(ts.PublicIntent)
	}
	if ts.FourElementsCheck != nil {
		m["four_elements_check"] = map[string]any{
			"time":          map[string]any{"met": ts.FourElementsCheck.TimeElement.Met, "score": ts.FourElementsCheck.TimeElement.Score, "detail": ts.FourElementsCheck.TimeElement.Detail},
			"place":         map[string]any{"met": ts.FourElementsCheck.PlaceElement.Met, "score": ts.FourElementsCheck.PlaceElement.Score, "detail": ts.FourElementsCheck.PlaceElement.Detail},
			"method":        map[string]any{"met": ts.FourElementsCheck.MethodElement.Met, "score": ts.FourElementsCheck.MethodElement.Score, "detail": ts.FourElementsCheck.MethodElement.Detail},
			"accessibility": map[string]any{"met": ts.FourElementsCheck.Accessibility.Met, "score": ts.FourElementsCheck.Accessibility.Score, "detail": ts.FourElementsCheck.Accessibility.Detail},
		}
	}
	return m
}
```

- [ ] **Step 4: 在 main.go 中添加路由**

编辑 `cmd/mady/main.go`，在子命令路由中添加：

```go
case "evidence":
    runEvidenceCLI(args)
```

- [ ] **Step 5: 运行 CLI 测试**

```bash
go test ./cmd/mady/... -run TestRunEvidenceCLI -count=1 -v
```
Expected: 4 PASS

- [ ] **Step 6: 编译验证**

```bash
go build ./cmd/mady/...
```
Expected: 成功

- [ ] **Step 7: Commit**

```bash
git add cmd/mady/evidence.go cmd/mady/evidence_test.go cmd/mady/main.go
git commit -m "feat(evidence): add mady evidence CLI with 5 subcommands (triple/burden/standard/conflict/type-specific)"
```

---

### Task 9: HTTP API 异步服务

**Files:**
- Create: `server/evidence.go`
- Create: `server/evidence_test.go`
- Modify: `server/server.go` (注册端点)

- [ ] **Step 1: 编写 API 测试**

```go
// server/evidence_test.go
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleEvidenceJudge_SubmitAndPoll(t *testing.T) {
	srv := &Server{
		evidenceTasks: make(map[string]*evidenceTask),
	}
	// 提交任务
	body := `{"source_uri":"patent:CN12345678A","snippet":"test snippet"}`
	req := httptest.NewRequest("POST", "/v1/evidence/judge", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleEvidenceJudge(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var submitResp struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
	}
	json.Unmarshal(w.Body.Bytes(), &submitResp)
	if submitResp.TaskID == "" {
		t.Fatal("expected task_id in response")
	}
	if submitResp.Status != "pending" {
		t.Errorf("expected pending, got %s", submitResp.Status)
	}

	// 轮询任务状态
	req2 := httptest.NewRequest("GET", "/v1/evidence/judge/"+submitResp.TaskID, nil)
	w2 := httptest.NewRecorder()
	srv.handleEvidenceJudgeStatus(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestHandleEvidenceJudge_InvalidBody(t *testing.T) {
	srv := &Server{
		evidenceTasks: make(map[string]*evidenceTask),
	}
	req := httptest.NewRequest("POST", "/v1/evidence/judge", bytes.NewReader([]byte(`bad json`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleEvidenceJudge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleEvidenceJudgeStatus_NotFound(t *testing.T) {
	srv := &Server{
		evidenceTasks: make(map[string]*evidenceTask),
	}
	req := httptest.NewRequest("GET", "/v1/evidence/judge/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.handleEvidenceJudgeStatus(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleEvidenceBurden(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest("GET", "/v1/evidence/burden/patent_infringement", nil)
	w := httptest.NewRecorder()
	srv.handleEvidenceBurden(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["holder"] != "专利权人" {
		t.Errorf("expected 专利权人, got %v", result["holder"])
	}
}

func TestHandleEvidenceConflict(t *testing.T) {
	srv := &Server{}
	body := `{"claims":[{"claim_id":"A","supporting":["e1"],"contradicting":["e2"]}]}`
	req := httptest.NewRequest("POST", "/v1/evidence/conflict", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleEvidenceConflict(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	if _, ok := result["conflicts"]; !ok {
		t.Error("missing conflicts in response")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./server/... -run TestHandleEvidence -count=1 -v
```
Expected: 编译错误 `evidenceTasks undefined`

- [ ] **Step 3: 实现 HTTP API**

```go
// server/evidence.go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
	"github.com/xujian519/mady/domains/evidence"
)

// evidenceTask 是异步证据判断任务的内部表示。
type evidenceTask struct {
	ID      string
	Status  string // pending / running / completed / failed
	Result  *evidence.EvidenceJudgment
	Results []*evidence.EvidenceJudgment
	Err     error
	mu      sync.RWMutex
	doneCh  chan struct{}
}

func (t *evidenceTask) markRunning() {
	t.mu.Lock()
	t.Status = "running"
	t.mu.Unlock()
}

func (t *evidenceTask) markCompleted(result *evidence.EvidenceJudgment) {
	t.mu.Lock()
	t.Status = "completed"
	t.Result = result
	t.mu.Unlock()
	close(t.doneCh)
}

func (t *evidenceTask) markFailed(err error) {
	t.mu.Lock()
	t.Status = "failed"
	t.Err = err
	t.mu.Unlock()
	close(t.doneCh)
}

func (t *evidenceTask) snapshot() map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()
	m := map[string]any{
		"task_id": t.ID,
		"status":  t.Status,
	}
	if t.Err != nil {
		m["error"] = t.Err.Error()
	}
	if t.Result != nil {
		m["result"] = judgmentToAPIMap(t.Result)
	}
	if t.Results != nil {
		results := make([]map[string]any, len(t.Results))
		for i, r := range t.Results {
			results[i] = judgmentToAPIMap(r)
		}
		m["results"] = results
	}
	return m
}

var evidenceTaskCounter int

func newEvidenceTaskID() string {
	evidenceTaskCounter++
	return fmt.Sprintf("evj_%d", evidenceTaskCounter)
}

// handleEvidenceJudge 处理 POST /v1/evidence/judge
func (s *Server) handleEvidenceJudge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceURI string `json:"source_uri"`
		Snippet   string `json:"snippet"`
		Scenario  string `json:"scenario"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体 JSON 无效: " + err.Error()})
		return
	}
	if req.Snippet == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少必填字段 'snippet'"})
		return
	}

	task := &evidenceTask{
		ID:     newEvidenceTaskID(),
		Status: "pending",
		doneCh: make(chan struct{}),
	}
	s.evidenceTasksMu.Lock()
	s.evidenceTasks[task.ID] = task
	s.evidenceTasksMu.Unlock()

	go s.runEvidenceJudge(task, req.SourceURI, req.Snippet)

	writeJSON(w, http.StatusAccepted, map[string]string{
		"task_id": task.ID,
		"status":  "pending",
	})
}

func (s *Server) runEvidenceJudge(task *evidenceTask, sourceURI, snippet string) {
	task.markRunning()

	engine := evidence.NewEngine(nil)
	span := agentcore_evidence.EvidenceSpan{
		ID:        task.ID,
		SourceURI: sourceURI,
		Snippet:   snippet,
	}
	judgment, err := engine.Judge(span)
	if err != nil {
		task.markFailed(err)
		slog.Error("evidence judge failed", "task_id", task.ID, "err", err)
		return
	}
	task.markCompleted(judgment)
}

// handleEvidenceJudgeStatus 处理 GET /v1/evidence/judge/{task_id}
func (s *Server) handleEvidenceJudgeStatus(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	s.evidenceTasksMu.Lock()
	task, ok := s.evidenceTasks[taskID]
	s.evidenceTasksMu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "任务不存在: " + taskID})
		return
	}

	// 等待任务完成或超时
	select {
	case <-task.doneCh:
	case <-r.Context().Done():
		return
	case <-time.After(30 * time.Second):
	}

	writeJSON(w, http.StatusOK, task.snapshot())
}

// handleEvidenceJudgeBatch 处理 POST /v1/evidence/judge/batch
func (s *Server) handleEvidenceJudgeBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []struct {
			SourceURI string `json:"source_uri"`
			Snippet   string `json:"snippet"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体无效: " + err.Error()})
		return
	}
	if len(req.Items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "items 不能为空"})
		return
	}

	task := &evidenceTask{
		ID:     "evb_" + newEvidenceTaskID()[4:],
		Status: "pending",
		doneCh: make(chan struct{}),
	}
	s.evidenceTasksMu.Lock()
	s.evidenceTasks[task.ID] = task
	s.evidenceTasksMu.Unlock()

	go s.runEvidenceJudgeBatch(task, req.Items)

	writeJSON(w, http.StatusAccepted, map[string]string{
		"task_id": task.ID,
		"status":  "pending",
	})
}

func (s *Server) runEvidenceJudgeBatch(task *evidenceTask, items []struct {
	SourceURI string `json:"source_uri"`
	Snippet   string `json:"snippet"`
}) {
	task.markRunning()

	engine := evidence.NewEngine(nil)
	results := make([]*evidence.EvidenceJudgment, len(items))
	for i, item := range items {
		span := agentcore_evidence.EvidenceSpan{
			ID:        fmt.Sprintf("%s_%d", task.ID, i),
			SourceURI: item.SourceURI,
			Snippet:   item.Snippet,
		}
		judgment, err := engine.Judge(span)
		if err != nil {
			task.markFailed(fmt.Errorf("item %d: %w", i, err))
			return
		}
		results[i] = judgment
	}
	task.mu.Lock()
	task.Status = "completed"
	task.Results = results
	task.mu.Unlock()
	close(task.doneCh)
}

// handleEvidenceBurden 处理 GET /v1/evidence/burden/{scenario}
func (s *Server) handleEvidenceBurden(w http.ResponseWriter, r *http.Request) {
	scenario := r.PathValue("scenario")
	if scenario == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 scenario"})
		return
	}
	result := evidence.DetermineBurden(evidence.BurdenScenario(scenario), nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"holder":       result.BurdenHolder,
		"standard":     result.Standard,
		"has_shifted":  result.HasShifted,
		"shift_reason": result.ShiftReason,
		"reasoning":    result.Reasoning,
	})
}

// handleEvidenceStandard 处理 GET /v1/evidence/standard/{standard}
func (s *Server) handleEvidenceStandard(w http.ResponseWriter, r *http.Request) {
	standardParam := r.PathValue("standard")
	if standardParam == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 standard"})
		return
	}
	result := evidence.AssessProofStandard(evidence.StandardOfProof(standardParam), 0, 0, nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"met":        result.Met,
		"standard":   result.Standard,
		"confidence": result.Confidence,
		"reasoning":  result.Reasoning,
	})
}

// handleEvidenceConflict 处理 POST /v1/evidence/conflict
func (s *Server) handleEvidenceConflict(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Claims []struct {
			ClaimID       string   `json:"claim_id"`
			Supporting    []string `json:"supporting"`
			Contradicting []string `json:"contradicting"`
		} `json:"claims"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体无效: " + err.Error()})
		return
	}

	cb := agentcore_evidence.NewClaimBinding()
	for _, c := range req.Claims {
		for _, sid := range c.Supporting {
			cb.RegisterSpan(agentcore_evidence.EvidenceSpan{
				ID: sid, Direction: agentcore_evidence.DirectionSupporting, ClaimRefs: []string{c.ClaimID},
			})
		}
		for _, sid := range c.Contradicting {
			cb.RegisterSpan(agentcore_evidence.EvidenceSpan{
				ID: sid, Direction: agentcore_evidence.DirectionContradicting, ClaimRefs: []string{c.ClaimID},
			})
		}
	}

	detector := agentcore_evidence.NewConflictDetector(cb)
	conflicts := detector.Detect()

	var out []map[string]any
	for _, c := range conflicts {
		out = append(out, map[string]any{
			"type":        string(c.Type),
			"description": c.Description,
			"span_ids":    c.SpanIDs,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"conflicts": out})
}

// judgmentToAPIMap 将 EvidenceJudgment 转换为 API 响应 map。
func judgmentToAPIMap(j *evidence.EvidenceJudgment) map[string]any {
	m := map[string]any{
		"overall":   j.OverallScore,
		"confidence": j.Confidence,
		"reasoning":  j.Reasoning,
	}
	if j.RelevanceJudgment != nil {
		m["relevance"] = map[string]any{
			"score": j.RelevanceJudgment.Score, "level": j.RelevanceJudgment.Level,
			"reasoning": j.RelevanceJudgment.Reasoning,
		}
	}
	if j.LegalityJudgment != nil {
		m["legality"] = map[string]any{
			"score": j.LegalityJudgment.Score, "level": j.LegalityJudgment.Level,
			"reasoning": j.LegalityJudgment.Reasoning,
		}
	}
	if j.AuthenticityJudgment != nil {
		m["authenticity"] = map[string]any{
			"score": j.AuthenticityJudgment.Score, "level": j.AuthenticityJudgment.Level,
			"reasoning": j.AuthenticityJudgment.Reasoning,
		}
	}
	if j.TypeSpecificJudgment != nil {
		ts := j.TypeSpecificJudgment
		tsMap := map[string]any{"evidence_type": string(ts.EvidenceType)}
		if ts.PlatformCredibility != nil {
			tsMap["platform_credibility"] = string(*ts.PlatformCredibility)
		}
		if ts.ContentIntegrity != "" {
			tsMap["content_integrity"] = string(ts.ContentIntegrity)
		}
		if ts.PublicIntent != "" {
			tsMap["public_intent"] = string(ts.PublicIntent)
		}
		if ts.FourElementsCheck != nil {
			tsMap["four_elements_check"] = map[string]any{
				"time":          map[string]any{"met": ts.FourElementsCheck.TimeElement.Met, "score": ts.FourElementsCheck.TimeElement.Score, "detail": ts.FourElementsCheck.TimeElement.Detail},
				"place":         map[string]any{"met": ts.FourElementsCheck.PlaceElement.Met, "score": ts.FourElementsCheck.PlaceElement.Score, "detail": ts.FourElementsCheck.PlaceElement.Detail},
				"method":        map[string]any{"met": ts.FourElementsCheck.MethodElement.Met, "score": ts.FourElementsCheck.MethodElement.Score, "detail": ts.FourElementsCheck.MethodElement.Detail},
				"accessibility": map[string]any{"met": ts.FourElementsCheck.Accessibility.Met, "score": ts.FourElementsCheck.Accessibility.Score, "detail": ts.FourElementsCheck.Accessibility.Detail},
			}
		}
		m["type_specific"] = tsMap
	}
	var issues []map[string]string
	for _, issue := range j.FlaggedIssues {
		issues = append(issues, map[string]string{
			"type": issue.Type, "description": issue.Description, "severity": issue.Severity,
		})
	}
	if len(issues) > 0 {
		m["issues"] = issues
	}
	return m
}
```

- [ ] **Step 4: 在 server.go 中添加 evidenceTasks 字段和端点注册**

在 `server/server.go` 的 `Server` struct 中添加：

```go
evidenceTasks   map[string]*evidenceTask
evidenceTasksMu sync.Mutex
```

在 `NewServer` 中初始化：

```go
evidenceTasks: make(map[string]*evidenceTask),
```

在 `Handler()` 方法中添加端点注册：

```go
mux.HandleFunc("POST /v1/evidence/judge", s.handleEvidenceJudge)
mux.HandleFunc("POST /v1/evidence/judge/batch", s.handleEvidenceJudgeBatch)
mux.HandleFunc("GET /v1/evidence/judge/{task_id}", s.handleEvidenceJudgeStatus)
mux.HandleFunc("GET /v1/evidence/burden/{scenario}", s.handleEvidenceBurden)
mux.HandleFunc("GET /v1/evidence/standard/{standard}", s.handleEvidenceStandard)
mux.HandleFunc("POST /v1/evidence/conflict", s.handleEvidenceConflict)
```

检查 Server struct 是否已有 `sync.Mutex` 导入 — 通常在 server.go 头部 import 中已有。

- [ ] **Step 5: 运行 API 测试**

```bash
go test ./server/... -run TestHandleEvidence -count=1 -v
```
Expected: 5 PASS

- [ ] **Step 6: 编译验证**

```bash
go build ./...
```
Expected: 成功

- [ ] **Step 7: Commit**

```bash
git add server/evidence.go server/evidence_test.go server/server.go
git commit -m "feat(evidence): add HTTP evidence judgment API endpoints"
```

---

### Task 10: 无效宣告工作流接入

**Files:**
- Modify: `workflows/patent/invalidation.go`

- [ ] **Step 1: 阅读当前 invalidation.go 的完整内容以确定修改位置**

```bash
wc -l workflows/patent/invalidation.go
```

- [ ] **Step 2: 添加 import**

在 `workflows/patent/invalidation.go` 头部添加：

```go
import (
    agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
    "github.com/xujian519/mady/domains/evidence"
)
```

确保 `context` 和 `fmt` 已在 import 中。

- [ ] **Step 3: 添加新的 State Keys**

在现有 const 块中添加：

```go
const (
    // ... 现有 keys ...
    InvStateEvidenceJudgments = "inv_evidence_judgments" // []EvidenceJudgment
    InvStateValidEvidence     = "inv_valid_evidence"     // []agentcore_evidence.EvidenceSpan
    InvStateConflicts         = "inv_conflicts"          // []agentcore_evidence.Conflict
)
```

- [ ] **Step 4: 实现 judgeEvidenceNode**

```go
// judgeEvidenceNode 对 gatherEvidenceNode 检索到的证据进行三性审查。
func judgeEvidenceNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
    // 透传现有 state
    out := copyInvBaseState(state)

    // 检查是否有证据
    evidenceRaw, ok := state[InvStateEvidence]
    if !ok {
        graph.MarkDegraded(out, InvStateEvidenceJudgments, []string{},
            graph.DegradationNotImplemented,
            "证据检索未完成，跳过证据判断")
        return out, nil
    }

    spans, ok := evidenceRaw.([]agentcore_evidence.EvidenceSpan)
    if !ok || len(spans) == 0 {
        out[InvStateEvidenceJudgments] = []evidence.EvidenceJudgment{}
        return out, nil
    }

    engine := evidence.NewEngine(nil)
    judgments := make([]evidence.EvidenceJudgment, 0, len(spans))
    for _, span := range spans {
        j, err := engine.Judge(span)
        if err != nil {
            continue // 跳过无法判断的证据
        }
        judgments = append(judgments, *j)
    }
    out[InvStateEvidenceJudgments] = judgments
    return out, nil
}
```

- [ ] **Step 5: 实现 filterEvidenceNode**

```go
// filterEvidenceNode 过滤三性评分不达标的证据（overall_score < 0.5）。
func filterEvidenceNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
    out := copyInvBaseState(state)

    judgmentsRaw, ok := state[InvStateEvidenceJudgments]
    if !ok {
        graph.MarkDegraded(out, InvStateValidEvidence, []string{},
            graph.DegradationNotImplemented,
            "证据判断尚未完成，跳过证据过滤")
        return out, nil
    }

    judgments, ok := judgmentsRaw.([]evidence.EvidenceJudgment)
    if !ok {
        out[InvStateValidEvidence] = []agentcore_evidence.EvidenceSpan{}
        return out, nil
    }

    evidenceRaw, _ := state[InvStateEvidence]
    spans, _ := evidenceRaw.([]agentcore_evidence.EvidenceSpan)

    // 构建 judgment index by span ID
    judgmentBySpanID := make(map[string]evidence.EvidenceJudgment)
    for _, j := range judgments {
        judgmentBySpanID[j.SpanID] = j
    }

    var valid []agentcore_evidence.EvidenceSpan
    for _, span := range spans {
        if j, ok := judgmentBySpanID[span.ID]; ok && j.OverallScore >= 0.5 {
            valid = append(valid, span)
        }
    }
    out[InvStateValidEvidence] = valid
    return out, nil
}
```

- [ ] **Step 6: 实现 detectConflictNode**

```go
// detectConflictNode 对过滤后的有效证据进行冲突检测。
func detectConflictNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
    out := copyInvBaseState(state)

    validRaw, ok := state[InvStateValidEvidence]
    if !ok {
        out[InvStateConflicts] = []agentcore_evidence.Conflict{}
        return out, nil
    }

    spans, ok := validRaw.([]agentcore_evidence.EvidenceSpan)
    if !ok || len(spans) == 0 {
        out[InvStateConflicts] = []agentcore_evidence.Conflict{}
        return out, nil
    }

    cb := agentcore_evidence.NewClaimBinding()
    for _, span := range spans {
        cb.RegisterSpan(span)
    }

    detector := agentcore_evidence.NewConflictDetector(cb)
    conflicts := detector.Detect()
    out[InvStateConflicts] = conflicts
    return out, nil
}

// copyInvBaseState 复制无效宣告的 base state（input/claims/grounds/claimTree）。
func copyInvBaseState(state graph.PregelState) graph.PregelState {
    out := graph.PregelState{}
    for _, key := range []string{InvStateInput, InvStateClaims, InvStateGrounds, InvStateClaimTree,
        InvStateEvidence, InvStateEvidenceJudgments, InvStateValidEvidence, InvStateConflicts} {
        if v, ok := state[key]; ok {
            out[key] = v
        }
    }
    return out
}
```

- [ ] **Step 7: 修改 BuildInvalidationGraphWithOpts — 将新节点加入图**

找到构建 Pregel 图的位置（`BuildInvalidationGraphWithOpts` 函数），在有 retriever 的路径中，将 `gather_evidence → analyze_grounds` 改为 `gather_evidence → judge_evidence → filter_evidence → detect_conflict → analyze_grounds`。

找到类似如下代码的结构并修改：

```go
// 原图: ... → gather_evidence → analyze_grounds → ...
// 改为: ... → gather_evidence → judge_evidence → filter_evidence → detect_conflict → analyze_grounds → ...
```

具体修改方式：
1. 新增 `judge_evidence`、`filter_evidence`、`detect_conflict` 三个节点定义
2. 修改边路由：`gather_evidence → judge_evidence`、`judge_evidence → filter_evidence`、`filter_evidence → detect_conflict`、`detect_conflict → analyze_grounds`
3. 无 retriever 时保持现有图结构不变（降级路径）

- [ ] **Step 8: 运行现有无效宣告测试确保无回归**

```bash
go test ./workflows/patent/... -run TestInvalidation -count=1 -v
```

- [ ] **Step 9: Commit**

```bash
git add workflows/patent/invalidation.go
git commit -m "feat(evidence): integrate evidence judgment into invalidation workflow"
```

---

### Task 11: 侵权分析工作流接入

**Files:**
- Modify: `workflows/patent/infringement.go`

- [ ] **Step 1: 添加 import**

```go
import (
    agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
    "github.com/xujian519/mady/domains/evidence"
)
```

- [ ] **Step 2: 添加新 State Keys**

```go
const (
    // ... 现有 keys ...
    InfStateEvidence          = "inf_evidence"            // []agentcore_evidence.EvidenceSpan
    InfStateEvidenceJudgments = "inf_evidence_judgments"  // []evidence.EvidenceJudgment
)
```

- [ ] **Step 3: 实现 collectEvidenceNode**

```go
// collectEvidenceNode 从权利要求特征构建证据记录。
func collectEvidenceNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
    out := graph.PregelState{
        InfStatePatentClaims:   state.GetString(InfStatePatentClaims),
        InfStateAccusedProduct: state.GetString(InfStateAccusedProduct),
    }
    // 透传已有特征
    for _, key := range []string{InfStateClaimFeatures, InfStateProductFeatures} {
        if v, ok := state[key]; ok {
            out[key] = v
        }
    }

    features, ok := state[InfStateClaimFeatures].([]string)
    if !ok || len(features) == 0 {
        out[InfStateEvidence] = []agentcore_evidence.EvidenceSpan{}
        return out, nil
    }

    spans := make([]agentcore_evidence.EvidenceSpan, len(features))
    for i, feat := range features {
        spans[i] = agentcore_evidence.EvidenceSpan{
            ID:        fmt.Sprintf("inf_claim_feat_%d", i),
            Snippet:   feat,
            Direction: agentcore_evidence.DirectionNeutral,
            ClaimRefs: []string{fmt.Sprintf("feature_%d", i)},
        }
    }
    out[InfStateEvidence] = spans
    return out, nil
}
```

- [ ] **Step 4: 实现侵权专用的 judgeEvidenceNode（或复用无效宣告的）**

由于两个工作流中的 `judgeEvidenceNode` 逻辑相同（都是对 `[]EvidenceSpan` 调用 `Engine.Judge()`），将其提取为共享函数或复制实现：

```go
// infJudgeEvidenceNode 对侵权证据进行三性审查。
func infJudgeEvidenceNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
    out := copyInfBaseState(state)

    evidenceRaw, ok := state[InfStateEvidence]
    if !ok {
        out[InfStateEvidenceJudgments] = []evidence.EvidenceJudgment{}
        return out, nil
    }

    spans, ok := evidenceRaw.([]agentcore_evidence.EvidenceSpan)
    if !ok || len(spans) == 0 {
        out[InfStateEvidenceJudgments] = []evidence.EvidenceJudgment{}
        return out, nil
    }

    engine := evidence.NewEngine(nil)
    judgments := make([]evidence.EvidenceJudgment, 0, len(spans))
    for _, span := range spans {
        j, err := engine.Judge(span)
        if err != nil {
            continue
        }
        judgments = append(judgments, *j)
    }
    out[InfStateEvidenceJudgments] = judgments
    return out, nil
}

func copyInfBaseState(state graph.PregelState) graph.PregelState {
    out := graph.PregelState{}
    for _, key := range []string{InfStatePatentClaims, InfStateAccusedProduct,
        InfStateClaimFeatures, InfStateProductFeatures, InfStateEvidence, InfStateEvidenceJudgments} {
        if v, ok := state[key]; ok {
            out[key] = v
        }
    }
    return out
}
```

- [ ] **Step 5: 修改 BuildInfringementGraph — 插入新节点**

在 `parse_claims → parse_product → full_coverage` 之间插入 `collect_evidence → judge_evidence`：

```
parse_claims → parse_product → collect_evidence → inf_judge_evidence → full_coverage → ...
```

具体修改：在 `BuildInfringementGraph` 函数中添加两个节点定义和对应边。

- [ ] **Step 6: 运行侵权测试**

```bash
go test ./workflows/patent/... -run TestInfringement -count=1 -v
```

- [ ] **Step 7: 最终全量测试**

```bash
go build ./...
go test ./domains/evidence/... ./server/... ./cmd/mady/... ./workflows/patent/... -count=1
```

- [ ] **Step 8: Commit**

```bash
git add workflows/patent/infringement.go
git commit -m "feat(evidence): integrate evidence collection and judgment into infringement workflow"
```

---

## 执行概要

| Task | 文件 | 预估时间 |
|------|------|---------|
| 1 | Extension 基础设施 | 15 min |
| 2 | `judge_triple` 工具 | 15 min |
| 3 | `check_burden` 工具 | 10 min |
| 4 | `assess_standard` 工具 | 10 min |
| 5 | `detect_conflict` 工具 | 15 min |
| 6 | `judge_type_specific` 工具 | 15 min |
| 7 | 注册到 Agent 框架 | 5 min |
| 8 | CLI 子命令 | 20 min |
| 9 | HTTP API | 20 min |
| 10 | 无效宣告工作流 | 20 min |
| 11 | 侵权分析工作流 | 15 min |
| **Total** | | **~160 min** |
