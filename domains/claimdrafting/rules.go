package claimdrafting

import (
	"github.com/xujian519/mady/domains/rulekit"
)

// =============================================================================
// ClaimRule 规则接口与 RuleEngine（骨架复用 rulekit，见 domains/rulekit）
// =============================================================================

// ClaimRule 是权利要求验证规则接口（rulekit.Rule 的类型别名）。
// 所有规则实现此接口，通过 RuleEngine 注册和执行。
type ClaimRule = rulekit.Rule[[]Claim, DraftInput]

// RuleEngine 管理一组验证规则，提供批量验证能力（rulekit.Engine 的类型别名）。
type RuleEngine = rulekit.Engine[[]Claim, DraftInput]

// NewRuleEngine 创建一个空的规则引擎。
func NewRuleEngine() *RuleEngine {
	return rulekit.NewEngine[[]Claim, DraftInput]()
}

// =============================================================================
// 规则注册辅助函数（集中注册所有规则到引擎）
// =============================================================================

// RegisterDefaultRules 注册所有默认验证规则到引擎。
func RegisterDefaultRules(engine *RuleEngine) {
	// 清楚性规则
	engine.RegisterAll(
		&clarityClaimTypeRule{BaseRule: rulekit.NewBaseRule("clarity-claim-type",
			"权利要求的类型应当清楚：必须明确是产品权利要求还是方法权利要求，不允许混合类型",
			"专利法实施细则第20条第2款")},
		&clarityWordingRule{BaseRule: rulekit.NewBaseRule("clarity-wording",
			"不得使用含义不确定的用语（如'约'、'大约'、'厚'、'薄'、'高温'、'高压'等）",
			"专利法第26条第4款")},
		&clarityForbiddenWordsRule{BaseRule: rulekit.NewBaseRule("clarity-forbidden-words",
			"不得使用'例如'、'最好是'、'尤其是'、'必要时'等导致保护范围不清楚的用语",
			"专利法第26条第4款")},
		&clarityReferenceRule{BaseRule: rulekit.NewBaseRule("clarity-reference",
			"从属权利要求的引用关系应当清楚：多项从属只能择一引用（用'或'），不得用'和'",
			"专利法实施细则第23条第2款")},
		&clarityReferenceChainRule{BaseRule: rulekit.NewBaseRule("clarity-reference-chain",
			"引用关系不得形成循环依赖",
			"专利法第26条第4款")},
		&clarityAntecedentBasisRule{BaseRule: rulekit.NewBaseRule("clarity-antecedent-basis",
			"从属权利要求中的术语应当在被引用的权利要求中有引用基础（先行词）",
			"专利法第26条第4款")},
	)

	// 形式规范规则
	engine.RegisterAll(
		&formalityNumberingRule{BaseRule: rulekit.NewBaseRule("formality-numbering",
			"权利要求书有多项权利要求的，应当用阿拉伯数字顺序编号",
			"专利法实施细则第20条第1款")},
		&formalityPeriodRule{BaseRule: rulekit.NewBaseRule("formality-period",
			"每一项权利要求只允许在其结尾处使用句号",
			"专利法实施细则第20条第4款")},
		&formalityNoIllustrationRule{BaseRule: rulekit.NewBaseRule("formality-no-illustration",
			"权利要求书中不得有插图",
			"专利法实施细则第20条第3款")},
		&formalityMultipleDependentRule{BaseRule: rulekit.NewBaseRule("formality-multiple-dependent",
			"多项从属权利要求不得作为另一项多项从属权利要求的基础",
			"专利法实施细则第23条第2款")},
		&formalityThemeConsistencyRule{BaseRule: rulekit.NewBaseRule("formality-theme-consistency",
			"从属权利要求的类型和主题名称应当与其引用的权利要求一致",
			"专利法实施细则第22条第3款")},
		&formalityScopeNarrowingRule{BaseRule: rulekit.NewBaseRule("formality-scope-narrowing",
			"从属权利要求的保护范围应当在其引用权利要求的保护范围之内",
			"专利法第26条第4款")},
		&formalityParallelClaimRule{BaseRule: rulekit.NewBaseRule("formality-parallel-claim",
			"并列独立权利要求的引用关系应当合法，不得循环引用或引用自身",
			"审查指南(2010)第二部分第二章§3.3")},
		&formalityDependentOrderingRule{BaseRule: rulekit.NewBaseRule("formality-dependent-ordering",
			"从属权利要求应从宽到窄递进布局，形成金字塔型保护层次",
			"审查指南(2010)第二部分第二章§3.3")},
	)

	// 支持性规则
	engine.RegisterAll(
		&supportEmbodimentRule{BaseRule: rulekit.NewBaseRule("support-embodiment",
			"权利要求的概括应当得到说明书实施例的支持，不得超出说明书公开的范围",
			"专利法第26条第4款")},
		&supportFunctionalRule{BaseRule: rulekit.NewBaseRule("support-functional",
			"功能性限定的使用应当恰当，以说明书中记载了具体的实现方式为前提",
			"审查指南第二部分第二章§3.2.1")},
		&supportPureFunctionalRule{BaseRule: rulekit.NewBaseRule("support-pure-functional",
			"不得出现纯功能性权利要求（仅用功能描述整个技术方案）",
			"审查指南第二部分第二章§3.2.1")},
		&supportMarkushUnityRule{BaseRule: rulekit.NewBaseRule("support-markush-unity",
			"马库什权利要求中的各可选方案应具有共同结构，满足单一性要求",
			"审查指南第二部分第十章§4.3")},
		&supportFunctionalVarietyRule{BaseRule: rulekit.NewBaseRule("support-functional-variety",
			"功能性限定占比应适当，避免过度依赖功能描述导致权利要求不清楚",
			"审查指南第二部分第二章§3.2.1")},
		&supportRangeEndpointRule{BaseRule: rulekit.NewBaseRule("support-range-endpoint",
			"数值范围限定的独立权利要求应得到说明书端点附近实施例的支持",
			"专利法第26条第4款；审查指南第二部分第二章§2.2.6")},
	)

	// 必要技术特征与单一性规则
	engine.RegisterAll(
		&necessityCompletenessRule{BaseRule: rulekit.NewBaseRule("necessity-completeness",
			"独立权利要求应当记载解决技术问题的全部必要技术特征",
			"专利法实施细则第21条第2款")},
		&necessityNonEssentialRule{BaseRule: rulekit.NewBaseRule("necessity-non-essential",
			"独立权利要求不应包含非必要技术特征，以免导致保护范围过窄",
			"专利法第26条第4款")},
		&unityInventionRule{BaseRule: rulekit.NewBaseRule("unity-invention",
			"多个独立权利要求之间应当满足单一性要求，包含相同或相应的特定技术特征",
			"专利法第31条第1款")},
	)

	// 保护范围规则
	engine.RegisterAll(
		&scopeOverSpecificationRule{BaseRule: rulekit.NewBaseRule("scope-over-specification",
			"独立权利要求中不宜使用过度具体的下位概念，应尽可能使用上位概念以拓宽保护范围",
			"专利法第26条第4款")},
		&scopeEquivalentsCoverageRule{BaseRule: rulekit.NewBaseRule("scope-equivalents-coverage",
			"从属权利要求应为等同替换预留空间，通过多层次布局覆盖替代方案",
			"审查指南第二部分第二章§3.3")},
		&scopePyramidRule{BaseRule: rulekit.NewBaseRule("scope-pyramid",
			"从属权利要求应形成从宽到窄的金字塔型多层次保护",
			"审查指南第二部分第二章§3.3")},
	)

	// 领域特定规则
	engine.RegisterAll(
		&domainMechanicalRule{BaseRule: rulekit.NewBaseRule("domain-mechanical",
			"机械领域产品独立权利要求应包含：零部件、配置关系、联系形式",
			"审查指南第二部分第二章")},
		&domainElectricalRule{BaseRule: rulekit.NewBaseRule("domain-electrical",
			"电路领域产品独立权利要求应包含：元器件、连接关系、电回路、功能描述",
			"审查指南第二部分第二章")},
		&domainChemicalRule{BaseRule: rulekit.NewBaseRule("domain-chemical",
			"化学组合物独立权利要求应包含组分及含量，含量之和应为100%",
			"审查指南第二部分第十章")},
		&domainSoftwareRule{BaseRule: rulekit.NewBaseRule("domain-software",
			"计算机程序发明可写为方法权利要求或产品（功能模块）权利要求",
			"审查指南第二部分第九章§5.2")},
		&domainUtilityModelRule{BaseRule: rulekit.NewBaseRule("domain-utility-model",
			"实用新型专利只能有产品权利要求，不能有方法权利要求",
			"专利法第2条第3款；审查指南第一部分第二章§6.1")},
		&domainMethodToProductRule{BaseRule: rulekit.NewBaseRule("domain-method-to-product",
			"软件领域可将方法权利要求同时表达为装置权利要求（用步骤限定装置）",
			"审查指南第二部分第九章§5.2")},
	)
}

// =============================================================================
// 词表（不确定用语 / 禁止用词，领域特定，检查委托 rulekit.ContainsAny）
// =============================================================================

// uncertainWords 不确定用语（模糊/相对性词汇）。
var uncertainWords = []string{
	"约", "大约", "左右", "接近",
	"厚", "薄", "宽", "窄", "强", "弱",
	"高温", "高压", "低温", "低压",
	"很宽范围", "合适的", "一定的",
}

// forbiddenWords 禁止使用的非限定性用语。
var forbiddenWords = []string{
	"例如", "最好是", "最好",
	"尤其是", "必要时",
	"等", "或类似物",
}
