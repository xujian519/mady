// Package inventiveness 提供完全独立的创造性分析 Pregel 子图。
//
// 子图不依赖 disclosure 管线，只通过 input state 获取数据。它实现三步法评估：
//
//	Step 1: 确定最接近的现有技术
//	Step 2: 确定区别特征和发明实际解决的技术问题
//	Step 3: 判断现有技术整体上是否存在技术启示
//
// 使用方式（通过 EventBus 接力）：
//
//	disclosure 管线完成后发射 DisclosureCompletedEvent →
//	InventivenessTrigger 接收事件 → 填充 PregelState → 运行子图 →
//	结果写回 session store / emit InventivenessCompletedEvent
package inventiveness

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// Input/Output 类型（值类型，不依赖 disclosure 包）
// =============================================================================

// EvidenceChunk 是检索到的现有技术证据片段（镜像 disclosure.EvidenceChunk 结构）。
type EvidenceChunk struct {
	DocID   string  `json:"doc_id"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// TechFeature 是技术特征（镜像 disclosure.TechFeature 结构）。
type TechFeature struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Function    string `json:"function"`
	Importance  string `json:"importance"`
}

// PFETriple 是问题-特征-效果三元组（镜像 disclosure.PFETriple 结构）。
type PFETriple struct {
	ID      string `json:"id"`
	Problem string `json:"problem"`
	Effect  string `json:"effect"`
}

// InventivenessInput 是创造性分析子图的完整输入。
type InventivenessInput struct {
	PriorArtChunks    []EvidenceChunk `json:"prior_art_chunks"`
	Features          []TechFeature   `json:"features"`
	PFETriples        []PFETriple     `json:"pfe_triples"`
	NoveltyConclusion string          `json:"novelty_conclusion"`
	EvidenceCoverage  string          `json:"evidence_coverage"` // "full" / "partial" / "none"
}

// InventivenessResult 是创造性分析子图的完整输出。
type InventivenessResult struct {
	Assessed   bool            `json:"assessed"`
	Skipped    bool            `json:"skipped,omitempty"`
	SkipReason string          `json:"skip_reason,omitempty"`
	ThreeStep  ThreeStepResult `json:"three_step_analysis"`
	Conclusion string          `json:"conclusion"`
	Confidence string          `json:"confidence"`
}

// ThreeStepResult 封装三步法的每一步产出。
type ThreeStepResult struct {
	ClosestPriorArt        string   `json:"closest_prior_art"`
	DistinguishingFeatures []string `json:"distinguishing_features"`
	ActualTechProblem      string   `json:"actual_tech_problem"`
	TechnicalSuggestion    bool     `json:"technical_suggestion"`
	SuggestionRationale    string   `json:"suggestion_rationale"`
}

// =============================================================================
// Pregel 图构建
// =============================================================================

// BuildInventivenessGraph 构建创造性分析的独立 Pregel 子图。
//
// 图拓扑:
//
//	load_input → step1_closest_prior_art → step2_distinguishing_features →
//	step3_technical_suggestion → generate_conclusion → __end__
//
// 每步均为单 Agent LLM 节点，输出结构化 JSON。
func BuildInventivenessGraph(provider agentcore.Provider) (*graph.CompiledPregelGraph, error) {
	pg := graph.NewPregelGraph()

	nodes := map[string]graph.PregelNode{
		"load_input":                    loadInputNode(),
		"step1_closest_prior_art":       step1ClosestPriorArtNode(provider),
		"step2_distinguishing_features": step2DistinguishingFeaturesNode(provider),
		"step3_technical_suggestion":    step3TechnicalSuggestionNode(provider),
		"generate_conclusion":           generateConclusionNode(provider),
	}

	for name, node := range nodes {
		if err := pg.AddNode(name, node); err != nil {
			return nil, fmt.Errorf("inventiveness: add node %q: %w", name, err)
		}
	}

	edges := [][2]string{
		{"load_input", "step1_closest_prior_art"},
		{"step1_closest_prior_art", "step2_distinguishing_features"},
		{"step2_distinguishing_features", "step3_technical_suggestion"},
		{"step3_technical_suggestion", "generate_conclusion"},
		{"generate_conclusion", graph.PregelEnd},
	}
	for _, e := range edges {
		if err := pg.AddEdge(e[0], e[1]); err != nil {
			return nil, fmt.Errorf("inventiveness: add edge %q→%q: %w", e[0], e[1], err)
		}
	}

	return pg.Compile("load_input", 100)
}

// =============================================================================
// State keys
// =============================================================================

const (
	stateKeyInput  = "inventiveness_input"
	stateKeyResult = "inventiveness_result"
	stateKeyStep1  = "step1_closest_prior_art"
	stateKeyStep2  = "step2_distinguishing_features"
	stateKeyStep3  = "step3_technical_suggestion"
)

// =============================================================================
// 节点实现
// =============================================================================

// loadInputNode 从 PregelState 读取 InventivenessInput。
// 当 EvidenceCoverage == "none" 时跳过，设置 Skipped=true。
func loadInputNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		raw, ok := state[stateKeyInput]
		if !ok {
			state[stateKeyResult] = &InventivenessResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "未提供输入数据（inventiveness_input state key 为空）",
			}
			return state, nil
		}

		input, ok := raw.(*InventivenessInput)
		if !ok || input == nil {
			state[stateKeyResult] = &InventivenessResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "输入数据格式无效",
			}
			return state, nil
		}

		if input.EvidenceCoverage == "none" {
			state[stateKeyResult] = &InventivenessResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "无检索证据，无法进行三步法创造性评估",
			}
			return state, nil
		}

		// Store validated input for downstream nodes.
		state[stateKeyInput] = input
		return state, nil
	}
}

// step1ClosestPriorArtNode 三步法第 1 步：确定最接近的现有技术。
func step1ClosestPriorArtNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		input := extractInput(state)
		if input == nil {
			return state, nil // skipped already
		}

		prompt := "你是一名资深专利审查员。请执行三步法创造性评估的第 1 步：\n\n"
		prompt += "从以下现有技术证据中，确定与目标技术方案最接近的一篇对比文件。\n"
		prompt += "最接近的现有技术是指与目标方案技术领域相同、要解决的技术问题最接近、"
		prompt += "或技术特征最多的现有技术文献。\n\n"
		prompt += "请列出选定文献的标题和理由。"

		inputText := buildInputText(input)
		agent := newInventivenessAgent(provider, "inventiveness-step1", prompt)
		defer agent.Close()

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("step1: %w", err)
		}

		state[stateKeyStep1] = output
		return state, nil
	}
}

// step2DistinguishingFeaturesNode 三步法第 2 步：确定区别特征和实际解决的技术问题。
func step2DistinguishingFeaturesNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step1Output, _ := state[stateKeyStep1].(string)
		input := extractInput(state)

		prompt := "你是一名资深专利审查员。请执行三步法创造性评估的第 2 步：\n\n"
		prompt += "基于第 1 步确定的最接近现有技术，进行以下分析：\n"
		prompt += "1. 逐项列出目标方案相对于最接近现有技术的区别技术特征\n"
		prompt += "2. 基于区别技术特征，重新确定发明实际解决的技术问题\n"
		prompt += "   （注意：不是「原要解决的技术问题」，而是区别特征客观上实际解决的问题）\n\n"
		prompt += "请输出 JSON 格式，包含：\n"
		prompt += "- distinguishing_features: 区别特征列表\n"
		prompt += "- actual_tech_problem: 实际解决的技术问题"

		agent := newInventivenessAgent(provider, "inventiveness-step2", prompt)
		defer agent.Close()

		inputText := buildInputText(input)
		if step1Output != "" {
			inputText = "第 1 步结论：\n" + step1Output + "\n\n" + inputText
		}

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("step2: %w", err)
		}

		state[stateKeyStep2] = output
		return state, nil
	}
}

// step3TechnicalSuggestionNode 三步法第 3 步：判断现有技术整体上是否存在技术启示。
// 这是多假设推理的典型场景——需要判断多篇对比文件结合是否存在技术启示。
func step3TechnicalSuggestionNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step2Output, _ := state[stateKeyStep2].(string)

		prompt := "你是一名资深专利审查员。请执行三步法创造性评估的第 3 步：\n\n"
		prompt += "基于第 2 步确定的区别特征和实际解决的技术问题，判断现有技术整体上\n"
		prompt += "（包括最接近现有技术和其他对比文件）是否给出了将区别特征应用到\n"
		prompt += "最接近现有技术以解决实际技术问题的技术启示。\n\n"
		prompt += "评估要点：\n"
		prompt += "1. 其他对比文件中是否公开了所述区别特征\n"
		prompt += "2. 该特征在其他对比文件中所起的作用是否与其在本发明中的作用相同\n"
		prompt += "3. 本领域技术人员是否有动机将该特征结合到最接近的现有技术中\n\n"
		prompt += "请输出 JSON 格式：\n"
		prompt += "- technical_suggestion: true/false（是否存在技术启示）\n"
		prompt += "- rationale: 详细推理过程\n"
		prompt += "- confidence: high/medium/low"

		agent := newInventivenessAgent(provider, "inventiveness-step3", prompt)
		defer agent.Close()

		inputText := "第 2 步结论：\n" + step2Output

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("step3: %w", err)
		}

		state[stateKeyStep3] = output
		return state, nil
	}
}

// generateConclusionNode 汇总三步法各步骤，生成最终创造性评估结论。
func generateConclusionNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step1, _ := state[stateKeyStep1].(string)
		step2, _ := state[stateKeyStep2].(string)
		step3, _ := state[stateKeyStep3].(string)

		prompt := "你是一名资深专利审查员。请基于三步法各步骤的产出，生成最终的创造性评估结论。\n\n"
		prompt += "结论应包含：\n"
		prompt += "1. 整体判断：该技术方案是否具备创造性\n"
		prompt += "2. 置信度：high/medium/low\n"
		prompt += "3. 辅助考虑因素（如有）：商业成功、预料不到的技术效果、长期需求等\n\n"
		prompt += "请输出 JSON 格式：\n"
		prompt += "- conclusion: 整体结论\n"
		prompt += "- confidence: high/medium/low\n"
		prompt += "- auxiliary_factors: 辅助考虑因素（可选）"

		agent := newInventivenessAgent(provider, "inventiveness-conclusion", prompt)
		defer agent.Close()

		inputText := fmt.Sprintf("第 1 步（最接近现有技术）:\n%s\n\n第 2 步（区别特征与技术问题）:\n%s\n\n第 3 步（技术启示判断）:\n%s",
			step1, step2, step3)

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("conclusion: %w", err)
		}

		// Build structured result from raw outputs.
		result := &InventivenessResult{
			Assessed:   true,
			Conclusion: output,
			Confidence: extractConfidence(output),
		}

		state[stateKeyResult] = result
		return state, nil
	}
}

// =============================================================================
// 辅助函数
// =============================================================================

// newInventivenessAgent 创建统一配置的 Agent 节点。
// 所有三步法 LLM 节点共享 Temperature=0.2 和 MaxTurns=1，仅 name/prompt 不同。
func newInventivenessAgent(provider agentcore.Provider, name, prompt string) *agentcore.Agent {
	return agentcore.New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:        name,
			Model:       "default",
			Provider:    provider,
			Temperature: 0.2,
		},
		SystemPrompt: prompt,
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 1,
		},
	})
}

// extractInput 从 state 中安全读取已验证的 InventivenessInput。
// loadInputNode 已确保类型正确且 EvidenceCoverage != "none"，此处仅做防御性断言。
func extractInput(state graph.PregelState) *InventivenessInput {
	raw, ok := state[stateKeyInput]
	if !ok {
		return nil
	}
	input, ok := raw.(*InventivenessInput)
	if !ok {
		return nil
	}
	return input
}

func stateHasSkip(state graph.PregelState) bool {
	raw, ok := state[stateKeyResult]
	if !ok {
		return false
	}
	r, ok := raw.(*InventivenessResult)
	return ok && r != nil && r.Skipped
}

func buildInputText(input *InventivenessInput) string {
	var sb strings.Builder
	if input == nil {
		return ""
	}

	if len(input.Features) > 0 {
		sb.WriteString("## 技术特征\n")
		for _, f := range input.Features {
			fmt.Fprintf(&sb, "- [%s] %s (%s)\n", f.Category, f.Description, f.Importance)
		}
		sb.WriteString("\n")
	}

	if len(input.PriorArtChunks) > 0 {
		sb.WriteString("## 现有技术证据\n")
		for i, c := range input.PriorArtChunks {
			fmt.Fprintf(&sb, "[%d] %s\n    %s\n\n", i+1, c.Title, c.Snippet)
		}
	}

	if input.NoveltyConclusion != "" {
		fmt.Fprintf(&sb, "## 新颖性初判结论\n%s\n\n", input.NoveltyConclusion)
	}

	return sb.String()
}

func extractConfidence(output string) string {
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return "medium"
	}
	var parsed struct {
		Confidence string `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return "medium"
	}
	switch parsed.Confidence {
	case "high", "medium", "low":
		return parsed.Confidence
	default:
		return "medium"
	}
}

func extractJSON(text string) string {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}
	return ""
}
