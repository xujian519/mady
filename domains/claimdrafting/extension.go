package claimdrafting

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
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
//
// 引擎层使用 8 节点 Pregel 图编排撰写流程（load_input → classify_features
// → draft_primary → draft_parallel → draft_dependents → validate → score
// → finalize），替代传统的串行 Builder 调用。
type Extension struct {
	mu      sync.RWMutex
	engine  *RuleEngine
	scorer  *ClaimScorer
	drafter *LLMDrafter                // 可选，通过 SetDrafter 注入
	graph   *graph.CompiledPregelGraph // 预编译的 Pregel 图
}

// NewExtension 创建一个 claimdrafting 扩展实例。
// engine 为共享的规则引擎实例（所有工具调用复用同一引擎）。
// 创建时预编译 8 节点 Pregel 图，后续所有 draft_claims 调用通过图执行完成。
func NewExtension(engine *RuleEngine) *Extension {
	if engine == nil {
		panic("claimdrafting: NewExtension 的 engine 参数不能为 nil")
	}
	scorer := NewClaimScorer(engine)
	g, err := BuildClaimGraph(engine, scorer)
	if err != nil {
		panic("claimdrafting: 构建 Pregel 图失败: " + err.Error())
	}
	return &Extension{
		engine: engine,
		scorer: scorer,
		graph:  g,
	}
}

// SetDrafter 设置 LLM 撰写器（可选）。
func (e *Extension) SetDrafter(d *LLMDrafter) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.drafter = d
}

// Drafter 返回已注入的 LLM 撰写器（读安全，可能为 nil）。
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
	input := DraftInput{
		Title:      params.Title,
		Problems:   params.Problems,
		Features:   params.Features,
		TechDomain: domain,
	}

	// 优先使用 drafter（LLM 增强），否则使用 Pregel 图执行。
	// drafter 内部在 provider 不可用时也会自行降级到 Builder。
	var output *DraftOutput
	if d := e.Drafter(); d != nil {
		var err error
		output, err = d.DraftFromScratch(input)

		// drafter 路径保留二次验证（LLM 输出可能不合规）
		if err == nil && e.engine != nil && output.Claims != nil {
			allClaims := output.Claims.Claims()
			violations := e.engine.Validate(allClaims, input)
			for _, v := range violations {
				if v.Severity == SeverityWarning || v.Severity == SeverityInfo {
					output.Warnings = append(output.Warnings, "["+v.RuleName+"] "+v.Message)
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("draft_claims: 撰写失败: %w", err)
		}
	} else {
		state := graph.PregelState{StateKeyInput: &input}
		result, gErr := e.graph.Run(ctx, state)
		if gErr != nil {
			return nil, fmt.Errorf("draft_claims: 图执行失败: %w", gErr)
		}
		out, ok := result[StateKeyOutput].(*DraftOutput)
		if !ok || out == nil {
			return nil, fmt.Errorf("draft_claims: 图执行未产生输出")
		}
		output = out
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
