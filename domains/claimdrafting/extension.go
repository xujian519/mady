package claimdrafting

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionName 是 claimdrafting 扩展的名称。
const ExtensionName = "claimdrafting"

// 编译期接口断言，确保 Extension 满足 agentcore 接口。
var (
	_ agentcore.Extension    = (*Extension)(nil)
	_ agentcore.ToolProvider = (*Extension)(nil)
)

// Extension 实现 agentcore.Extension 和 ToolProvider 接口，
// 将权利要求撰写功能注册为 Agent 可用的工具。
type Extension struct {
	mu      sync.RWMutex
	engine  *RuleEngine
	scorer  *ClaimScorer
	drafter *LLMDrafter // 可选，通过 SetDrafter 注入
}

// NewExtension 创建一个 claimdrafting 扩展实例。
// engine 为共享的规则引擎实例（所有工具调用复用同一引擎）。
func NewExtension(engine *RuleEngine) *Extension {
	return &Extension{
		engine: engine,
		scorer: NewClaimScorer(engine),
	}
}

// SetDrafter 设置 LLM 撰写器（可选）。
func (e *Extension) SetDrafter(d *LLMDrafter) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.drafter = d
}

// Name 实现 agentcore.Extension 接口。
func (e *Extension) Name() string { return ExtensionName }

// Init 实现 agentcore.Extension 接口。
func (e *Extension) Init(ctx context.Context, agent *agentcore.Agent) error { return nil }

// Dispose 实现 agentcore.Extension 接口。
func (e *Extension) Dispose() error { return nil }

// Tools 实现 ToolProvider 接口，注册权利要求相关工具。
func (e *Extension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		{
			Name:        "draft_claims",
			Description: "根据技术交底书撰写符合中国专利法要求的权利要求书。输入技术特征、问题和效果，输出结构化的独立权利要求和从属权利要求。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title": map[string]any{
						"type":        "string",
						"description": "发明名称",
					},
					"problems": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "技术问题列表",
					},
					"features": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "object"},
						"description": "技术特征列表，每项包含 id、description、category、importance、prior_status",
					},
					"tech_domain": map[string]any{
						"type":        "string",
						"enum":        []string{"mechanical", "electrical", "chemical", "software", "general"},
						"description": "技术领域（可选，为空时自动识别）",
					},
				},
				"required": []string{"title", "features"},
			},
			Func: e.handleDraftClaims,
		},
	}
}

// handleDraftClaims 处理 draft_claims 工具调用。
func (e *Extension) handleDraftClaims(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Title      string    `json:"title"`
		Problems   []string  `json:"problems"`
		Features   []Feature `json:"features"`
		TechDomain string    `json:"tech_domain"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("draft_claims: 参数解析失败: %w", err)
	}
	if params.Title == "" {
		return nil, fmt.Errorf("draft_claims: 发明名称不能为空")
	}
	if len(params.Features) == 0 {
		return nil, fmt.Errorf("draft_claims: 技术特征不能为空")
	}

	domain := parseTechDomain(params.TechDomain)

	// 使用领域特定的 Builder（每个工具调用创建新的 Builder 以保证领域隔离）
	builder := NewClaimBuilder(domain, "")
	input := DraftInput{
		Title:    params.Title,
		Problems: params.Problems,
		Features: params.Features,
	}

	output, err := builder.Build(input)
	if err != nil {
		return nil, fmt.Errorf("draft_claims: 撰写失败: %w", err)
	}

	// 通过扩展的引擎进行二次验证
	if e.engine != nil {
		allClaims := output.Claims.Claims()
		violations := e.engine.Validate(allClaims, input)
		for _, v := range violations {
			if v.Severity == SeverityWarning || v.Severity == SeverityInfo {
				output.Warnings = append(output.Warnings, "["+v.RuleName+"] "+v.Message)
			}
		}
	}

	return output, nil
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
