package enablement

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// 节点实现
// =============================================================================

// loadInputNode 从 PregelState 读取 EnablementInput 并验证有效性。
// 当 PFE 三元组为空或特征数为 0 时设置 Skipped=true，后续节点全部跳过。
func loadInputNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		raw, ok := state[stateKeyInput]
		if !ok {
			state[stateKeyResult] = &EnablementResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "未提供输入数据（enablement_input state key 为空）",
			}
			return state, nil
		}

		input, ok := raw.(*EnablementInput)
		if !ok || input == nil {
			state[stateKeyResult] = &EnablementResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "输入数据格式无效",
			}
			return state, nil
		}

		// 无特征数据时无法评估充分公开。
		if len(input.Features) == 0 && len(input.PFETriples) == 0 {
			state[stateKeyResult] = &EnablementResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "未提取到技术特征或 PFE 三元组，无法评估说明书充分公开",
			}
			return state, nil
		}

		// 存储已验证的输入，供下游节点使用。
		state[stateKeyInput] = input
		return state, nil
	}
}

// step1CompletenessNode 三步法第 1 步：检查说明书结构完整性。
// 对照 5 项必要章节（技术领域/背景技术/发明内容/附图说明/具体实施方式），
// 识别缺失章节并给出完整性评分。
func step1CompletenessNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}

		prompt := strings.Join([]string{
			"你是一名资深专利审查员。请执行专利法第26条第3款（充分公开）评估的第 1 步：",
			"**检查说明书结构完整性**。",
			"",
			"根据审查指南第二部分第二章第 2.1 节，说明书应当包含以下 5 项必要内容：",
			"1. 技术领域",
			"2. 背景技术",
			"3. 发明内容（要解决的技术问题、技术方案、有益效果）",
			"4. 附图说明（如有附图）",
			"5. 具体实施方式（至少一个实施例）",
			"",
			"请基于提供的说明书章节内容，判断每一项是否存在及其质量。",
			"如果某项章节缺失或内容过于简略（如仅有一句话），请标记为缺失。",
			"",
			"请输出 JSON 格式：",
			`{"has_tech_field": bool, "has_background": bool, "has_content": bool,`,
			`"has_drawings": bool, "has_embodiment": bool,`,
			`"missing_sections": ["缺失章节名称"], "score": 0.0-1.0,`,
			`"notes": "详细说明"}`,
		}, "\n")

		agent := newEnablementAgent(provider, "enablement-step1", prompt, completenessSchema())
		defer agent.Close()

		output, err := agent.Run(ctx, buildCompletenessInput(input))
		if err != nil {
			return state, fmt.Errorf("step1_completeness: %w", err)
		}

		state[stateKeyStep1] = output
		return state, nil
	}
}

// step2ClarityNode 三步法第 2 步：检查说明书清楚性。
// 判断技术术语是否含义明确、无歧义；PFE 三元组中是否存在孤立的特征或效果。
func step2ClarityNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step1Output, _ := state[stateKeyStep1].(string)
		input := extractInput(state)

		prompt := strings.Join([]string{
			"你是一名资深专利审查员。请执行专利法第26条第3款（充分公开）评估的第 2 步：",
			"**检查说明书清楚性**。",
			"",
			"根据专利法第26条第3款，「清楚」是指：",
			"- 技术术语含义明确，无歧义",
			"- 技术方案描述逻辑自洽",
			"- 问题-特征-效果（PFE）因果链完整，不存在孤立的特征或效果",
			"",
			"请基于提供的 PFE 三元组（问题→特征→效果因果链）和说明书内容，判断：",
			"1. 是否存在含义不明确的术语或表述",
			"2. 是否存在没有对应技术效果的特征（孤立特征）",
			"3. 是否存在没有对应技术特征的效果（孤立效果）",
			"",
			"请输出 JSON 格式：",
			`{"is_clear": bool, "ambiguous_terms": ["歧义术语"],`,
			`"orphan_features": ["孤立特征描述"], "orphan_effects": ["孤立效果描述"],`,
			`"notes": "详细说明"}`,
		}, "\n")

		agent := newEnablementAgent(provider, "enablement-step2", prompt, claritySchema())
		defer agent.Close()

		inputText := buildPFEInput(input)
		if step1Output != "" {
			inputText = "第 1 步（完整性检查）结论：\n" + step1Output + "\n\n" + inputText
		}

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("step2_clarity: %w", err)
		}

		state[stateKeyStep2] = output
		return state, nil
	}
}

// step3EnablementNode 三步法第 3 步（核心）：检查能够实现性。
// 判断本领域技术人员根据说明书记载能否无需创造性劳动即可实施，
// 逐一检测四种公开不充分的经典情形。
func step3EnablementNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step2Output, _ := state[stateKeyStep2].(string)

		prompt := strings.Join([]string{
			"你是一名资深专利审查员。请执行专利法第26条第3款（充分公开）评估的第 3 步：",
			"**检查能够实现性**（这是 26.3 的核心标准）。",
			"",
			"根据专利法第26条第3款和审查指南第二部分第二章第 2.1 节，",
			"「能够实现」是指所属技术领域的技术人员根据说明书记载，",
			"无需创造性劳动即可实施该发明。",
			"",
			"请逐一检查以下四种公开不充分的经典情形：",
			"",
			"1. **缺少关键技术手段**：说明书是否给出了实施发明所需的关键技术手段？",
			"   （例如：未公开核心算法、关键参数、特定材料）",
			"2. **技术手段含糊不清**：技术描述是否具体、可操作？",
			"   （例如：使用「合适的」「根据需要调节」等开放性表述而无具体范围）",
			"3. **仅给出任务/设想**：是否仅提出了要解决的问题或希望达到的目标，",
			"   而未给出实现的具体技术方案？",
			"4. **实验数据不足**：如果技术效果需要实验数据支撑，",
			"   说明书是否提供了充分的实验数据？",
			"",
			"请基于 PFE 三元组（问题→特征→效果因果链）判断：",
			"每个 Problem 是否都能通过一条完整的 Feature→Effect 链路实现？",
			"如果某个 Problem 缺少对应的 Feature，即为公开不充分。",
			"",
			"请输出 JSON 格式：",
			`{"can_implement": bool,`,
			`"missing_key_means": bool, "vague_means": bool,`,
			`"only_task_no_means": bool, "insufficient_data": bool,`,
			`"failure_reasons": ["具体原因"], "notes": "详细推理过程"}`,
		}, "\n")

		agent := newEnablementAgent(provider, "enablement-step3", prompt, enablementSchema())
		defer agent.Close()

		inputText := "第 2 步（清楚性检查）结论：\n" + step2Output

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("step3_enablement: %w", err)
		}

		state[stateKeyStep3] = output
		return state, nil
	}
}

// generateConclusionNode 汇总三步骤结果，生成结构化最终结论。
func generateConclusionNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step1, _ := state[stateKeyStep1].(string)
		step2, _ := state[stateKeyStep2].(string)
		step3, _ := state[stateKeyStep3].(string)

		prompt := strings.Join([]string{
			"你是一名资深专利审查员。请基于专利法第26条第3款（充分公开）评估的",
			"三个步骤的产出，生成最终的结构化评估结论。",
			"",
			"结论应包含：",
			"1. 整体判断：该说明书是否满足 26.3 充分公开的要求（is_sufficient: true/false）",
			"2. 置信度：high（证据充分且结论明确）/ medium（有一定依据但存在不确定性）/",
			"   low（信息不足，难以形成确定判断）",
			"3. 具体缺陷列表（deficiencies）：如果 is_sufficient=false，列出所有具体的公开缺陷",
			"4. 法律提示：本判断由 AI 辅助生成，不构成正式法律意见",
			"",
			"请输出 JSON 格式：",
			`{"is_sufficient": bool, "reasoning": "综合推理过程",`,
			`"confidence": "high/medium/low",`,
			`"deficiencies": ["具体缺陷1", "具体缺陷2"],`,
			`"legal_note": "本判断由 AI 辅助生成，不构成正式法律意见"}`,
		}, "\n")

		agent := newEnablementAgent(provider, "enablement-conclusion", prompt, conclusionSchemaDef())
		defer agent.Close()

		inputText := fmt.Sprintf(
			"第 1 步（完整性检查）:\n%s\n\n第 2 步（清楚性检查）:\n%s\n\n第 3 步（能够实现性检查）:\n%s",
			step1, step2, step3)

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("generate_conclusion: %w", err)
		}

		result := buildResult(step1, step2, step3, output)
		state[stateKeyResult] = result
		return state, nil
	}
}

// =============================================================================
// 辅助函数
// =============================================================================

// newEnablementAgent 创建统一配置的 LLM Agent 节点。
// 所有评估节点共享 Temperature=0.2 和 MaxTurns=1，仅 name/prompt/schema 不同。
func newEnablementAgent(provider agentcore.Provider, name, prompt string, schema map[string]any) *agentcore.Agent {
	cfg := agentcore.Config{
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
	}
	if schema != nil {
		cfg.ResponseFormat = agentcore.NewJSONSchemaResponseFormat(name, schema)
	}
	return agentcore.New(cfg)
}

// completenessSchema 是 step1 的 JSON Schema。
func completenessSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"has_tech_field": map[string]any{"type": "boolean"},
			"has_background": map[string]any{"type": "boolean"},
			"has_content":    map[string]any{"type": "boolean"},
			"has_drawings":   map[string]any{"type": "boolean"},
			"has_embodiment": map[string]any{"type": "boolean"},
			"missing_sections": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"score": map[string]any{"type": "number"},
			"notes": map[string]any{"type": "string"},
		},
		"required": []string{"has_tech_field", "has_background", "has_content", "has_embodiment", "missing_sections", "score"},
	}
}

// claritySchema 是 step2 的 JSON Schema。
func claritySchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"is_clear": map[string]any{"type": "boolean"},
			"ambiguous_terms": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"orphan_features": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"orphan_effects": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"notes": map[string]any{"type": "string"},
		},
		"required": []string{"is_clear", "ambiguous_terms", "orphan_features", "orphan_effects"},
	}
}

// enablementSchema 是 step3 的 JSON Schema。
func enablementSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"can_implement":      map[string]any{"type": "boolean"},
			"missing_key_means":  map[string]any{"type": "boolean"},
			"vague_means":        map[string]any{"type": "boolean"},
			"only_task_no_means": map[string]any{"type": "boolean"},
			"insufficient_data":  map[string]any{"type": "boolean"},
			"failure_reasons": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"notes": map[string]any{"type": "string"},
		},
		"required": []string{"can_implement", "missing_key_means", "vague_means", "only_task_no_means", "insufficient_data"},
	}
}

// conclusionSchemaDef 是 generateConclusion 的 JSON Schema。
func conclusionSchemaDef() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"is_sufficient": map[string]any{"type": "boolean"},
			"reasoning":     map[string]any{"type": "string"},
			"confidence": map[string]any{
				"type": "string",
				"enum": []string{"high", "medium", "low"},
			},
			"deficiencies": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required": []string{"is_sufficient", "reasoning", "confidence", "deficiencies"},
	}
}

// extractInput 从 state 中安全读取已验证的 EnablementInput。
func extractInput(state graph.PregelState) *EnablementInput {
	raw, ok := state[stateKeyInput]
	if !ok {
		return nil
	}
	input, ok := raw.(*EnablementInput)
	if !ok {
		return nil
	}
	return input
}

// stateHasSkip 检查 loadInputNode 是否设置了跳过标志。
func stateHasSkip(state graph.PregelState) bool {
	raw, ok := state[stateKeyResult]
	if !ok {
		return false
	}
	r, ok := raw.(*EnablementResult)
	return ok && r != nil && r.Skipped
}

// buildCompletenessInput 构建 Step 1 的 LLM 输入文本。
func buildCompletenessInput(input *EnablementInput) string {
	var sb strings.Builder
	sb.WriteString("## 说明书章节内容\n\n")

	if len(input.DocSections) > 0 {
		for name, content := range input.DocSections {
			fmt.Fprintf(&sb, "### %s\n%s\n\n", name, truncateText(content, 1000))
		}
	} else {
		sb.WriteString("（未提供章节切分结果，请基于技术特征和 PFE 三元组进行判断）\n\n")
	}

	fmt.Fprintf(&sb, "## 是否有附图\n%v\n\n", input.HasDrawings)

	if len(input.Features) > 0 {
		sb.WriteString("## 技术特征\n")
		for _, f := range input.Features {
			fmt.Fprintf(&sb, "- [%s] %s (重要度: %s)\n", f.Category, f.Description, f.Importance)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// buildPFEInput 构建 Step 2/3 的 LLM 输入文本（基于 PFE 三元组）。
func buildPFEInput(input *EnablementInput) string {
	var sb strings.Builder

	if len(input.Problems) > 0 {
		sb.WriteString("## 技术问题\n")
		for _, p := range input.Problems {
			fmt.Fprintf(&sb, "- %s\n", p)
		}
		sb.WriteString("\n")
	}

	if len(input.Features) > 0 {
		sb.WriteString("## 技术特征\n")
		for _, f := range input.Features {
			fmt.Fprintf(&sb, "- [%s] %s (功能: %s, 重要度: %s)\n",
				f.Category, f.Description, f.Function, f.Importance)
		}
		sb.WriteString("\n")
	}

	if len(input.Effects) > 0 {
		sb.WriteString("## 技术效果\n")
		for _, e := range input.Effects {
			fmt.Fprintf(&sb, "- %s\n", e)
		}
		sb.WriteString("\n")
	}

	if len(input.PFETriples) > 0 {
		sb.WriteString("## PFE 三元组（问题→特征→效果因果链）\n")
		for _, t := range input.PFETriples {
			fmt.Fprintf(&sb, "- [%s] 问题: %s → 特征: %v → 效果: %s\n",
				t.ID, t.Problem, t.FeatureIDs, t.Effect)
		}
		sb.WriteString("\n")
	}

	if len(input.GuidelineRefs) > 0 {
		sb.WriteString("## 审查指南参考\n")
		for _, ref := range input.GuidelineRefs {
			fmt.Fprintf(&sb, "- %s\n", ref)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// buildResult 从四步 LLM 输出构建结构化的 EnablementResult。
func buildResult(step1, step2, step3, conclusion string) *EnablementResult {
	result := &EnablementResult{
		Assessed: true,
	}

	// 解析 completeness 结果
	result.Completeness = parseCompleteness(step1)

	// 解析 clarity 结果
	result.Clarity = parseClarity(step2)

	// 解析 enablement 结果
	result.Enablement = parseEnablementJudgment(step3)

	// 解析最终结论
	cc := parseConclusion(conclusion)
	result.Conclusion = cc.Reasoning
	result.IsSufficient = cc.IsSufficient
	result.Confidence = cc.Confidence
	result.Deficiencies = cc.Deficiencies

	return result
}

// parsedConclusion 是最终结论的解析结果。
type parsedConclusion struct {
	IsSufficient bool     `json:"is_sufficient"`
	Reasoning    string   `json:"reasoning"`
	Confidence   string   `json:"confidence"`
	Deficiencies []string `json:"deficiencies"`
}

func parseCompleteness(output string) CompletenessResult {
	r := CompletenessResult{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		r.Notes = output
		return r
	}

	var parsed struct {
		HasTechField    bool     `json:"has_tech_field"`
		HasBackground   bool     `json:"has_background"`
		HasContent      bool     `json:"has_content"`
		HasDrawings     bool     `json:"has_drawings"`
		HasEmbodiment   bool     `json:"has_embodiment"`
		MissingSections []string `json:"missing_sections"`
		Score           float64  `json:"score"`
		Notes           string   `json:"notes"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		r.Notes = output
		return r
	}

	r.MissingSections = parsed.MissingSections
	r.Score = parsed.Score
	r.Notes = parsed.Notes
	return r
}

func parseClarity(output string) ClarityResult {
	r := ClarityResult{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		r.Notes = output
		return r
	}

	var parsed struct {
		IsClear        bool     `json:"is_clear"`
		AmbiguousTerms []string `json:"ambiguous_terms"`
		OrphanFeatures []string `json:"orphan_features"`
		OrphanEffects  []string `json:"orphan_effects"`
		Notes          string   `json:"notes"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		r.Notes = output
		return r
	}

	r.IsClear = parsed.IsClear
	r.AmbiguousTerms = parsed.AmbiguousTerms
	r.OrphanFeatures = parsed.OrphanFeatures
	r.OrphanEffects = parsed.OrphanEffects
	r.Notes = parsed.Notes
	return r
}

func parseEnablementJudgment(output string) EnablementJudgment {
	r := EnablementJudgment{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		r.Notes = output
		return r
	}

	var parsed struct {
		CanImplement     bool     `json:"can_implement"`
		MissingKeyMeans  bool     `json:"missing_key_means"`
		VagueMeans       bool     `json:"vague_means"`
		OnlyTaskNoMeans  bool     `json:"only_task_no_means"`
		InsufficientData bool     `json:"insufficient_data"`
		FailureReasons   []string `json:"failure_reasons"`
		Notes            string   `json:"notes"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		r.Notes = output
		return r
	}

	r.CanImplement = parsed.CanImplement
	r.MissingKeyMeans = parsed.MissingKeyMeans
	r.VagueMeans = parsed.VagueMeans
	r.OnlyTaskNoMeans = parsed.OnlyTaskNoMeans
	r.InsufficientData = parsed.InsufficientData
	r.FailureReasons = parsed.FailureReasons
	r.Notes = parsed.Notes
	return r
}

func parseConclusion(output string) parsedConclusion {
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return parsedConclusion{
			Reasoning:  output,
			Confidence: "medium",
		}
	}

	var parsed parsedConclusion
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return parsedConclusion{
			Reasoning:  output,
			Confidence: "medium",
		}
	}

	switch parsed.Confidence {
	case "high", "medium", "low":
	default:
		parsed.Confidence = "medium"
	}

	return parsed
}

// extractJSON 从文本中提取第一个 JSON 对象。
func extractJSON(text string) string {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}
	return ""
}

// truncateText 截断文本到指定长度（rune 安全）。
func truncateText(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}
