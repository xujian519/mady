package specdrafting

import (
	"context"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// 节点辅助函数
// =============================================================================

// newSpecAgent 创建统一配置的 LLM Agent 节点。
func newSpecAgent(provider agentcore.Provider, name, prompt string, schema map[string]any) *agentcore.Agent {
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

// stateHasSkip 检查 state 中是否包含跳过标记。
func stateHasSkip(state graph.PregelState) bool {
	if v, ok := state[StateKeyOutput]; ok {
		if s, ok := v.(*SpecOutput); ok && s != nil && s.Metadata.FeatureCount == -1 {
			return true
		}
	}
	return false
}

// extractInput 从 state 中提取 SpecInput。
func extractInput(state graph.PregelState) *SpecInput {
	if v, ok := state[StateKeyInput]; ok {
		if input, ok := v.(*SpecInput); ok {
			return input
		}
	}
	return nil
}

// =============================================================================
// 节点实现
// =============================================================================

// loadInputNode 从 PregelState 读取输入并验证。
func loadInputNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		raw, ok := state[StateKeyInput]
		if !ok {
			state[StateKeyOutput] = &SpecOutput{
				Metadata: SpecMetadata{FeatureCount: -1},
				Warnings: []string{"未提供输入数据"},
			}
			return state, nil
		}

		input, ok := raw.(*SpecInput)
		if !ok || input == nil {
			state[StateKeyOutput] = &SpecOutput{
				Metadata: SpecMetadata{FeatureCount: -1},
				Warnings: []string{"输入数据格式无效"},
			}
			return state, nil
		}

		// 检测技术领域
		domain := input.TechDomain
		if domain == "" {
			domain = classifyDomain(input)
			input.TechDomain = domain
		}

		state[StateKeyInput] = input
		state[StateKeyDomain] = string(domain)
		return state, nil
	}
}

// classifyDomainNode 细化技术领域分类。
// 即使 input.TechDomain 已设置，也执行关键词重分类以覆盖 DomainGeneral 等情况。
func classifyDomainNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}

		// 始终执行分类器，用于细化
		detected := classifyDomain(input)
		if detected != DomainGeneral {
			// 仅当原值为 '' 或 DomainGeneral 时覆盖
			if input.TechDomain == "" || input.TechDomain == DomainGeneral {
				input.TechDomain = detected
				state[StateKeyDomain] = string(detected)
			}
		}
		return state, nil
	}
}

// draftTitleNode 生成发明/实用新型名称。
func draftTitleNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}

		title := input.Title
		if title == "" && len(input.Problems) > 0 {
			title = input.Problems[0]
		}
		if title == "" {
			title = "一种" + string(input.TechDomain) + "技术方案"
		}

		state[StateKeyTitle] = title
		return state, nil
	}
}

// draftTechFieldNode 生成技术领域章节。
func draftTechFieldNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}

		builder := NewSpecBuilder(nil)
		content := builder.defaultTechField(*input, input.TechDomain)
		state[StateKeyTechField] = content
		return state, nil
	}
}

// draftBackgroundNode 生成背景技术章节。
func draftBackgroundNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}
		builder := NewSpecBuilder(nil)
		content := builder.defaultBackground(*input)
		state[StateKeyBackground] = content
		return state, nil
	}
}

// draftContentNode 生成发明内容章节（问题+方案+效果）。
func draftContentNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}
		builder := NewSpecBuilder(nil)
		content := builder.defaultContent(*input, input.TechDomain)
		state[StateKeyContent] = content
		return state, nil
	}
}

// draftDrawingsNode 生成附图说明章节。
func draftDrawingsNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}
		builder := NewSpecBuilder(nil)
		content := builder.defaultDrawings(*input)
		state[StateKeyDrawings] = content
		return state, nil
	}
}

// draftEmbodimentNode 生成具体实施方式章节。
func draftEmbodimentNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}
		builder := NewSpecBuilder(nil)
		content := builder.defaultEmbodiment(*input, input.TechDomain)
		state[StateKeyEmbodiment] = content
		return state, nil
	}
}

// draftAbstractNode 生成摘要。
func draftAbstractNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}

		title, _ := state[StateKeyTitle].(string)
		inputCopy := *input
		inputCopy.Title = title
		builder := NewSpecBuilder(nil)
		abstract := builder.buildAbstract(inputCopy, nil)
		state[StateKeyAbstract] = abstract
		return state, nil
	}
}

// validateNode 运行规则引擎验证。
func validateNode(engine *RuleEngine) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil || engine == nil {
			return state, nil
		}

		// 从 state 构建完整的 SpecOutput
		spec := buildSpecFromState(state, *input)
		violations := engine.Validate(spec, *input)
		warnings := make([]string, 0, len(violations))
		for _, v := range violations {
			warnings = append(warnings, "["+v.RuleName+"] "+v.Message)
		}
		state[StateKeyOutput+"_violations"] = violations
		state[StateKeyOutput+"_warnings"] = warnings
		return state, nil
	}
}

// scoreNode 运行质量评分器。
func scoreNode(scorer *SpecScorer) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil || scorer == nil {
			return state, nil
		}

		spec := buildSpecFromState(state, *input)
		report := scorer.Score(spec, *input)
		state[StateKeyScore] = report
		return state, nil
	}
}

// finalizeNode 组装最终的 SpecOutput。
func finalizeNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}

		spec := buildSpecFromState(state, *input)

		// 附加评分
		if report, ok := state[StateKeyScore].(*ScoreReport); ok && report != nil {
			spec.Score = report.OverallScore
			for _, v := range report.Violations {
				spec.Warnings = append(spec.Warnings, "["+v.RuleName+"] "+v.Message)
			}
		}

		// 附加 warn
		if w, ok := state[StateKeyOutput+"_warnings"].([]string); ok {
			spec.Warnings = append(spec.Warnings, w...)
		}

		spec.Timestamp = timestamp()
		state[StateKeyOutput] = spec
		return state, nil
	}
}

// =============================================================================
// 辅助函数
// =============================================================================

// buildSpecFromState 从 state 各章节 key 构建完整的 SpecOutput。
func buildSpecFromState(state graph.PregelState, input SpecInput) *SpecOutput {
	title, _ := state[StateKeyTitle].(string)
	abstract, _ := state[StateKeyAbstract].(string)
	tf, _ := state[StateKeyTechField].(string)
	bg, _ := state[StateKeyBackground].(string)
	ct, _ := state[StateKeyContent].(string)
	dw, _ := state[StateKeyDrawings].(string)
	em, _ := state[StateKeyEmbodiment].(string)
	domainStr, _ := state[StateKeyDomain].(string)

	sections := []SpecSection{}
	if tf != "" {
		sections = append(sections, SpecSection{Name: SecTechField, Content: tf, WordCnt: ChineseCharCount(tf)})
	}
	if bg != "" {
		sections = append(sections, SpecSection{Name: SecBackground, Content: bg, WordCnt: ChineseCharCount(bg)})
	}
	if ct != "" {
		sections = append(sections, SpecSection{Name: SecContent, Content: ct, WordCnt: ChineseCharCount(ct)})
	}
	if dw != "" {
		sections = append(sections, SpecSection{Name: SecDrawings, Content: dw, WordCnt: ChineseCharCount(dw)})
	}
	if em != "" {
		sections = append(sections, SpecSection{Name: SecEmbodiment, Content: em, WordCnt: ChineseCharCount(em)})
	}

	totalWords := 0
	for _, s := range sections {
		totalWords += s.WordCnt
	}

	domain := TechDomain(domainStr)
	if domain == "" {
		domain = input.TechDomain
	}

	return &SpecOutput{
		Title:    title,
		Abstract: abstract,
		Sections: sections,
		Metadata: SpecMetadata{
			PatentType:   input.PatentType,
			TechDomain:   domain,
			FeatureCount: len(input.Features),
			HasDrawings:  input.HasDrawings,
			WordCount:    totalWords,
		},
	}
}

// classifyDomain 根据输入特征推断技术领域。
func classifyDomain(input *SpecInput) TechDomain {
	allText := input.Title
	for _, p := range input.Problems {
		allText += " " + p
	}
	for _, f := range input.Features {
		allText += " " + f.Description + " " + f.Category
	}
	for _, e := range input.Effects {
		allText += " " + e
	}

	mechKw := []string{"机械", "装置", "机构", "连接", "固定", "支撑", "壳体", "弹簧", "齿轮"}
	elecKw := []string{"电路", "电压", "电流", "信号", "电极", "导线", "传感器", "放大"}
	chemKw := []string{"组合物", "化合物", "组分", "含量", "百分比", "摩尔", "催化剂"}
	softKw := []string{"数据", "方法", "步骤", "程序", "处理", "计算", "算法", "图像"}

	score := map[TechDomain]int{
		DomainMechanical: countKeys(allText, mechKw),
		DomainElectrical: countKeys(allText, elecKw),
		DomainChemical:   countKeys(allText, chemKw),
		DomainSoftware:   countKeys(allText, softKw),
	}

	// 特征分类加权
	for _, f := range input.Features {
		switch f.Category {
		case "structure":
			score[DomainMechanical] += 2
		case "parameter":
			score[DomainElectrical] += 2
		case "material":
			score[DomainChemical] += 3
		case "method":
			score[DomainSoftware] += 2
		}
	}

	var best TechDomain
	bestScore := 0
	for d, s := range score {
		if s > bestScore {
			bestScore = s
			best = d
		}
	}

	if bestScore == 0 {
		return DomainGeneral
	}
	return best
}

func countKeys(text string, keys []string) int {
	n := 0
	for _, k := range keys {
		if containsStr(text, k) {
			n++
		}
	}
	return n
}

// sectionJSONSchema 生成各章节的 JSON Schema。
func sectionJSONSchema(field string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			field: map[string]any{"type": "string", "description": "该章节的完整内容"},
		},
		"required": []string{field},
	}
}

// titleJSONSchema 生成标题的 JSON Schema。
var titleJSONSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"title":    map[string]any{"type": "string", "description": "发明或实用新型名称，不超过25个字"},
		"abstract": map[string]any{"type": "string", "description": "摘要，不超过300字"},
	},
	"required": []string{"title", "abstract"},
}

// serializeStateSection 序列化 state 中的章节内容为 LLM 输入。
func serializeStateSection(state graph.PregelState, key string) string {
	if v, ok := state[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// buildLLMContext 构建 LLM 节点的完整上下文输入。
func buildLLMContext(state graph.PregelState) string {
	input := extractInput(state)
	if input == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("发明名称：" + input.Title + "\n")
	if len(input.Problems) > 0 {
		b.WriteString("技术问题：" + strings.Join(input.Problems, "；") + "\n")
	}
	if len(input.Features) > 0 {
		b.WriteString("技术特征：\n")
		for _, f := range input.Features {
			b.WriteString("- " + f.Description)
			if f.Function != "" {
				b.WriteString("（用于" + f.Function + "）")
			}
			b.WriteString("\n")
		}
	}
	if len(input.Effects) > 0 {
		b.WriteString("技术效果：" + strings.Join(input.Effects, "；") + "\n")
	}
	return b.String()
}
