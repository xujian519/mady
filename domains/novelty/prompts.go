package novelty

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// =============================================================================
// JSON Schema 定义
// =============================================================================

// priorArtSchema 现有技术审查的 JSON Schema。
func priorArtSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"effective_date":           map[string]any{"type": "string"},
			"is_publicly_known":        map[string]any{"type": "boolean"},
			"public_known_std":         map[string]any{"type": "string"},
			"is_sufficient_disclosure": map[string]any{"type": "boolean"},
			"prior_art_type":           map[string]any{"type": "string"},
			"disclosure_reason":        map[string]any{"type": "string"},
		},
		"required": []string{"is_publicly_known", "prior_art_type", "disclosure_reason"},
	}
}

// compareSchema 单独对比的 JSON Schema。
func compareSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"claim_features": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"disclosed_features": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"missing_features": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"same_field":            map[string]any{"type": "boolean"},
			"same_problem":          map[string]any{"type": "boolean"},
			"same_effect":           map[string]any{"type": "boolean"},
			"upper_lower_concept":   map[string]any{"type": "string"},
			"direct_replacement":    map[string]any{"type": "boolean"},
			"numeric_range_result":  map[string]any{"type": "string"},
			"full_feature_coverage": map[string]any{"type": "boolean"},
		},
		"required": []string{"claim_features", "disclosed_features", "missing_features", "full_feature_coverage"},
	}
}

// conflictSchema 抵触申请审查的 JSON Schema。
func conflictSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"is_conflict_app": map[string]any{"type": "boolean"},
			"conflict_reasons": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"full_content_compare": map[string]any{"type": "boolean"},
			"conflict_doc_id":      map[string]any{"type": "string"},
		},
		"required": []string{"is_conflict_app", "full_content_compare"},
	}
}

// gracePrioritySchema 宽限期与优先权的 JSON Schema。
func gracePrioritySchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"has_grace_period": map[string]any{"type": "boolean"},
			"grace_type":       map[string]any{"type": "string"},
			"grace_within_6m":  map[string]any{"type": "boolean"},
			"has_priority":     map[string]any{"type": "boolean"},
			"priority_valid":   map[string]any{"type": "boolean"},
			"same_subject":     map[string]any{"type": "string"},
		},
		"required": []string{"has_grace_period", "has_priority"},
	}
}

// specialDomainSchema 特殊领域的 JSON Schema。
func specialDomainSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"domain_specific_rules": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"affects_novelty": map[string]any{"type": "boolean"},
			"reasoning":       map[string]any{"type": "string"},
		},
		"required": []string{"affects_novelty", "reasoning"},
	}
}

// conclusionSchema 最终结论的 JSON Schema。
func conclusionSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"conclusion":  map[string]any{"type": "string"},
			"has_novelty": map[string]any{"type": "boolean"},
			"confidence":  map[string]any{"type": "string"},
			"failed_claims": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required": []string{"conclusion", "has_novelty", "confidence"},
	}
}

// =============================================================================
// 解析函数
// =============================================================================

// parsePriorArt 从 LLM 输出解析 PriorArtResult。
func parsePriorArt(output string) PriorArtResult {
	r := PriorArtResult{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		r.DisclosureReason = output
		return r
	}

	var parsed struct {
		EffectiveDate          string `json:"effective_date"`
		IsPubliclyKnown        bool   `json:"is_publicly_known"`
		PublicKnownStd         string `json:"public_known_std"`
		IsSufficientDisclosure bool   `json:"is_sufficient_disclosure"`
		PriorArtType           string `json:"prior_art_type"`
		DisclosureReason       string `json:"disclosure_reason"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		r.DisclosureReason = output
		return r
	}

	r.EffectiveDate = parsed.EffectiveDate
	r.IsPubliclyKnown = parsed.IsPubliclyKnown
	r.PublicKnownStd = parsed.PublicKnownStd
	r.IsSufficientDisclosure = parsed.IsSufficientDisclosure
	r.PriorArtType = parsed.PriorArtType
	r.DisclosureReason = parsed.DisclosureReason
	return r
}

// parseCompare 从 LLM 输出解析 CompareResult。
func parseCompare(output string) CompareResult {
	r := CompareResult{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return r
	}

	var parsed struct {
		ClaimFeatures       []string `json:"claim_features"`
		DisclosedFeatures   []string `json:"disclosed_features"`
		MissingFeatures     []string `json:"missing_features"`
		SameField           bool     `json:"same_field"`
		SameProblem         bool     `json:"same_problem"`
		SameEffect          bool     `json:"same_effect"`
		UpperLowerConcept   string   `json:"upper_lower_concept"`
		DirectReplacement   bool     `json:"direct_replacement"`
		NumericRangeResult  string   `json:"numeric_range_result"`
		FullFeatureCoverage bool     `json:"full_feature_coverage"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return r
	}

	r.ClaimFeatures = parsed.ClaimFeatures
	r.DisclosedFeatures = parsed.DisclosedFeatures
	r.MissingFeatures = parsed.MissingFeatures
	r.SameField = parsed.SameField
	r.SameProblem = parsed.SameProblem
	r.SameEffect = parsed.SameEffect
	r.UpperLowerConcept = parsed.UpperLowerConcept
	r.DirectReplacement = parsed.DirectReplacement
	r.NumericRangeResult = parsed.NumericRangeResult
	r.FullFeatureCoverage = parsed.FullFeatureCoverage
	return r
}

// parseConflict 从 LLM 输出解析 ConflictResult。
func parseConflict(output string) ConflictResult {
	r := ConflictResult{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return r
	}

	var parsed struct {
		IsConflictApp      bool     `json:"is_conflict_app"`
		ConflictReasons    []string `json:"conflict_reasons"`
		FullContentCompare bool     `json:"full_content_compare"`
		ConflictDocID      string   `json:"conflict_doc_id"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return r
	}

	r.IsConflictApp = parsed.IsConflictApp
	r.ConflictReasons = parsed.ConflictReasons
	r.FullContentCompare = parsed.FullContentCompare
	r.ConflictDocID = parsed.ConflictDocID
	return r
}

// parseGracePriority 从 LLM 输出解析 ExceptionResult。
func parseGracePriority(output string) ExceptionResult {
	r := ExceptionResult{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return r
	}

	var parsed struct {
		HasGracePeriod bool   `json:"has_grace_period"`
		GraceType      string `json:"grace_type"`
		GraceWithin6m  bool   `json:"grace_within_6m"`
		HasPriority    bool   `json:"has_priority"`
		PriorityValid  bool   `json:"priority_valid"`
		SameSubject    string `json:"same_subject"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return r
	}

	r.HasGracePeriod = parsed.HasGracePeriod
	r.GraceType = parsed.GraceType
	r.GraceWithin6m = parsed.GraceWithin6m
	r.HasPriority = parsed.HasPriority
	r.PriorityValid = parsed.PriorityValid
	r.SameSubject = parsed.SameSubject
	return r
}

// parseConclusion 从 LLM 输出解析最终结论。
func parseConclusion(output string) parsedConclusion {
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return parsedConclusion{
			Conclusion: output,
			Confidence: "medium",
		}
	}

	var parsed parsedConclusion
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return parsedConclusion{
			Conclusion: output,
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

// =============================================================================
// 结论构建
// =============================================================================

// buildResult 从各步 LLM 输出构建完整的 NoveltyResult。
func buildResult(priorArt, compare, conflict, gracePriority, conclusion string) *NoveltyResult {
	result := &NoveltyResult{
		Assessed: true,
	}

	// Parse individual step results.
	result.PriorArtCheck = parsePriorArt(priorArt)
	result.SingleCompare = parseCompare(compare)
	result.ConflictCheck = parseConflict(conflict)
	result.GracePriority = parseGracePriority(gracePriority)

	// Parse final conclusion.
	cc := parseConclusion(conclusion)
	result.Conclusion = cc.Conclusion
	result.Confidence = cc.Confidence
	result.FailedClaims = cc.FailedClaims

	// 核心结论逻辑：HasNovelty 决策
	// 优先级：1. LLM 显式结论 > 2. 自动计算
	switch {
	case cc.HasNovelty:
		result.HasNovelty = true
	default:
		// 默认逻辑：
		// 不具备新颖性 = 特征全覆盖 OR 构成抵触申请
		result.HasNovelty = !result.SingleCompare.FullFeatureCoverage && !result.ConflictCheck.IsConflictApp
	}

	return result
}

// =============================================================================
// Agent 工厂函数
// =============================================================================

// newNoveltyAgent 创建统一配置的 Agent 节点。
// 所有 LLM 节点共享 Temperature=0.2 和 MaxTurns=1，仅 name/prompt/schema 不同。
func newNoveltyAgent(provider agentcore.Provider, name, prompt string, schema map[string]any) *agentcore.Agent {
	if provider == nil {
		return agentcore.New(agentcore.Config{
			ModelConfig: agentcore.ModelConfig{
				Name: name,
			},
		})
	}
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

// =============================================================================
// 辅助函数
// =============================================================================

// extractInput 从 state 中安全读取已验证的 NoveltyInput。
func extractInput(state map[string]any) *NoveltyInput {
	raw, ok := state[StateKeyNoveltyInput]
	if !ok {
		return nil
	}
	input, ok := raw.(*NoveltyInput)
	if !ok {
		return nil
	}
	return input
}

// stateHasSkip 检查 loadInputNode 是否设置了跳过标志。
func stateHasSkip(state map[string]any) bool {
	raw, ok := state[StateKeyNoveltyResult]
	if !ok {
		return false
	}
	r, ok := raw.(*NoveltyResult)
	return ok && r != nil && r.Skipped
}

// getStateString 从 state 中读取字符串值，key 不存在或类型不匹配时返回空字符串。
func getStateString(state map[string]any, key string) string {
	raw, ok := state[key]
	if !ok {
		return ""
	}
	s, ok := raw.(string)
	if !ok {
		return ""
	}
	return s
}

// buildInputText 将结构化输入格式化为 LLM 友好的 Markdown 文本。
func buildInputText(input *NoveltyInput) string {
	var sb strings.Builder
	if input == nil {
		return ""
	}

	// 申请日和优先权信息
	fmt.Fprintf(&sb, "## 申请信息\n")
	fmt.Fprintf(&sb, "- 申请日：%s\n", input.FilingDate)
	if input.PriorityDate != "" {
		fmt.Fprintf(&sb, "- 优先权日：%s\n", input.PriorityDate)
	}
	if input.PriorityInfo != nil && input.PriorityInfo.HasPriority {
		fmt.Fprintf(&sb, "- 优先权类型：%s\n", input.PriorityInfo.PriorityType)
	}
	fmt.Fprintf(&sb, "- 技术领域：%s\n", input.TechDomain)
	if input.GracePeriodInfo != nil && input.GracePeriodInfo.HasGraceClaim {
		fmt.Fprintf(&sb, "- 宽限期主张：%s（%s，6个月内：%v）\n",
			input.GracePeriodInfo.GraceType, input.GracePeriodInfo.GraceDate, input.GracePeriodInfo.WithinSixMonths)
	}
	sb.WriteString("\n")

	// 权利要求
	if len(input.Claims) > 0 {
		sb.WriteString("## 权利要求\n")
		for _, c := range input.Claims {
			fmt.Fprintf(&sb, "[%s] (%s) %s\n", c.ID, c.Type, c.Text)
		}
		sb.WriteString("\n")
	}

	// 对比文件
	if len(input.PriorArtDocs) > 0 {
		sb.WriteString("## 对比文件（现有技术）\n")
		for i, d := range input.PriorArtDocs {
			fmt.Fprintf(&sb, "[%d] %s (公开日: %s, 类型: %s)\n", i+1, d.Title, d.PubDate, d.PubType)
			if d.Snippet != "" {
				fmt.Fprintf(&sb, "    摘要: %s\n", d.Snippet)
			}
		}
		sb.WriteString("\n")
	}

	// 抵触申请
	if len(input.ConflictApps) > 0 {
		sb.WriteString("## 抵触申请\n")
		for i, ca := range input.ConflictApps {
			fmt.Fprintf(&sb, "[%d] %s (在先申请日: %s, 公开日: %s)\n", i+1, ca.Title, ca.FilingDate, ca.PubDate)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// =============================================================================
// 公共提示词片段
// =============================================================================

// personSkilledDefinition 返回「本领域的技术人员」标准定义。
func personSkilledDefinition() string {
	return strings.Join([]string{
		"**判断主体：「本领域的技术人员」**",
		"一种假设的「人」，具有以下属性：",
		"- 知晓申请日/优先权日之前所属技术领域所有的普通技术知识",
		"- 能够获知该领域中所有的现有技术",
		"- 具有应用该日期之前常规实验手段的能力",
		"- 不具有创造能力",
		"- 如果技术问题促使其在其他技术领域寻找技术手段，也具有从其他技术领域获知的能力",
	}, "\n")
}

// chemistryNoveltyFramework 返回化学领域新颖性判断的特殊规则框架。
func chemistryNoveltyFramework() string {
	return strings.Join([]string{
		"## 化学领域发明的新颖性——特殊判断规则",
		"",
		"**审查指南依据**：审查指南第二部分第十章",
		"",
		"### 1. 化合物发明",
		"",
		"**1.1 通式化合物**",
		"- 通式化合物的公开**不破坏**该通式范围内具体化合物的新颖性",
		"- 除非对比文件明确提到了该具体化合物（即使只是列举）",
		"",
		"**1.2 具体化合物**",
		"- 对比文件明确提到了具体化合物 → 该具体化合物丧失新颖性",
		"",
		"**1.3 立体异构体和互变异构体**",
		"- 对比文件公开的是外消旋混合物 → 特定对映异构体可能具有新颖性",
		"- 需判断所属领域技术人员是否有动机获得该特定异构体",
		"",
		"**1.4 晶体化合物**",
		"- 用 X 射线粉末衍射峰表征晶体微观结构时，应将所有衍射峰作为",
		"  **整体特征**与对比文件比对",
		"- 如果衍射峰整体不同 → 晶体化合物具备新颖性",
		"",
		"**1.5 纯度限定的化合物**",
		"- 仅以纯度限定的化合物，如果高纯度产品在现有技术中已是常规可获得 → 无新颖性",
		"",
		"### 2. 组合物发明",
		"",
		"**2.1 开放式与封闭式表达**",
		"- 开放式表达（「包含」「含有」等）：允许含有未列出的组分",
		"- 封闭式表达（「由……组成」）：不允许含有未列出的组分",
		"- 对比文件以开放式表达的组合物 → 不破坏以封闭式表达限定的组合物的新颖性",
		"",
		"**2.2 包含用药特征的药物组合物**",
		"- 用药特征（给药剂量、给药方式）如果不隐含药物组合物本身的结构或组成变化",
		"  → 不限制产品权利要求",
		"",
		"### 3. 物质的制药用途发明",
		"",
		"- 制药用途发明的新颖性取决于**疾病适应症**是否与现有技术相同",
		"- 如果适应症不同，即使活性物质相同 → 具备新颖性",
		"- 基于新的作用机理发现的新制药用途，如果该用途在现有技术中未被公开 → 具备新颖性",
		"- 如果现有技术已公开该物质可用于治疗某疾病（即使描述层次不同）→ 可能破坏新颖性",
		"",
		"### 4. 性能/参数/用途/制备方法特征",
		"",
		"- **性能、参数特征**：如果隐含了特定结构/组成 → 则有新颖性；否则可推定无新颖性",
		"- **用途特征**：如果用途由产品固有特性决定且未改变结构/组成 → 则无新颖性",
		"- **制备方法特征**：如果方法导致产品结构/组成不同 → 则有新颖性",
		"- 示例：用 X 衍射数据等多种参数表征的结晶形态化合物 A，如果根据对比文件",
		"  难以区分结晶形态 → 可推定无新颖性，除非申请人能证明结晶形态不同",
	}, "\n")
}

// extractJSON 从文本中提取第一个 JSON 对象字符串。
func extractJSON(text string) string {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}
	return ""
}
