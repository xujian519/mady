package specdrafting

import (
	"github.com/xujian519/mady/domains/rulekit"
)

// =============================================================================
// SpecRule 规则接口与 RuleEngine（骨架复用 rulekit，见 domains/rulekit）
// =============================================================================

// SpecRule 是说明书验证规则接口（rulekit.Rule 的类型别名）。
type SpecRule = rulekit.Rule[*SpecOutput, SpecInput]

// RuleEngine 管理一组验证规则，提供批量验证能力（rulekit.Engine 的类型别名）。
type RuleEngine = rulekit.Engine[*SpecOutput, SpecInput]

// NewRuleEngine 创建一个空的规则引擎。
func NewRuleEngine() *RuleEngine {
	return rulekit.NewEngine[*SpecOutput, SpecInput]()
}

// =============================================================================
// 默认规则注册
// =============================================================================

// RegisterDefaultRules 注册所有默认说明书验证规则到引擎。
func RegisterDefaultRules(engine *RuleEngine) {
	engine.RegisterAll(
		&structureSectionsRule{BaseRule: rulekit.NewBaseRule("structure-sections",
			"说明书必须包含五项必要章节", "专利法实施细则第18条")},
		&structureTitleLengthRule{BaseRule: rulekit.NewBaseRule("structure-title-length",
			"发明名称不得超过25个字", "专利法实施细则第17条第1款")},
		&structureAbstractLengthRule{BaseRule: rulekit.NewBaseRule("structure-abstract-length",
			"说明书摘要不超过300字", "专利法实施细则第23条第2款")},
		&structureContentTriadRule{BaseRule: rulekit.NewBaseRule("structure-content-triad",
			"发明内容须包含问题+方案+效果三要素", "专利法实施细则第18条第1款第(三)项")},
		&structureEmbodimentDetailRule{BaseRule: rulekit.NewBaseRule("structure-embodiment-detail",
			"具体实施方式应至少给出一个详细实施例", "专利法第26条第3款")},

		&clarityTerminologyRule{BaseRule: rulekit.NewBaseRule("clarity-terminology",
			"应使用清楚的技术术语", "专利法第26条第3款")},
		&clarityForbiddenWordsRule{BaseRule: rulekit.NewBaseRule("clarity-forbidden-words",
			"不得使用禁止用词和不确定用语", "专利法第26条第3款；审查指南第二部分第二章§2.1.1")},
		&clarityPFEConsistencyRule{BaseRule: rulekit.NewBaseRule("clarity-pfe-consistency",
			"问题、方案、效果三者应相互适应", "专利法第26条第3款")},
		&clarityTermConsistencyRule{BaseRule: rulekit.NewBaseRule("clarity-term-consistency",
			"术语全文应保持一致", "专利法第26条第3款")},
		&clarityEffectsSpecificRule{BaseRule: rulekit.NewBaseRule("clarity-effects-specific",
			"有益效果应具体分析因果关系，避免仅写笼统优点", "审查指南第二部分第二章§2.1.3")},
		&clarityCitationRule{BaseRule: rulekit.NewBaseRule("clarity-citation",
			"背景技术应引证反映现有技术的文件", "专利法实施细则第17条第2款")},

		&domainMechanicalRule{BaseRule: rulekit.NewBaseRule("domain-mechanical",
			"机械领域应描述零部件及其配置关系", "审查指南第二部分第二章")},
		&domainElectricalRule{BaseRule: rulekit.NewBaseRule("domain-electrical",
			"电学领域应描述元器件、连接关系和功能", "审查指南第二部分第二章")},
		&domainChemicalRule{BaseRule: rulekit.NewBaseRule("domain-chemical",
			"化学领域应公开组分含量及实验数据", "审查指南第二部分第十章")},
		&domainChemicalEmbodimentRule{BaseRule: rulekit.NewBaseRule("domain-chemical-embodiment",
			"化学领域应提供足够数量和类型的实施例", "审查指南第二部分第十章§3.4")},
		&domainSoftwareRule{BaseRule: rulekit.NewBaseRule("domain-software",
			"软件领域应描述方法步骤或功能模块", "审查指南第二部分第九章§5.2")},

		&utilityDrawingsRequiredRule{BaseRule: rulekit.NewBaseRule("utility-drawings-required",
			"实用新型必须有附图", "专利法实施细则第39条")},
		&utilityProductOnlyRule{BaseRule: rulekit.NewBaseRule("utility-product-only",
			"实用新型仅保护产品形状/构造", "专利法第2条第3款")},
		&utilitySingleIndependentRule{BaseRule: rulekit.NewBaseRule("utility-single-independent",
			"实用新型应只有一个独立权利要求", "专利法实施细则第21条第1款")},

		// 充分公开规则（专利法第26条第3款）
		&enablementMeansExistRule{BaseRule: rulekit.NewBaseRule("enablement-means-exist",
			"说明书应当清楚、完整地说明发明，使所属领域技术人员能够实现", "专利法第26条第3款；审查指南第二部分第二章§2.1.3")},
		&enablementExperimentEvidenceRule{BaseRule: rulekit.NewBaseRule("enablement-experiment-evidence",
			"化学/材料领域应提供实验数据证实技术效果", "专利法第26条第3款；审查指南第二部分第二章§2.1.3")},
	)
}

// =============================================================================
// 字符串检查辅助
// =============================================================================

var uncertainWords = []string{
	"约", "大约", "左右", "接近",
	"高温", "高压", "低温", "低压",
	"合适的", "一定的",
}

var forbiddenWords = []string{
	"最好是", "最好",
	"尤其是", "必要时",
	"等", "或类似物",
	"性能卓越", "市场广阔",
}
