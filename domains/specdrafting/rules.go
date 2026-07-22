package specdrafting

import "sync"

// =============================================================================
// SpecRule 规则接口
// =============================================================================

// SpecRule 是说明书验证规则接口。
type SpecRule interface {
	Name() string
	Description() string
	LegalBasis() string
	Check(spec *SpecOutput, input SpecInput) []Violation
}

// =============================================================================
// RuleEngine 规则引擎
// =============================================================================

// RuleEngine 管理一组验证规则，提供批量验证能力。
type RuleEngine struct {
	mu    sync.RWMutex
	rules []SpecRule
}

// NewRuleEngine 创建一个空的规则引擎。
func NewRuleEngine() *RuleEngine {
	return &RuleEngine{rules: make([]SpecRule, 0)}
}

// Register 注册一条规则（线程安全）。
func (e *RuleEngine) Register(rule SpecRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rule)
}

// RegisterAll 批量注册规则。
func (e *RuleEngine) RegisterAll(rules ...SpecRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rules...)
}

// Rules 返回当前注册的所有规则。
func (e *RuleEngine) Rules() []SpecRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	cp := make([]SpecRule, len(e.rules))
	copy(cp, e.rules)
	return cp
}

// Validate 执行所有注册规则的检查，返回违规列表。
func (e *RuleEngine) Validate(spec *SpecOutput, input SpecInput) []Violation {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var all []Violation
	for _, rule := range e.rules {
		all = append(all, rule.Check(spec, input)...)
	}
	return all
}

// ValidateAndGroup 执行检查并按严重程度分组。
func (e *RuleEngine) ValidateAndGroup(spec *SpecOutput, input SpecInput) (errors, warnings, infos []Violation) {
	for _, v := range e.Validate(spec, input) {
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
// 基础规则
// =============================================================================

type baseRule struct {
	name        string
	description string
	legalBasis  string
}

func (r *baseRule) Name() string        { return r.name }
func (r *baseRule) Description() string { return r.description }
func (r *baseRule) LegalBasis() string  { return r.legalBasis }

func newBaseRule(name, desc, basis string) baseRule {
	return baseRule{name: name, description: desc, legalBasis: basis}
}

// =============================================================================
// 默认规则注册
// =============================================================================

// RegisterDefaultRules 注册所有默认说明书验证规则到引擎。
func RegisterDefaultRules(engine *RuleEngine) {
	engine.RegisterAll(
		&structureSectionsRule{baseRule: newBaseRule("structure-sections",
			"说明书必须包含五项必要章节", "专利法实施细则第18条")},
		&structureTitleLengthRule{baseRule: newBaseRule("structure-title-length",
			"发明名称不得超过25个字", "专利法实施细则第17条第1款")},
		&structureAbstractLengthRule{baseRule: newBaseRule("structure-abstract-length",
			"说明书摘要不超过300字", "专利法实施细则第23条第2款")},
		&structureContentTriadRule{baseRule: newBaseRule("structure-content-triad",
			"发明内容须包含问题+方案+效果三要素", "专利法实施细则第18条第1款第(三)项")},
		&structureEmbodimentDetailRule{baseRule: newBaseRule("structure-embodiment-detail",
			"具体实施方式应至少给出一个详细实施例", "专利法第26条第3款")},

		&clarityTerminologyRule{baseRule: newBaseRule("clarity-terminology",
			"应使用清楚的技术术语", "专利法第26条第3款")},
		&clarityForbiddenWordsRule{baseRule: newBaseRule("clarity-forbidden-words",
			"不得使用禁止用词和不确定用语", "审查指南第二部分第二章§2.1.1")},
		&clarityPFEConsistencyRule{baseRule: newBaseRule("clarity-pfe-consistency",
			"问题、方案、效果三者应相互适应", "专利法第26条第3款")},
		&clarityTermConsistencyRule{baseRule: newBaseRule("clarity-term-consistency",
			"术语全文应保持一致", "专利法第26条第3款")},

		&domainMechanicalRule{baseRule: newBaseRule("domain-mechanical",
			"机械领域应描述零部件及其配置关系", "审查指南第二部分第二章")},
		&domainElectricalRule{baseRule: newBaseRule("domain-electrical",
			"电学领域应描述元器件、连接关系和功能", "审查指南第二部分第二章")},
		&domainChemicalRule{baseRule: newBaseRule("domain-chemical",
			"化学领域应公开组分含量及实验数据", "审查指南第二部分第十章")},
		&domainSoftwareRule{baseRule: newBaseRule("domain-software",
			"软件领域应描述方法步骤或功能模块", "审查指南第二部分第九章§5.2")},

		&utilityDrawingsRequiredRule{baseRule: newBaseRule("utility-drawings-required",
			"实用新型必须有附图", "专利法实施细则第39条")},
		&utilityProductOnlyRule{baseRule: newBaseRule("utility-product-only",
			"实用新型仅保护产品形状/构造", "专利法第2条第3款")},
		&utilitySingleIndependentRule{baseRule: newBaseRule("utility-single-independent",
			"实用新型应只有一个独立权利要求", "专利法实施细则第21条第1款")},
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

func containsAny(s string, words []string) (string, bool) {
	for _, w := range words {
		if containsStr(s, w) {
			return w, true
		}
	}
	return "", false
}

func containsStr(s, substr string) bool {
	return indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
