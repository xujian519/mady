package evidence

// BurdenScenario 描述举证责任分配的场景。
type BurdenScenario string

const (
	BurdenScenarioPatentInvalidation BurdenScenario = "patent_invalidation" // 专利无效宣告
	BurdenScenarioPatentInfringement BurdenScenario = "patent_infringement" // 专利侵权
	BurdenScenarioNoveltyChallenge   BurdenScenario = "novelty_challenge"   // 新颖性质疑
	BurdenScenarioInventiveness      BurdenScenario = "inventiveness"       // 创造性评估
	BurdenScenarioDisclosure         BurdenScenario = "disclosure"          // 充分公开
	BurdenScenarioPriority           BurdenScenario = "priority"            // 优先权核实
)

// BurdenRule 描述某场景下的举证责任规则。
type BurdenRule struct {
	Scenario     BurdenScenario `yaml:"scenario"`
	Holder       string         `yaml:"holder"`
	Standard     string         `yaml:"standard"`
	ShiftAllowed bool           `yaml:"shift_allowed"`
	ShiftTrigger string         `yaml:"shift_trigger"`
}

// DefaultBurdenRules 返回各场景的默认举证责任规则。
func DefaultBurdenRules() []BurdenRule {
	return []BurdenRule{
		{
			Scenario:     BurdenScenarioPatentInvalidation,
			Holder:       "请求人",
			Standard:     "优势证据",
			ShiftAllowed: true,
			ShiftTrigger: "请求人提供初步证据后，举证责任转移至专利权人",
		},
		{
			Scenario:     BurdenScenarioPatentInfringement,
			Holder:       "专利权人",
			Standard:     "高度盖然性",
			ShiftAllowed: true,
			ShiftTrigger: "专利权人证明侵权基本事实后，举证责任转移至被控侵权人",
		},
		{
			Scenario:     BurdenScenarioNoveltyChallenge,
			Holder:       "质疑方",
			Standard:     "优势证据",
			ShiftAllowed: false,
			ShiftTrigger: "",
		},
		{
			Scenario:     BurdenScenarioInventiveness,
			Holder:       "请求人",
			Standard:     "优势证据",
			ShiftAllowed: false,
			ShiftTrigger: "",
		},
		{
			Scenario:     BurdenScenarioDisclosure,
			Holder:       "审查员",
			Standard:     "优势证据",
			ShiftAllowed: true,
			ShiftTrigger: "审查员提出充分公开缺陷后，申请人负反驳义务",
		},
		{
			Scenario:     BurdenScenarioPriority,
			Holder:       "主张优先权方",
			Standard:     "高度盖然性",
			ShiftAllowed: false,
			ShiftTrigger: "",
		},
	}
}

// DetermineBurden 根据场景确定举证责任分配。
func DetermineBurden(scenario BurdenScenario, context map[string]string) *BurdenDetermination {
	rules := DefaultBurdenRules()

	for _, rule := range rules {
		if rule.Scenario == scenario {
			result := &BurdenDetermination{
				BurdenHolder: rule.Holder,
				Standard:     rule.Standard,
				HasShifted:   false,
				Reasoning:    formatBurdenReasoning(rule, context),
			}

			// 检查是否满足举证责任转移条件
			if rule.ShiftAllowed {
				for k, v := range context {
					if k == "prima_facie_established" || k == "初步证据已提供" {
						if v == "true" || v == "yes" || v == "是" {
							result.HasShifted = true
							result.ShiftReason = rule.ShiftTrigger
						}
					}
				}
			}

			return result
		}
	}

	// 未知场景，默认由主张方承担举证责任
	return &BurdenDetermination{
		BurdenHolder: "主张方",
		Standard:     "优势证据",
		HasShifted:   false,
		Reasoning:    "未匹配到特定场景规则，默认由主张方承担举证责任",
	}
}

func formatBurdenReasoning(rule BurdenRule, ctx map[string]string) string {
	reasoning := "场景: " + string(rule.Scenario) + "; "
	reasoning += "举证责任方: " + rule.Holder + "; "
	reasoning += "证明标准: " + rule.Standard + "."

	if rule.ShiftAllowed && rule.ShiftTrigger != "" {
		reasoning += " 可转移条件: " + rule.ShiftTrigger + "."
	}

	if len(ctx) > 0 {
		reasoning += " 上下文信息: "
		first := true
		for k, v := range ctx {
			if !first {
				reasoning += "; "
			}
			reasoning += k + "=" + v
			first = false
		}
		reasoning += "."
	}

	return reasoning
}
