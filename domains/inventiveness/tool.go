package inventiveness

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/disclosure"
	"github.com/xujian519/mady/graph"
)

// InventivenessOption 是 NewInventivenessTool 的函数式选项。
type InventivenessOption func(*inventivenessConfig)

type inventivenessConfig struct {
	provider agentcore.Provider
}

// WithProvider 设置 LLM provider。
func WithProvider(p agentcore.Provider) InventivenessOption {
	return func(c *inventivenessConfig) { c.provider = p }
}

// NewInventivenessTool 创建 A22.3 创造性三步法评估工具。
// 工具通过 Pregel 子图运行专利法第 22 条第 3 款的创造性判断，
// 返回结构化的 InventivenessResult。
func NewInventivenessTool(opts ...InventivenessOption) *agentcore.Tool {
	cfg := &inventivenessConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return &agentcore.Tool{
		Name: "evaluate_inventiveness",
		Description: stringsJoin(
			"评估技术方案是否具备专利法第22条第3款规定的创造性。",
			"基于现有技术证据和技术特征，通过三步法判断：",
			"确定最接近现有技术→确定区别特征和实际技术问题→判断技术启示。",
			"支持 A22.3 / 22.3 / 创造性 / 三步法 / inventive step 相关分析。",
		),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prior_art_chunks": map[string]any{
					"type":        "array",
					"description": "现有技术证据片段列表，每个包含 doc_id/title/snippet/score",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"doc_id":  map[string]any{"type": "string"},
							"title":   map[string]any{"type": "string"},
							"snippet": map[string]any{"type": "string"},
							"score":   map[string]any{"type": "number"},
						},
					},
				},
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
							"id":      map[string]any{"type": "string"},
							"problem": map[string]any{"type": "string"},
							"effect":  map[string]any{"type": "string"},
						},
					},
				},
				"novelty_conclusion": map[string]any{
					"type":        "string",
					"description": "新颖性初判结论（作为三步法的辅助参考）",
				},
				"evidence_coverage": map[string]any{
					"type":        "string",
					"description": "证据覆盖状态：full/partial/none。none 时跳过评估",
				},
				"invention_type": map[string]any{
					"type":        "string",
					"description": "发明类型：pioneering/combination/selection/transfer/new_use/element_change（可选）",
				},
				"tech_domain": map[string]any{
					"type":        "string",
					"description": "技术领域：chemistry/computer/tcm（可选）",
				},
			},
		},
		ReadOnly: true,
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return runInventivenessTool(ctx, cfg, args)
		},
	}
}

// runInventivenessTool 是 evaluate_inventiveness 工具的实际执行函数。
func runInventivenessTool(ctx context.Context, cfg *inventivenessConfig, args json.RawMessage) (any, error) {
	if cfg.provider == nil {
		return map[string]any{"error": "provider 未配置，无法运行创造性评估"}, nil
	}

	input := parseInventivenessArgs(args)
	if input == nil {
		return map[string]any{"error": "参数解析失败，请提供 features 和 prior_art_chunks"}, nil
	}

	compiled, err := BuildInventivenessGraph(cfg.provider)
	if err != nil {
		return map[string]any{"error": "构建评估图失败: " + err.Error()}, nil
	}

	state := graph.PregelState{}
	state[StateKeyInput] = input

	state, runErr := compiled.Run(ctx, state)

	if raw, ok := state[StateKeyResult]; ok {
		if result, ok := raw.(*InventivenessResult); ok {
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
		return map[string]any{"error": runErr.Error()}, nil
	}
	return map[string]any{"error": "评估图执行后未找到结果"}, nil
}

// parseInventivenessArgs 从 JSON 参数解析 InventivenessInput。
func parseInventivenessArgs(args json.RawMessage) *InventivenessInput {
	input := &InventivenessInput{}

	var raw struct {
		PriorArtChunks []struct {
			DocID   string  `json:"doc_id"`
			Title   string  `json:"title"`
			Snippet string  `json:"snippet"`
			Score   float64 `json:"score"`
		} `json:"prior_art_chunks"`
		Features []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
			Category    string `json:"category"`
			Function    string `json:"function"`
			Importance  string `json:"importance"`
		} `json:"features"`
		PFETriples []struct {
			ID      string `json:"id"`
			Problem string `json:"problem"`
			Effect  string `json:"effect"`
		} `json:"pfe_triples"`
		NoveltyConclusion string `json:"novelty_conclusion"`
		EvidenceCoverage  string `json:"evidence_coverage"`
		InventionType     string `json:"invention_type"`
		TechDomain        string `json:"tech_domain"`
		ExperimentalData  *struct {
			HasOriginalData   bool   `json:"has_original_data"`
			HasSupplementData bool   `json:"has_supplement_data"`
			DataSummary       string `json:"data_summary,omitempty"`
			ComparisonType    string `json:"comparison_type,omitempty"`
		} `json:"experimental_data,omitempty"`
	}
	if err := json.Unmarshal(args, &raw); err != nil {
		return nil
	}

	if raw.ExperimentalData != nil {
		input.ExperimentalData = &ExperimentalData{
			HasOriginalData:   raw.ExperimentalData.HasOriginalData,
			HasSupplementData: raw.ExperimentalData.HasSupplementData,
			DataSummary:       raw.ExperimentalData.DataSummary,
			ComparisonType:    raw.ExperimentalData.ComparisonType,
		}
	}

	for _, c := range raw.PriorArtChunks {
		input.PriorArtChunks = append(input.PriorArtChunks, EvidenceChunk{
			DocID:   c.DocID,
			Title:   c.Title,
			Snippet: c.Snippet,
			Score:   c.Score,
		})
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
			ID:      t.ID,
			Problem: t.Problem,
			Effect:  t.Effect,
		})
	}
	input.NoveltyConclusion = raw.NoveltyConclusion
	input.EvidenceCoverage = raw.EvidenceCoverage
	input.InventionType = raw.InventionType
	input.TechDomain = raw.TechDomain
	if input.EvidenceCoverage == "" {
		input.EvidenceCoverage = "partial"
	}
	if len(input.Features) > 0 && input.EvidenceCoverage == "partial" {
		input.EvidenceCoverage = "full"
	}

	return input
}

// NewInventivenessToolFromReport 从 disclosure 报告构造评估输入并执行创造性评估。
// 这是一个便捷函数，供需要从 disclosure.AnalysisReport 直接创建输入的场景使用。
func NewInventivenessToolFromReport(provider agentcore.Provider, report *disclosure.AnalysisReport, evidence []disclosure.EvidenceChunk, coverage string) (*InventivenessResult, error) {
	if provider == nil {
		return nil, fmt.Errorf("inventiveness: provider is nil")
	}

	input := &InventivenessInput{EvidenceCoverage: coverage}
	if input.EvidenceCoverage == "" {
		input.EvidenceCoverage = "partial"
	}

	// 1. 转换现有技术证据片段。
	for _, c := range evidence {
		input.PriorArtChunks = append(input.PriorArtChunks, EvidenceChunk{
			DocID:   c.DocID,
			Title:   c.Title,
			Snippet: c.Snippet,
			Score:   c.Score,
		})
	}

	// 2. 转换技术特征和 PFE 三元组。
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
				ID:      t.ID,
				Problem: t.Problem,
				Effect:  t.Effect,
			})
		}

		// 3. 特征非空时覆盖度提升为 full。
		if len(input.Features) > 0 && input.EvidenceCoverage == "partial" {
			input.EvidenceCoverage = "full"
		}
	}

	// 4. 新颖性初判结论。
	if report != nil && report.Novelty != nil {
		input.NoveltyConclusion = report.Novelty.Conclusion
	}

	compiled, err := BuildInventivenessGraph(provider)
	if err != nil {
		return nil, err
	}

	state := graph.PregelState{}
	state[StateKeyInput] = input

	// 使用带超时的 context，防止 LLM API 挂起时 goroutine 永久泄漏。
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	state, runErr := compiled.Run(timeoutCtx, state)

	if raw, ok := state[StateKeyResult]; ok {
		if result, ok := raw.(*InventivenessResult); ok {
			return result, runErr
		}
	}
	return nil, runErr
}

// stringsJoin 用空格拼接字符串。
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
