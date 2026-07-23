package enablement

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/disclosure"
	"github.com/xujian519/mady/graph"
)

// EnablementOption 是 NewEnablementTool 的函数式选项。
type EnablementOption func(*enablementConfig)

type enablementConfig struct {
	provider           agentcore.Provider
	knowledgeRetriever KnowledgeRetriever
}

// WithProvider 设置 LLM provider。
func WithProvider(p agentcore.Provider) EnablementOption {
	return func(c *enablementConfig) { c.provider = p }
}

// WithKnowledgeRetriever 设置知识检索器，用于自动获取审查指南参考和类案信息。
// 注入后，每次评估前会自动检索相关知识并填充到 GuidelineRefs 和 SimilarCases 字段。
// 不注入（nil）时行为不变，降级为仅依赖 LLM 内部知识。
func WithKnowledgeRetriever(r KnowledgeRetriever) EnablementOption {
	return func(c *enablementConfig) { c.knowledgeRetriever = r }
}

// NewEnablementTool 创建 26.3 充分公开评估工具。
// 工具通过 Pregel 子图运行专利法第 26 条第 3 款的充分公开判断，
// 返回结构化的 EnablementResult。
func NewEnablementTool(opts ...EnablementOption) *agentcore.Tool {
	cfg := &enablementConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return &agentcore.Tool{
		Name: "evaluate_enablement",
		Description: stringsJoin(
			"评估说明书是否满足专利法第26条第3款（充分公开/可实现性）的要求。",
			"基于技术特征、PFE因果链（问题→特征→效果）和说明书章节内容，",
			"通过三步法判断：完整性→清楚性→能够实现性。",
			"支持 A26.3 / 26.3 / 充分公开 / 公开不充分 相关分析。",
		),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"features": map[string]any{
					"type":        "array",
					"description": "技术特征列表，每个特征包含 id/description/category/function/importance",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":          map[string]any{"type": "string"},
							"description": map[string]any{"type": "string"},
							"category":    map[string]any{"type": "string"},
							"function":    map[string]any{"type": "string"},
							"importance":  map[string]any{"type": "string"},
						},
					},
				},
				"pfe_triples": map[string]any{
					"type":        "array",
					"description": "PFE三元组（问题→特征→效果因果链）",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":          map[string]any{"type": "string"},
							"problem":     map[string]any{"type": "string"},
							"feature_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
							"effect":      map[string]any{"type": "string"},
						},
					},
				},
				"problems": map[string]any{
					"type":        "array",
					"description": "技术问题列表",
					"items":       map[string]any{"type": "string"},
				},
				"effects": map[string]any{
					"type":        "array",
					"description": "技术效果列表",
					"items":       map[string]any{"type": "string"},
				},
				"doc_sections": map[string]any{
					"type":        "object",
					"description": "说明书章节内容（key为章节名如 technical_field）",
				},
				"has_drawings": map[string]any{
					"type":        "boolean",
					"description": "是否有附图",
				},
			},
		},
		ReadOnly: true,
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return runEnablementTool(ctx, cfg, args)
		},
	}
}

// runEnablementTool 是 evaluate_enablement 工具的实际执行函数。
func runEnablementTool(ctx context.Context, cfg *enablementConfig, args json.RawMessage) (any, error) {
	if cfg.provider == nil {
		return map[string]string{"error": "provider 未配置，无法运行 26.3 评估"}, nil
	}

	input := parseEnablementArgs(args)
	if input == nil {
		return map[string]string{"error": "参数解析失败，请提供 features 和 pfe_triples"}, nil
	}

	// 知识检索增强：填充 GuidelineRefs 和 SimilarCases
	EnrichInput(ctx, input, cfg.knowledgeRetriever)

	compiled, err := BuildEnablementGraph(cfg.provider)
	if err != nil {
		return map[string]string{"error": "构建评估图失败: " + err.Error()}, nil
	}

	state := graph.PregelState{}
	state[stateKeyInput] = input

	state, runErr := compiled.Run(ctx, state)

	if raw, ok := state[stateKeyResult]; ok {
		if result, ok := raw.(*EnablementResult); ok {
			if runErr != nil {
				return map[string]any{
					"result": result,
					"error":  runErr.Error(),
				}, nil
			}
			return result, nil
		}
	}

	if runErr != nil {
		return map[string]string{"error": runErr.Error()}, nil
	}
	return map[string]string{"error": "评估图执行后未找到结果"}, nil
}

// parseEnablementArgs 从 JSON 参数解析 EnablementInput。
func parseEnablementArgs(args json.RawMessage) *EnablementInput {
	input := &EnablementInput{}

	var raw struct {
		Features []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
			Category    string `json:"category"`
			Function    string `json:"function"`
			Importance  string `json:"importance"`
		} `json:"features"`
		PFETriples []struct {
			ID         string   `json:"id"`
			Problem    string   `json:"problem"`
			FeatureIDs []string `json:"feature_ids"`
			Effect     string   `json:"effect"`
		} `json:"pfe_triples"`
		Problems    []string          `json:"problems"`
		Effects     []string          `json:"effects"`
		DocSections map[string]string `json:"doc_sections"`
		HasDrawings bool              `json:"has_drawings"`
	}
	if err := json.Unmarshal(args, &raw); err != nil {
		return nil
	}

	for _, f := range raw.Features {
		input.Features = append(input.Features, TechFeature{
			ID:          f.ID,
			Description: f.Description,
			Category:    f.Category,
			Function:    f.Function,
			Importance:  f.Importance,
		})
	}
	for _, t := range raw.PFETriples {
		input.PFETriples = append(input.PFETriples, PFETriple{
			ID:         t.ID,
			Problem:    t.Problem,
			FeatureIDs: t.FeatureIDs,
			Effect:     t.Effect,
		})
	}
	input.Problems = raw.Problems
	input.Effects = raw.Effects
	input.DocSections = raw.DocSections
	input.HasDrawings = raw.HasDrawings
	input.EvidenceCoverage = "partial"
	if len(input.Features) > 0 {
		input.EvidenceCoverage = "full"
	}

	return input
}

// EnrichInput 在评估图执行前用知识检索结果增强 EnablementInput。
// 检索结果填充到 GuidelineRefs 和 SimilarCases 字段，供后续 LLM 节点使用。
// retriever 为 nil 时跳过检索，行为不变。
func EnrichInput(ctx context.Context, input *EnablementInput, retriever KnowledgeRetriever) {
	if retriever == nil || input == nil {
		return
	}

	// 检测技术领域作为检索上下文
	domain := DetectDomain(input)

	// 检索审查指南条款
	guidelines, err := retriever.SearchGuidelines(ctx, domain, input.Problems, input.Features)
	if err != nil {
		slog.Warn("enablement: 检索审查指南条款失败",
			"error", err,
			"domain", domain,
		)
	} else if len(guidelines) > 0 {
		input.GuidelineRefs = append(input.GuidelineRefs, guidelines...)
		slog.Debug("enablement: 检索到审查指南条款",
			"count", len(guidelines),
			"domain", domain,
		)
	}

	// 检索类案
	cases, err := retriever.SearchSimilarCases(ctx, domain, input.Features, input.Problems)
	if err != nil {
		slog.Warn("enablement: 检索类案失败",
			"error", err,
			"domain", domain,
		)
	} else if len(cases) > 0 {
		input.SimilarCases = append(input.SimilarCases, cases...)
		slog.Debug("enablement: 检索到类案",
			"count", len(cases),
			"domain", domain,
		)
	}
}

func stringsJoin(s ...string) string {
	var b strings.Builder
	for i, str := range s {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(str)
	}
	return b.String()
}

// NewEnablementToolFromReport 从 disclosure 报告构造评估输入并执行评估。
// 这是一个便捷函数，供需要从 disclosure.AnalysisReport 直接创建输入的场景使用。
func NewEnablementToolFromReport(provider agentcore.Provider, report *disclosure.AnalysisReport) (*EnablementResult, error) {
	if provider == nil {
		return nil, nil
	}

	input := &EnablementInput{EvidenceCoverage: "partial"}
	if report != nil && report.Extraction != nil {
		for _, f := range report.Extraction.Features {
			input.Features = append(input.Features, TechFeature{
				ID:          f.ID,
				Description: f.Description,
				Category:    string(f.Category),
				Function:    f.Function,
				Importance:  f.Importance,
			})
		}
		for _, t := range report.Extraction.PFETriples {
			input.PFETriples = append(input.PFETriples, PFETriple{
				ID:         t.ID,
				Problem:    t.Problem,
				FeatureIDs: t.FeatureIDs,
				Effect:     t.Effect,
			})
		}
		input.Problems = report.Extraction.Problems
		input.Effects = report.Extraction.Effects
		if len(input.Features) > 0 {
			input.EvidenceCoverage = "full"
		}
	}
	// Copy document sections and drawing flag for step1 completeness check.
	if report != nil && report.Document != nil {
		input.HasDrawings = report.Document.HasDrawings
		input.DocSections = make(map[string]string)
		for section, content := range report.Document.Sections {
			input.DocSections[string(section)] = content
		}
	}

	// Note: NewEnablementToolFromReport 暂不支持知识检索增强。
	// 如需知识赋能，请使用 NewEnablementTool + WithKnowledgeRetriever 路径。

	compiled, err := BuildEnablementGraph(provider)
	if err != nil {
		return nil, err
	}

	state := graph.PregelState{}
	state[stateKeyInput] = input

	// 使用带超时的 context，防止 LLM API 挂起时 goroutine 永久泄漏。
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	state, runErr := compiled.Run(timeoutCtx, state)

	if raw, ok := state[stateKeyResult]; ok {
		if result, ok := raw.(*EnablementResult); ok {
			return result, runErr
		}
	}
	return nil, runErr
}
