package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// =============================================================================
// Built-in StageHandler implementations
// =============================================================================

// =============================================================================
// Local types for pipeline stage handler decoupling.
// These mirror subset of retrieval/domain types without creating an import
// cycle (agentcore → retrieval/domain → knowledge → agentcore).
// =============================================================================

// Retriever is the local interface for search stage handlers.
type Retriever interface {
	Search(ctx context.Context, query RetrieverQuery) (*RetrieverResults, error)
	SourceName() string
}

// RetrieverQuery is the local query type for Retriever.Search.
type RetrieverQuery struct {
	Text       string
	Keywords   []string
	MaxResults int
}

// RetrieverResults is the local result type for Retriever.Search.
type RetrieverResults struct {
	Documents  []RetrieverDocument
	TotalCount int
	Source     string
}

// RetrieverDocument is a single search result.
type RetrieverDocument struct {
	ID      string
	Title   string
	Snippet string
	Content string
	URL     string
	Score   float64
}

// =============================================================================
// Built-in StageHandler implementations
// =============================================================================

// searchHandler executes patent/legal prior-art search.
// It reads the following keys from PipelineState:
//   - "query" (string, optional): natural-language query text
//   - "keywords" ([]string, optional): structured keywords
//   - "max_results" (int, optional): max results to return (default: 8)
//   - "_retriever" (Retriever): the retriever to use
//
// Output keys:
//   - "prior_art" ([]RetrieverDocument): retrieved documents
//   - "search_summary" (string): human-readable search summary
type searchHandler struct{}

func (h *searchHandler) Name() string { return "search" }

func (h *searchHandler) Execute(ctx context.Context, state PipelineState, provider Provider) (PipelineState, error) {
	// Extract retriever from state.
	rawRetriever, ok := state["_retriever"]
	if !ok {
		return PipelineState{"_error": "search: no retriever configured (set state[\"_retriever\"])"}, nil
	}
	retriever, ok := rawRetriever.(Retriever)
	if !ok || retriever == nil {
		return PipelineState{"_error": "search: retriever is nil or invalid type"}, nil
	}

	// Build query.
	query := state.GetString("query")
	keywords := extractKeywordsFromState(state)
	maxResults := 8
	if mr, ok := state["max_results"].(int); ok && mr > 0 {
		maxResults = mr
	}

	if query == "" && len(keywords) == 0 {
		return PipelineState{"_error": "search: no query or keywords provided"}, nil
	}

	results, err := retriever.Search(ctx, RetrieverQuery{
		Text:       query,
		Keywords:   keywords,
		MaxResults: maxResults,
	})
	if err != nil {
		return PipelineState{"_error": fmt.Sprintf("search failed: %v", err)}, nil
	}

	out := make(PipelineState)
	out["prior_art"] = results.Documents
	out["search_summary"] = fmt.Sprintf(
		"在 %s 中找到 %d 篇相关文献（共 %d 条匹配）",
		retriever.SourceName(), len(results.Documents), results.TotalCount,
	)
	return out, nil
}

func extractKeywordsFromState(state PipelineState) []string {
	if raw, ok := state["keywords"]; ok {
		switch v := raw.(type) {
		case []string:
			return v
		case []any:
			kw := make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok {
					kw = append(kw, s)
				}
			}
			return kw
		}
	}
	return nil
}

// =============================================================================
// extractHandler — structured information extraction
// =============================================================================

// extractHandler extracts structured information (features, problems, effects)
// from text using an LLM Agent with JSON schema output.
//
// Input keys:
//   - "text" (string): the text to extract from
//   - "extraction_type" (string): "features", "problems", "effects", or "all"
//   - "domain" (string): "patent" or "legal"
//
// Output keys:
//   - "extraction_result" (string): raw JSON output from the LLM
//   - "features" ([]any, optional): parsed feature list (if extraction_type includes features)
//   - "problems" ([]any, optional): parsed problem list
//   - "effects" ([]any, optional): parsed effect list
type extractHandler struct{}

func (h *extractHandler) Name() string { return "extract" }

func (h *extractHandler) Execute(ctx context.Context, state PipelineState, provider Provider) (PipelineState, error) {
	text := state.GetString("text")
	if text == "" {
		return PipelineState{"_error": "extract: no input text provided (set state[\"text\"])"}, nil
	}
	extractType := state.GetString("extraction_type")
	if extractType == "" {
		extractType = "all"
	}
	domainName := state.GetString("domain")
	if domainName == "" {
		domainName = "patent"
	}

	prompt := buildExtractPrompt(extractType, domainName)

	cfg := Config{
		ModelConfig: ModelConfig{
			Name:        "pipeline-extract",
			Model:       "default",
			Provider:    provider,
			Temperature: 0.3,
		},
		SystemPrompt: prompt,
		ExecutionConfig: ExecutionConfig{
			MaxTurns: 1,
		},
	}

	if schema := buildExtractSchema(extractType); schema != nil {
		cfg.ResponseFormat = NewJSONSchemaResponseFormat("extract_"+extractType, schema)
	}

	agent := New(cfg)
	defer agent.Close()

	output, err := agent.Run(ctx, text)
	if err != nil {
		return PipelineState{"_error": fmt.Sprintf("extract failed: %v", err)}, nil
	}

	out := make(PipelineState)
	out["extraction_result"] = output

	// Attempt to parse JSON output for structured fields.
	if parsed := tryParseExtraction(output); parsed != nil {
		for k, v := range parsed {
			out[k] = v
		}
	}

	return out, nil
}

func buildExtractPrompt(extractType, domainName string) string {
	var sb strings.Builder
	sb.WriteString("你是一名资深的")
	if domainName == "legal" {
		sb.WriteString("法律")
	} else {
		sb.WriteString("专利")
	}
	sb.WriteString("专业人员，负责从技术文档中提取结构化信息。\n")
	sb.WriteString("请精确提取指定要素，严格按 JSON Schema 输出。\n\n")
	sb.WriteString("提取类型：")
	sb.WriteString(extractType)
	sb.WriteString("\n")
	switch extractType {
	case "features":
		sb.WriteString("提取所有技术特征（最小技术单元），每个特征包含：ID、描述、分类、功能。\n")
	case "problems":
		sb.WriteString("提取所有要解决的技术问题。\n")
	case "effects":
		sb.WriteString("提取所有技术效果/有益效果。\n")
	default:
		sb.WriteString("提取以下要素：\n1. 技术问题（problems）\n2. 技术特征（features）\n3. 技术效果（effects）\n")
	}
	return sb.String()
}

func buildExtractSchema(extractType string) map[string]any {
	switch extractType {
	case "features":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"features": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":          map[string]any{"type": "string"},
							"description": map[string]any{"type": "string"},
							"category":    map[string]any{"type": "string", "enum": []any{"structure", "method", "parameter", "material"}},
							"function":    map[string]any{"type": "string"},
						},
						"required": []any{"id", "description", "category", "function"},
					},
				},
			},
			"required": []any{"features"},
		}
	case "problems":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"problems": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":   map[string]any{"type": "string"},
							"text": map[string]any{"type": "string"},
						},
						"required": []any{"id", "text"},
					},
				},
			},
			"required": []any{"problems"},
		}
	case "effects":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"effects": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":   map[string]any{"type": "string"},
							"text": map[string]any{"type": "string"},
						},
						"required": []any{"id", "text"},
					},
				},
			},
			"required": []any{"effects"},
		}
	default:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"problems": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":   map[string]any{"type": "string"},
							"text": map[string]any{"type": "string"},
						},
						"required": []any{"id", "text"},
					},
				},
				"features": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":          map[string]any{"type": "string"},
							"description": map[string]any{"type": "string"},
							"category":    map[string]any{"type": "string", "enum": []any{"structure", "method", "parameter", "material"}},
							"function":    map[string]any{"type": "string"},
						},
						"required": []any{"id", "description", "category", "function"},
					},
				},
				"effects": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":   map[string]any{"type": "string"},
							"text": map[string]any{"type": "string"},
						},
						"required": []any{"id", "text"},
					},
				},
			},
			"required": []any{"problems", "features", "effects"},
		}
	}
}

// tryParseExtraction attempts to parse the LLM output as a structured extraction JSON.
func tryParseExtraction(output string) map[string]any {
	jsonStr := extractJSONFromText(output)
	if jsonStr == "" {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil
	}
	return parsed
}

// extractJSONFromText finds the first JSON object in a text string.
func extractJSONFromText(text string) string {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}
	return ""
}

// =============================================================================
// compareHandler — feature-by-feature comparison
// =============================================================================

// compareHandler performs feature-by-feature comparison between a claim and
// prior art, producing a claim chart. Uses LLM Agent with JSON schema.
//
// Input keys:
//   - "claim" (string): the claim/features to compare
//   - "prior_art" ([]RetrieverDocument or string): prior art to compare against
//   - "comparison_scope" (string): "full" or "novelty_only" (default: "full")
//
// Output keys:
//   - "claim_chart" (string): structured comparison result (JSON)
//   - "diff_features" ([]string): features found in claim but not in prior art
type compareHandler struct{}

func (h *compareHandler) Name() string { return "compare" }

func (h *compareHandler) Execute(ctx context.Context, state PipelineState, provider Provider) (PipelineState, error) {
	claim := state.GetString("claim")
	if claim == "" {
		return PipelineState{"_error": "compare: no claim text provided (set state[\"claim\"])"}, nil
	}

	priorArtText := formatPriorArt(state)
	if priorArtText == "" {
		return PipelineState{"_error": "compare: no prior art provided (set state[\"prior_art\"])"}, nil
	}

	scope := state.GetString("comparison_scope")
	if scope == "" {
		scope = "full"
	}

	prompt := buildComparisonPrompt(scope)

	cfg := Config{
		ModelConfig: ModelConfig{
			Name:           "pipeline-compare",
			Model:          "default",
			Provider:       provider,
			Temperature:    0.2,
			ResponseFormat: NewJSONSchemaResponseFormat("comparison", buildCompareSchema()),
		},
		SystemPrompt: prompt,
		ExecutionConfig: ExecutionConfig{
			MaxTurns: 1,
		},
	}

	input := fmt.Sprintf("## 权利要求/技术方案\n\n%s\n\n## 现有技术\n\n%s", claim, priorArtText)

	agent := New(cfg)
	defer agent.Close()

	output, err := agent.Run(ctx, input)
	if err != nil {
		return PipelineState{"_error": fmt.Sprintf("compare failed: %v", err)}, nil
	}

	out := make(PipelineState)
	out["claim_chart"] = output

	// Extract diff_features from JSON if parseable.
	if parsed := tryParseExtraction(output); parsed != nil {
		if diffs, ok := parsed["diff_features"]; ok {
			out["diff_features"] = diffs
		}
	}

	return out, nil
}

func buildComparisonPrompt(scope string) string {
	base := "你是一名专利审查员，负责逐项对比技术特征。\n"
	if scope == "novelty_only" {
		base += "请坚持单独对比原则——以每一篇对比文件为独立单位，与目标方案逐项对比。\n"
		base += "判断是否存在一篇对比文件公开了目标方案的全部技术特征。\n"
	} else {
		base += "请进行全面的特征对比分析，标注每个特征在现有技术中的状态。\n"
	}
	base += "严格按 JSON Schema 输出。\n"
	return base
}

func buildCompareSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"claim_chart": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"feature":          map[string]any{"type": "string"},
						"in_prior_art":     map[string]any{"type": "boolean"},
						"prior_art_source": map[string]any{"type": "string"},
						"notes":            map[string]any{"type": "string"},
					},
					"required": []any{"feature", "in_prior_art", "prior_art_source", "notes"},
				},
			},
			"diff_features": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Features present in claim but absent in prior art",
			},
			"conclusion": map[string]any{"type": "string"},
		},
		"required": []any{"claim_chart", "diff_features", "conclusion"},
	}
}

func formatPriorArt(state PipelineState) string {
	raw, ok := state["prior_art"]
	if !ok {
		return ""
	}

	switch v := raw.(type) {
	case string:
		return v
	case []RetrieverDocument:
		if len(v) == 0 {
			return ""
		}
		var sb strings.Builder
		for i, doc := range v {
			fmt.Fprintf(&sb, "[%d] %s\n", i+1, doc.Title)
			sb.WriteString("    ")
			sb.WriteString(doc.Snippet)
			sb.WriteString("\n\n")
		}
		return sb.String()
	default:
		return ""
	}
}

// =============================================================================
// reasoningHandler — general-purpose LLM reasoning
// =============================================================================

// reasoningHandler performs general-purpose LLM reasoning for a pipeline stage.
// Unlike compare/extract, this handler does not enforce a JSON schema; it
// returns free-text output from the LLM.
//
// Input keys:
//   - "reasoning_prompt" (string): the domain-specific instruction (overrides built-in)
//   - "reasoning_input" (string): the text to reason about
//
// Output keys:
//   - "reasoning_output" (string): free-text LLM output
//   - "conclusion" (string): same as reasoning_output (alias for ergonomics)
type reasoningHandler struct{}

func (h *reasoningHandler) Name() string { return "reasoning" }

func (h *reasoningHandler) Execute(ctx context.Context, state PipelineState, provider Provider) (PipelineState, error) {
	input := state.GetString("reasoning_input")
	if input == "" {
		// Fallback: build input from all non-metadata state keys.
		input = formatStateForReasoning(state)
	}
	if input == "" {
		return PipelineState{"_error": "reasoning: no input provided"}, nil
	}

	prompt := state.GetString("reasoning_prompt")
	if prompt == "" {
		prompt = "你是一名资深的专利分析师。请根据以下信息进行分析和推理。"
	}

	cfg := Config{
		ModelConfig: ModelConfig{
			Name:        "pipeline-reasoning",
			Model:       "default",
			Provider:    provider,
			Temperature: 0.3,
		},
		SystemPrompt: prompt,
		ExecutionConfig: ExecutionConfig{
			MaxTurns: 1,
		},
	}

	agent := New(cfg)
	defer agent.Close()

	output, err := agent.Run(ctx, input)
	if err != nil {
		return PipelineState{"_error": fmt.Sprintf("reasoning failed: %v", err)}, nil
	}

	out := make(PipelineState)
	out["reasoning_output"] = output
	out["conclusion"] = output
	return out, nil
}

// formatStateForReasoning concatenates all meaningful state keys into a text block.
func formatStateForReasoning(state PipelineState) string {
	var sb strings.Builder
	skipKeys := map[string]bool{
		"_retriever": true, "_warnings": true, "_executed_stages": true, "_interrupted_at": true,
		"plugin_name": true, "plugin_domain": true,
	}

	for k, v := range state {
		if skipKeys[k] {
			continue
		}
		switch val := v.(type) {
		case string:
			if val != "" {
				fmt.Fprintf(&sb, "【%s】\n%s\n\n", k, val)
			}
		case []RetrieverDocument:
			if len(val) > 0 {
				fmt.Fprintf(&sb, "【%s】\n", k)
				for i, doc := range val {
					fmt.Fprintf(&sb, "[%d] %s: %s\n", i+1, doc.Title, doc.Snippet)
				}
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}

// =============================================================================
// approvalGateHandler — human-in-the-loop gate
// =============================================================================

// approvalGateHandler pauses pipeline execution for human review.
// It mirrors the disclosure pipeline's review_gate node pattern.
//
// Input keys:
//   - "review_context" (string): context to present to the reviewer
//   - "guardrail_level" (string): "light", "standard", or "strict"
//
// This handler always returns an InterruptStageError. The pipeline caller
// must handle this error and resume after human review.
type approvalGateHandler struct{}

func (h *approvalGateHandler) Name() string { return "approval-gate" }

func (h *approvalGateHandler) Execute(ctx context.Context, state PipelineState, provider Provider) (PipelineState, error) {
	reviewCtx := state.GetString("review_context")
	if reviewCtx == "" {
		reviewCtx = fmt.Sprintf("管线「%s」的分析结果需要人工审阅确认", state.GetString("plugin_name"))
	}

	data := map[string]any{
		"gate":  "pipeline_approval",
		"stage": "approval-gate",
	}
	if reviewCtx != "" {
		data["review_context"] = reviewCtx
	}
	if domain := state.GetString("plugin_domain"); domain != "" {
		data["domain"] = domain
	}

	return nil, &InterruptStageError{
		StageID: "approval-gate",
		Message: reviewCtx,
		Data:    data,
	}
}

// =============================================================================
// Registration
// =============================================================================

var registerBuiltinHandlers sync.Once

// RegisterBuiltinStageHandlers registers all built-in stage handlers.
// Safe to call multiple times.
func RegisterBuiltinStageHandlers() {
	registerBuiltinHandlers.Do(func() {
		RegisterStageHandler(&searchHandler{})
		RegisterStageHandler(&extractHandler{})
		RegisterStageHandler(&compareHandler{})
		RegisterStageHandler(&reasoningHandler{})
		RegisterStageHandler(&approvalGateHandler{})
	})
}
