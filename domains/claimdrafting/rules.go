package claimdrafting

import (
	"strings"
	"sync"
)

// =============================================================================
// ClaimRule 规则接口
// =============================================================================

// ClaimRule 是权利要求验证规则接口。
// 所有规则实现此接口，通过 RuleEngine 注册和执行。
type ClaimRule interface {
	// Name 返回规则唯一标识符（kebab-case，如 "clarity-wording"）。
	Name() string
	// Description 返回规则的人类可读描述。
	Description() string
	// LegalBasis 返回法律依据（如"专利法第26条第4款"）。
	LegalBasis() string
	// Check 检查一组权利要求，返回违规列表。
	// input 提供上下文数据（如说明书摘要、技术特征等）供规则判断。
	Check(claims []Claim, input DraftInput) []Violation
}

// =============================================================================
// RuleEngine 规则引擎
// =============================================================================

// RuleEngine 管理一组验证规则，提供批量验证能力。
type RuleEngine struct {
	mu    sync.RWMutex
	rules []ClaimRule
}

// NewRuleEngine 创建一个空的规则引擎。
func NewRuleEngine() *RuleEngine {
	return &RuleEngine{
		rules: make([]ClaimRule, 0),
	}
}

// Register 注册一条规则（线程安全）。
func (e *RuleEngine) Register(rule ClaimRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rule)
}

// RegisterAll 批量注册规则（线程安全）。
func (e *RuleEngine) RegisterAll(rules ...ClaimRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rules...)
}

// Rules 返回当前注册的所有规则。
func (e *RuleEngine) Rules() []ClaimRule {
	cp := make([]ClaimRule, len(e.rules))
	copy(cp, e.rules)
	return cp
}

// Validate 执行所有注册规则的检查，返回违规列表。
func (e *RuleEngine) Validate(claims []Claim, input DraftInput) []Violation {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var all []Violation
	for _, rule := range e.rules {
		violations := rule.Check(claims, input)
		all = append(all, violations...)
	}
	return all
}

// ValidateAndGroup 执行检查并按严重程度分组。
func (e *RuleEngine) ValidateAndGroup(claims []Claim, input DraftInput) (errors, warnings, infos []Violation) {
	all := e.Validate(claims, input)
	for _, v := range all {
		switch v.Severity {
		case SeverityError:
			errors = append(errors, v)
		case SeverityWarning:
			warnings = append(warnings, v)
		case SeverityInfo:
			infos = append(infos, v)
		}
	}
	return
}

// =============================================================================
// 基础规则实现（公共功能）
// =============================================================================

// baseRule 提供 ClaimRule 实现的公用字段。
type baseRule struct {
	name        string
	description string
	legalBasis  string
}

func (r *baseRule) Name() string        { return r.name }
func (r *baseRule) Description() string { return r.description }
func (r *baseRule) LegalBasis() string  { return r.legalBasis }

// newBaseRule 创建基础规则。
func newBaseRule(name, desc, basis string) baseRule {
	return baseRule{name: name, description: desc, legalBasis: basis}
}

// =============================================================================
// 规则注册辅助函数（集中注册所有规则到引擎）
// =============================================================================

// RegisterDefaultRules 注册所有默认验证规则到引擎。
func RegisterDefaultRules(engine *RuleEngine) {
	// 清楚性规则
	engine.RegisterAll(
		&clarityClaimTypeRule{baseRule: newBaseRule("clarity-claim-type",
			"权利要求的类型应当清楚：必须明确是产品权利要求还是方法权利要求，不允许混合类型",
			"专利法实施细则第20条第2款")},
		&clarityWordingRule{baseRule: newBaseRule("clarity-wording",
			"不得使用含义不确定的用语（如'约'、'大约'、'厚'、'薄'、'高温'、'高压'等）",
			"专利法第26条第4款")},
		&clarityForbiddenWordsRule{baseRule: newBaseRule("clarity-forbidden-words",
			"不得使用'例如'、'最好是'、'尤其是'、'必要时'等导致保护范围不清楚的用语",
			"专利法第26条第4款")},
		&clarityReferenceRule{baseRule: newBaseRule("clarity-reference",
			"从属权利要求的引用关系应当清楚：多项从属只能择一引用（用'或'），不得用'和'",
			"专利法实施细则第23条第2款")},
		&clarityReferenceChainRule{baseRule: newBaseRule("clarity-reference-chain",
			"引用关系不得形成循环依赖",
			"专利法第26条第4款")},
		&clarityAntecedentBasisRule{baseRule: newBaseRule("clarity-antecedent-basis",
			"从属权利要求中的术语应当在被引用的权利要求中有引用基础（先行词）",
			"专利法第26条第4款")},
	)

	// 形式规范规则
	engine.RegisterAll(
		&formalityNumberingRule{baseRule: newBaseRule("formality-numbering",
			"权利要求书有多项权利要求的，应当用阿拉伯数字顺序编号",
			"专利法实施细则第20条第1款")},
		&formalityPeriodRule{baseRule: newBaseRule("formality-period",
			"每一项权利要求只允许在其结尾处使用句号",
			"专利法实施细则第20条第4款")},
		&formalityNoIllustrationRule{baseRule: newBaseRule("formality-no-illustration",
			"权利要求书中不得有插图",
			"专利法实施细则第20条第3款")},
		&formalityMultipleDependentRule{baseRule: newBaseRule("formality-multiple-dependent",
			"多项从属权利要求不得作为另一项多项从属权利要求的基础",
			"专利法实施细则第23条第2款")},
		&formalityThemeConsistencyRule{baseRule: newBaseRule("formality-theme-consistency",
			"从属权利要求的类型和主题名称应当与其引用的权利要求一致",
			"专利法实施细则第22条第3款")},
		&formalityScopeNarrowingRule{baseRule: newBaseRule("formality-scope-narrowing",
			"从属权利要求的保护范围应当在其引用权利要求的保护范围之内",
			"专利法第26条第4款")},
		&formalityParallelClaimRule{baseRule: newBaseRule("formality-parallel-claim",
			"并列独立权利要求的引用关系应当合法，不得循环引用或引用自身",
			"审查指南(2010)第二部分第二章§3.3")},
	)

	// 支持性规则
	engine.RegisterAll(
		&supportEmbodimentRule{baseRule: newBaseRule("support-embodiment",
			"权利要求的概括应当得到说明书实施例的支持，不得超出说明书公开的范围",
			"专利法第26条第4款")},
		&supportFunctionalRule{baseRule: newBaseRule("support-functional",
			"功能性限定的使用应当恰当，以说明书中记载了具体的实现方式为前提",
			"审查指南第二部分第二章§3.2.1")},
		&supportPureFunctionalRule{baseRule: newBaseRule("support-pure-functional",
			"不得出现纯功能性权利要求（仅用功能描述整个技术方案）",
			"审查指南第二部分第二章§3.2.1")},
	)

	// 必要技术特征与单一性规则
	engine.RegisterAll(
		&necessityCompletenessRule{baseRule: newBaseRule("necessity-completeness",
			"独立权利要求应当记载解决技术问题的全部必要技术特征",
			"专利法实施细则第21条第2款")},
		&necessityNonEssentialRule{baseRule: newBaseRule("necessity-non-essential",
			"独立权利要求不应包含非必要技术特征，以免导致保护范围过窄",
			"专利法第26条第4款")},
		&unityInventionRule{baseRule: newBaseRule("unity-invention",
			"多个独立权利要求之间应当满足单一性要求，包含相同或相应的特定技术特征",
			"专利法第31条第1款")},
	)

	// 保护范围规则
	engine.RegisterAll(
		&scopeOverSpecificationRule{baseRule: newBaseRule("scope-over-specification",
			"独立权利要求中不宜使用过度具体的下位概念，应尽可能使用上位概念以拓宽保护范围",
			"专利法第26条第4款")},
		&scopeEquivalentsCoverageRule{baseRule: newBaseRule("scope-equivalents-coverage",
			"从属权利要求应为等同替换预留空间，通过多层次布局覆盖替代方案",
			"审查指南第二部分第二章§3.3")},
	)

	// 领域特定规则
	engine.RegisterAll(
		&domainMechanicalRule{baseRule: newBaseRule("domain-mechanical",
			"机械领域产品独立权利要求应包含：零部件、配置关系、联系形式",
			"审查指南第二部分第二章")},
		&domainElectricalRule{baseRule: newBaseRule("domain-electrical",
			"电路领域产品独立权利要求应包含：元器件、连接关系、电回路、功能描述",
			"审查指南第二部分第二章")},
		&domainChemicalRule{baseRule: newBaseRule("domain-chemical",
			"化学组合物独立权利要求应包含组分及含量，含量之和应为100%",
			"审查指南第二部分第十章")},
		&domainSoftwareRule{baseRule: newBaseRule("domain-software",
			"计算机程序发明可写为方法权利要求或产品（功能模块）权利要求",
			"审查指南第二部分第九章§5.2")},
		&domainUtilityModelRule{baseRule: newBaseRule("domain-utility-model",
			"实用新型专利只能有产品权利要求，不能有方法权利要求",
			"专利法第2条第3款；审查指南第一部分第二章§6.1")},
	)
}

// =============================================================================
// 字符串辅助
// =============================================================================

// containsUncertainWord 检查文本中是否包含不确定用语（模糊/相对性词汇）。
var uncertainWords = []string{
	"约", "大约", "左右", "接近",
	"厚", "薄", "宽", "窄", "强", "弱",
	"高温", "高压", "低温", "低压",
	"很宽范围", "合适的", "一定的",
}

func containsUncertainWord(s string) (string, bool) {
	for _, w := range uncertainWords {
		if strings.Contains(s, w) {
			return w, true
		}
	}
	return "", false
}

// containsForbiddenWord 检查文本中是否包含禁止使用的非限定性用语。
var forbiddenWords = []string{
	"例如", "最好是", "最好",
	"尤其是", "必要时",
	"等", "或类似物",
}

func containsForbiddenWord(s string) (string, bool) {
	for _, w := range forbiddenWords {
		if strings.Contains(s, w) {
			return w, true
		}
	}
	return "", false
}
