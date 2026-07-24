package specdrafting

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/disclosure"
)

// ExtensionName 是 specdrafting 扩展的名称。
const ExtensionName = "specdrafting"

// 编译期接口断言。
var (
	_ agentcore.Extension    = (*Extension)(nil)
	_ agentcore.ToolProvider = (*Extension)(nil)
)

// Extension 实现 agentcore.Extension 和 ToolProvider 接口。
type Extension struct {
	mu      sync.RWMutex
	engine  *RuleEngine
	scorer  *SpecScorer
	builder *SpecBuilder
	drafter *LLMDrafter
}

// NewExtension 创建一个 specdrafting 扩展实例。
// engine 不能为 nil，否则 panic。
func NewExtension(engine *RuleEngine) *Extension {
	if engine == nil {
		panic("specdrafting: NewExtension 的 engine 参数不能为 nil")
	}
	return &Extension{
		engine:  engine,
		scorer:  NewSpecScorer(engine),
		builder: NewSpecBuilder(nil),
	}
}

// SetDrafter 设置 LLM 撰写器（可选）。
func (e *Extension) SetDrafter(d *LLMDrafter) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.drafter = d
}

// Drafter 返回 LLM 撰写器（读安全）。
func (e *Extension) Drafter() *LLMDrafter {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.drafter
}

// Name 实现 agentcore.Extension 接口。
func (e *Extension) Name() string { return ExtensionName }

// Init 实现 agentcore.Extension 接口。
func (e *Extension) Init(ctx context.Context, agent *agentcore.Agent) error { return nil }

// Dispose 实现 agentcore.Extension 接口。
func (e *Extension) Dispose() error { return nil }

// Tools 实现 ToolProvider 接口。
func (e *Extension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		{
			Name:        "draft_specification",
			Description: "根据技术交底书撰写符合中国专利法要求的完整专利说明书。当用户要求撰写说明书、写专利申请文件时，必须调用此工具，严禁自行手写说明书文本。包含技术领域、背景技术、发明/实用新型内容、附图说明和具体实施方式。支持发明和实用新型两种专利类型，自动适配机械/电学/化学/软件四大技术领域。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"disclosure": map[string]any{
						"type":        "string",
						"description": "技术交底书内容（可选），与 extraction 二选一",
					},
					"extraction": map[string]any{
						"type":        "object",
						"description": "技术交底书的结构化提取结果（可选），与 disclosure 二选一",
					},
					"patent_type": map[string]any{
						"type":        "string",
						"enum":        []string{"invention", "utility_model"},
						"description": "专利类型：发明或实用新型（默认 invention）",
					},
					"tech_domain": map[string]any{
						"type":        "string",
						"enum":        []string{"mechanical", "electrical", "chemical", "software", "general"},
						"description": "技术领域（可选，为空时自动识别）",
					},
					"has_drawings": map[string]any{
						"type":        "boolean",
						"description": "是否有附图（实用新型必须有附图）",
					},
					"claims": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "权利要求文本列表（可选，用于增强发明内容撰写）",
					},
				},
			},
			Func: e.handleDraftSpecification,
		},
		{
			Name:        "validate_specification",
			Description: "验证专利说明书是否符合中国专利法的撰写要求，包括结构完整性、清楚性、领域适配性、形式规范等16条检查规则。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"specification": map[string]any{
						"type":        "object",
						"description": "待验证的说明书，包含 title/abstract/sections 字段",
					},
				},
				"required":             []string{"specification"},
				"additionalProperties": false,
			},
			Func: e.handleValidateSpecification,
		},
	}
}

// handleDraftSpecification 处理 draft_specification 工具调用。
func (e *Extension) handleDraftSpecification(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Disclosure  string          `json:"disclosure"`
		Extraction  json.RawMessage `json:"extraction"`
		PatentType  string          `json:"patent_type"`
		TechDomain  string          `json:"tech_domain"`
		HasDrawings bool            `json:"has_drawings"`
		Claims      []string        `json:"claims"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("draft_specification: 参数解析失败: %w", err)
	}

	patentType := PatentTypeInvention
	if params.PatentType == "utility_model" {
		patentType = PatentTypeUtilityModel
	}
	domain := parseTechDomain(params.TechDomain)

	// 构造 SpecInput
	input := buildInputFromParams(params.Disclosure, params.Extraction, patentType, domain, params.HasDrawings, params.Claims)
	if input == nil {
		return nil, fmt.Errorf("draft_specification: 请提供技术交底书(disclosure)或提取结果(extraction)")
	}

	// 使用 drafter（如果可用）否则使用 builder，避免双重构建
	var output *SpecOutput
	if d := e.Drafter(); d != nil {
		output = d.Draft(*input)
	} else {
		output = e.builder.Build(*input)
	}

	// 运行规则验证（engine 经 NewExtension 保证非 nil）
	violations := e.engine.Validate(output, *input)
	for _, v := range violations {
		if v.Severity == SeverityWarning || v.Severity == SeverityError {
			output.Warnings = append(output.Warnings, "["+v.RuleName+"] "+v.Message)
		}
	}

	// 运行评分
	report := e.scorer.Score(output, *input)
	output.Score = report.OverallScore

	return output, nil
}

// handleValidateSpecification 处理 validate_specification 工具调用。
func (e *Extension) handleValidateSpecification(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Specification json.RawMessage `json:"specification"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("validate_specification: 参数解析失败: %w", err)
	}
	if len(params.Specification) == 0 {
		return nil, fmt.Errorf("validate_specification: 请提供待验证的说明书 JSON")
	}

	// 解析 SpecOutput 后评分
	var spec SpecOutput
	if err := json.Unmarshal(params.Specification, &spec); err != nil {
		return nil, fmt.Errorf("validate_specification: 说明书 JSON 解析失败: %w", err)
	}

	return e.scorer.Score(&spec, SpecInput{}), nil
}

// buildInputFromParams 从工具参数构造 SpecInput。
func buildInputFromParams(rawDisc string, extraction json.RawMessage, patentType PatentType, domain TechDomain, hasDrawings bool, claims []string) *SpecInput {
	if len(extraction) > 0 {
		var ext disclosure.ExtractionResult
		if err := json.Unmarshal(extraction, &ext); err == nil {
			input := SpecInputFromExtraction(&ext, patentType, hasDrawings, claims)
			input.TechDomain = domain
			return input
		}
	}
	if rawDisc != "" {
		return &SpecInput{
			PatentType:  patentType,
			TechDomain:  domain,
			HasDrawings: hasDrawings,
			Title:       string([]rune(rawDisc)[:min(len([]rune(rawDisc)), 80)]),
			Problems:    []string{string([]rune(rawDisc)[:min(len([]rune(rawDisc)), 200)])},
			Claims:      claims,
		}
	}
	return nil
}

// parseTechDomain 将字符串转换为 TechDomain。
func parseTechDomain(s string) TechDomain {
	switch s {
	case "mechanical":
		return DomainMechanical
	case "electrical":
		return DomainElectrical
	case "chemical":
		return DomainChemical
	case "software":
		return DomainSoftware
	default:
		return DomainGeneral
	}
}
