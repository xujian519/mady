// Package fees 按随包费用索引为一件中国专利案件计价。
//
// 计价只回答「该案应缴哪些费种、按什么基准计数」，金额是否给出取决于索引是否
// 转录了该条目的金额。金额未转录、减缴比例未转录或缺少计数输入时，本包拒绝给出
// 合计，而不是按 0 或估算值凑数。
package fees

import (
	"fmt"
	"slices"

	"github.com/xujian519/mady/pkg/indexasset"
)

// PatentType 是专利类型。
type PatentType string

const (
	// PatentInvention 表示发明专利。
	PatentInvention PatentType = "invention"
	// PatentUtilityModel 表示实用新型专利。
	PatentUtilityModel PatentType = "utility-model"
	// PatentDesign 表示外观设计专利。
	PatentDesign PatentType = "design"
)

// Trigger 是收费阶段。
type Trigger string

const (
	// TriggerFiling 表示申请阶段。
	TriggerFiling Trigger = "filing"
	// TriggerSubstantiveExamination 表示实质审查请求。
	TriggerSubstantiveExamination Trigger = "substantive-examination"
	// TriggerAnnualFee 表示年费。
	TriggerAnnualFee Trigger = "annual-fee"
	// TriggerReexamination 表示复审请求。
	TriggerReexamination Trigger = "reexamination"
	// TriggerInvalidation 表示无效宣告请求。
	TriggerInvalidation Trigger = "invalidation"
	// TriggerEvaluationReport 表示专利权评价报告请求。
	TriggerEvaluationReport Trigger = "evaluation-report"
	// TriggerRestoration 表示恢复权利请求。
	TriggerRestoration Trigger = "restoration"
	// TriggerExtension 表示延长期限请求。
	TriggerExtension Trigger = "extension"
	// TriggerRecordChange 表示著录项目变更。
	TriggerRecordChange Trigger = "record-change"
	// TriggerPCTGrace 表示宽限费。
	TriggerPCTGrace Trigger = "pct-grace"
)

// Basis 是计数基准。
type Basis string

const (
	// BasisPerCase 表示每件。
	BasisPerCase Basis = "per-case"
	// BasisPerClaimBeyond 表示超过免费基数的每项权利要求。
	BasisPerClaimBeyond Basis = "per-claim-beyond"
	// BasisPerPageBeyond 表示超过免费基数的每页说明书。
	BasisPerPageBeyond Basis = "per-page-beyond"
	// BasisPerPriority 表示每项优先权要求。
	BasisPerPriority Basis = "per-priority"
	// BasisPerAnnuityYear 表示每个专利年度。
	BasisPerAnnuityYear Basis = "per-annuity-year"
	// BasisPerMonth 表示每请求月数。
	BasisPerMonth Basis = "per-month"
)

var (
	validPatentTypes = []PatentType{PatentInvention, PatentUtilityModel, PatentDesign}
	validTriggers    = []Trigger{
		TriggerFiling, TriggerSubstantiveExamination, TriggerAnnualFee, TriggerReexamination,
		TriggerInvalidation, TriggerEvaluationReport, TriggerRestoration, TriggerExtension,
		TriggerRecordChange, TriggerPCTGrace,
	}
	validBases = []Basis{
		BasisPerCase, BasisPerClaimBeyond, BasisPerPageBeyond,
		BasisPerPriority, BasisPerAnnuityYear, BasisPerMonth,
	}
)

// Tier 是年费分档。
type Tier struct {
	// FromYear 是起始专利年度。
	FromYear int
	// ToYear 是结束专利年度。
	ToYear int
	// Amount 是该档年费（元）；nil 表示未转录。
	Amount *string
}

// LatePayment 是年费滞纳金规则。
type LatePayment struct {
	// MonthlyPercent 是每超期一个月的加收百分比（按当年全额年费）。
	MonthlyPercent int
	// MaxMonths 是可补缴的月数上限。
	MaxMonths int
	// LegalBasis 是该规则的法定依据。
	LegalBasis string
	// SourceDoc 是规则的来源文书。
	SourceDoc string
	// VerifiedOn 是核验日期。
	VerifiedOn *string
}

// Item 是费用索引中的一个费种。
type Item struct {
	// ID 是条目标识。
	ID string
	// Name 是费种名称。
	Name string
	// Trigger 是收费阶段。
	Trigger Trigger
	// PatentTypes 是适用对象；nil 表示不限。
	PatentTypes []PatentType
	// Basis 是计数基准。
	Basis Basis
	// FreeUnits 是免费基数。
	FreeUnits int
	// Amount 是单价（元）；nil 表示未转录。
	Amount *string
	// Reducible 标记是否属于费用减缴范围；nil 表示未登记。
	Reducible *bool
	// ReductionMaxYears 是年费减缴覆盖的最高专利年度。
	ReductionMaxYears int
	// Tiers 是年费分档。
	Tiers []Tier
	// LatePayment 是滞纳金规则。
	LatePayment *LatePayment
	// LegalBasis 是法定依据。
	LegalBasis string
	// SourceDoc 是金额来源文书。
	SourceDoc string
	// VerifiedOn 是核验日期。
	VerifiedOn *string
}

// Reduction 是费用减缴规则。
type Reduction struct {
	// Kind 是减缴类型（individual / enterprise）。
	Kind string
	// Label 是显示名。
	Label string
	// ReductionPercent 是减缴比例；nil 表示未转录。
	ReductionPercent *int
	// RequiresFiling 表示该减缴以事前办理费减备案为前提。
	RequiresFiling bool
	// SourceDoc 是来源文书。
	SourceDoc string
	// VerifiedOn 是核验日期。
	VerifiedOn *string
}

// Table 是费用索引。
type Table struct {
	// Document 是索引对应的官方文书。
	Document string
	// Revision 是版本。
	Revision string
	// Currency 是币种。
	Currency string
	// VerifiedOn 是整个索引的核验日期。
	VerifiedOn *string
	// Reductions 是减缴规则。
	Reductions []Reduction
	// Items 是费种条目。
	Items []Item
}

// LoadTable 从资产文本加载费用索引。
func LoadTable(data []byte, origin string) (*Table, error) {
	fail := indexasset.FailFor(origin)
	root, err := indexasset.ParseYAMLMapping(data, "费用索引", fail)
	if err != nil {
		return nil, err
	}
	if err := indexasset.RequireKnownFields(root,
		[]string{"document", "revision", "currency", "sourceDoc", "effectiveFrom", "verifiedOn", "reductions", "items"},
		"费用索引根节点", fail); err != nil {
		return nil, err
	}
	table := &Table{}
	if table.Document, err = indexasset.ReadString(root, "document", fail); err != nil {
		return nil, err
	}
	if table.Revision, err = indexasset.ReadString(root, "revision", fail); err != nil {
		return nil, err
	}
	if table.Currency, err = indexasset.ReadString(root, "currency", fail); err != nil {
		return nil, err
	}
	if table.VerifiedOn, err = indexasset.ReadOptionalDate(root, "verifiedOn", fail); err != nil {
		return nil, err
	}
	if table.Reductions, err = readReductions(root["reductions"], fail); err != nil {
		return nil, err
	}
	if table.Items, err = readItems(root["items"], fail); err != nil {
		return nil, err
	}
	if len(table.Items) == 0 {
		return nil, fail("费用索引必须至少登记一个费种")
	}
	return table, nil
}

// readReductions 读取减缴规则。
func readReductions(value any, fail indexasset.Fail) ([]Reduction, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fail("字段 reductions 必须是数组")
	}
	reductions := make([]Reduction, 0, len(items))
	for _, raw := range items {
		entry, err := indexasset.ReadMapping(raw, "reductions 的每一项", fail)
		if err != nil {
			return nil, err
		}
		if err := indexasset.RequireKnownFields(entry,
			[]string{"kind", "label", "reductionPercent", "requiresFiling", "sourceDoc", "effectiveFrom", "verifiedOn"},
			"减缴条目", fail); err != nil {
			return nil, err
		}
		reduction := Reduction{}
		if reduction.Kind, err = indexasset.ReadString(entry, "kind", fail); err != nil {
			return nil, err
		}
		label, err := indexasset.ReadOptionalString(entry, "label", fail)
		if err != nil {
			return nil, err
		}
		if label != nil {
			reduction.Label = *label
		} else {
			reduction.Label = reduction.Kind
		}
		percent, err := indexasset.ReadOptionalCount(entry, "reductionPercent", fail)
		if err != nil {
			return nil, err
		}
		if percent != nil {
			reduction.ReductionPercent = percent
		}
		filed, err := indexasset.ReadOptionalBoolean(entry, "requiresFiling", fail)
		if err != nil {
			return nil, err
		}
		if filed != nil {
			reduction.RequiresFiling = *filed
		}
		if reduction.SourceDoc, err = readOptionalStringValue(entry, "sourceDoc", fail); err != nil {
			return nil, err
		}
		if reduction.VerifiedOn, err = indexasset.ReadOptionalDate(entry, "verifiedOn", fail); err != nil {
			return nil, err
		}
		reductions = append(reductions, reduction)
	}
	return reductions, nil
}

// readItems 读取费种条目。
func readItems(value any, fail indexasset.Fail) ([]Item, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, fail("字段 items 必须是数组")
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]Item, 0, len(items))
	for _, raw := range items {
		entry, err := indexasset.ReadMapping(raw, "items 的每一项", fail)
		if err != nil {
			return nil, err
		}
		if err := indexasset.RequireKnownFields(entry,
			[]string{"id", "name", "trigger", "patentTypes", "basis", "freeUnits", "amount", "reducible",
				"reductionMaxYears", "tiers", "latePayment", "legalBasis", "sourceDoc", "effectiveFrom", "verifiedOn"},
			"费种条目", fail); err != nil {
			return nil, err
		}
		item, err := readItem(entry, fail)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[item.ID]; dup {
			return nil, fail(fmt.Sprintf("费种 id 重复：%s", item.ID))
		}
		seen[item.ID] = struct{}{}
		result = append(result, item)
	}
	return result, nil
}

// readItem 读取单个费种。
func readItem(entry map[string]any, fail indexasset.Fail) (Item, error) {
	item := Item{}
	var err error
	if item.ID, err = indexasset.ReadString(entry, "id", fail); err != nil {
		return Item{}, err
	}
	if item.Name, err = indexasset.ReadString(entry, "name", fail); err != nil {
		return Item{}, err
	}
	trigger, err := indexasset.ReadEnum(entry["trigger"], triggerStrings(), "字段 trigger", fail)
	if err != nil {
		return Item{}, err
	}
	item.Trigger = Trigger(trigger)
	basis, err := indexasset.ReadEnum(entry["basis"], basisStrings(), "字段 basis", fail)
	if err != nil {
		return Item{}, err
	}
	item.Basis = Basis(basis)

	if item.PatentTypes, err = readPatentTypes(entry["patentTypes"], fail); err != nil {
		return Item{}, err
	}
	if freeUnits, err := indexasset.ReadOptionalCount(entry, "freeUnits", fail); err != nil {
		return Item{}, err
	} else if freeUnits != nil {
		item.FreeUnits = *freeUnits
	}
	if item.Amount, err = indexasset.ReadOptionalString(entry, "amount", fail); err != nil {
		return Item{}, err
	}
	if item.Reducible, err = indexasset.ReadOptionalBoolean(entry, "reducible", fail); err != nil {
		return Item{}, err
	}
	if maxYears, err := indexasset.ReadOptionalCount(entry, "reductionMaxYears", fail); err != nil {
		return Item{}, err
	} else if maxYears != nil {
		item.ReductionMaxYears = *maxYears
	}
	if item.Tiers, err = readTiers(entry["tiers"], item.ID, fail); err != nil {
		return Item{}, err
	}
	if item.LatePayment, err = readLatePayment(entry["latePayment"], item.ID, fail); err != nil {
		return Item{}, err
	}
	if item.LegalBasis, err = readOptionalStringValue(entry, "legalBasis", fail); err != nil {
		return Item{}, err
	}
	if item.SourceDoc, err = readOptionalStringValue(entry, "sourceDoc", fail); err != nil {
		return Item{}, err
	}
	if item.VerifiedOn, err = indexasset.ReadOptionalDate(entry, "verifiedOn", fail); err != nil {
		return Item{}, err
	}

	// 需要计数的基准必须能拿到单价或分档，否则该条目只能报「未转录」。
	if item.Basis == BasisPerAnnuityYear && len(item.Tiers) == 0 && item.Amount == nil {
		return Item{}, fail(fmt.Sprintf("年费条目 %s 既无单价也无分档", item.ID))
	}
	return item, nil
}

// readPatentTypes 读取适用专利类型；null 表示不限。
func readPatentTypes(value any, fail indexasset.Fail) ([]PatentType, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil, fail("字段 patentTypes 必须是 null 或非空数组")
	}
	types := make([]PatentType, 0, len(items))
	for _, raw := range items {
		name, err := indexasset.ReadEnum(raw, patentTypeStrings(), "patentTypes", fail)
		if err != nil {
			return nil, err
		}
		types = append(types, PatentType(name))
	}
	return types, nil
}

// readTiers 读取年费分档。
func readTiers(value any, itemID string, fail indexasset.Fail) ([]Tier, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil, fail(fmt.Sprintf("费种 %s 的 tiers 必须是 null 或非空数组", itemID))
	}
	tiers := make([]Tier, 0, len(items))
	for _, raw := range items {
		entry, err := indexasset.ReadMapping(raw, "tiers 的每一项", fail)
		if err != nil {
			return nil, err
		}
		if err := indexasset.RequireKnownFields(entry, []string{"fromYear", "toYear", "amount"}, "分档条目", fail); err != nil {
			return nil, err
		}
		from, err := indexasset.ReadOptionalCount(entry, "fromYear", fail)
		if err != nil {
			return nil, err
		}
		to, err := indexasset.ReadOptionalCount(entry, "toYear", fail)
		if err != nil {
			return nil, err
		}
		if from == nil || to == nil || *to < *from {
			return nil, fail(fmt.Sprintf("费种 %s 的分档年度区间无效", itemID))
		}
		amount, err := indexasset.ReadOptionalString(entry, "amount", fail)
		if err != nil {
			return nil, err
		}
		tiers = append(tiers, Tier{FromYear: *from, ToYear: *to, Amount: amount})
	}
	return tiers, nil
}

// readLatePayment 读取滞纳金规则。
func readLatePayment(value any, itemID string, fail indexasset.Fail) (*LatePayment, error) {
	if value == nil {
		return nil, nil
	}
	entry, err := indexasset.ReadMapping(value, "latePayment", fail)
	if err != nil {
		return nil, err
	}
	if err := indexasset.RequireKnownFields(entry,
		[]string{"monthlyPercent", "maxMonths", "legalBasis", "sourceDoc", "effectiveFrom", "verifiedOn"},
		"滞纳金规则", fail); err != nil {
		return nil, err
	}
	rule := &LatePayment{}
	percent, err := indexasset.ReadOptionalCount(entry, "monthlyPercent", fail)
	if err != nil {
		return nil, err
	}
	if percent == nil {
		return nil, fail(fmt.Sprintf("费种 %s 的滞纳金规则缺少 monthlyPercent", itemID))
	}
	rule.MonthlyPercent = *percent
	months, err := indexasset.ReadOptionalCount(entry, "maxMonths", fail)
	if err != nil {
		return nil, err
	}
	if months == nil {
		return nil, fail(fmt.Sprintf("费种 %s 的滞纳金规则缺少 maxMonths", itemID))
	}
	rule.MaxMonths = *months
	if rule.LegalBasis, err = readOptionalStringValue(entry, "legalBasis", fail); err != nil {
		return nil, err
	}
	if rule.SourceDoc, err = readOptionalStringValue(entry, "sourceDoc", fail); err != nil {
		return nil, err
	}
	if rule.VerifiedOn, err = indexasset.ReadOptionalDate(entry, "verifiedOn", fail); err != nil {
		return nil, err
	}
	return rule, nil
}

// readOptionalStringValue 读取可缺省字符串，nil 时返回空串。
func readOptionalStringValue(root map[string]any, field string, fail indexasset.Fail) (string, error) {
	value, err := indexasset.ReadOptionalString(root, field, fail)
	if err != nil || value == nil {
		return "", err
	}
	return *value, nil
}

// patentTypeStrings 返回合法的专利类型字符串。
func patentTypeStrings() []string {
	out := make([]string, len(validPatentTypes))
	for i, value := range validPatentTypes {
		out[i] = string(value)
	}
	return out
}

// triggerStrings 返回合法的收费阶段字符串。
func triggerStrings() []string {
	out := make([]string, len(validTriggers))
	for i, value := range validTriggers {
		out[i] = string(value)
	}
	return out
}

// basisStrings 返回合法的计数基准字符串。
func basisStrings() []string {
	out := make([]string, len(validBases))
	for i, value := range validBases {
		out[i] = string(value)
	}
	return out
}

// supportsType 判断费种是否适用于某专利类型。
func (item Item) supportsType(patentType PatentType) bool {
	if item.PatentTypes == nil {
		return true
	}
	return slices.Contains(item.PatentTypes, patentType)
}
