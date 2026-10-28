package fees

import (
	"fmt"
	"slices"
	"strings"
)

// AmountStatus 是单行金额的状态。
type AmountStatus string

const (
	// StatusVerified 表示金额已转录且有核验日期。
	StatusVerified AmountStatus = "verified"
	// StatusUnverified 表示金额已录入但未记录核验日期。
	StatusUnverified AmountStatus = "unverified"
	// StatusUnrecorded 表示金额未转录。
	StatusUnrecorded AmountStatus = "unrecorded"
)

// ReductionRequest 是案件请求的费用减缴。
type ReductionRequest struct {
	// Kind 是减缴类型（individual / enterprise）。
	Kind string
	// Filed 表示已办理费减备案。
	Filed bool
}

// Query 是一件案件的计费事实。
//
// 指针字段区分「未提供」与「提供 0」：未提供时该费种进入 Pending，
// 而不是按 0 计价。
type Query struct {
	// PatentType 是专利类型。
	PatentType PatentType
	// Triggers 是本次要计算的收费阶段。
	Triggers []Trigger
	// Claims 是权利要求总项数。
	Claims *int
	// SpecificationPages 是说明书总页数。
	SpecificationPages *int
	// PriorityClaims 是优先权要求项数。
	PriorityClaims *int
	// ExtensionMonths 是请求延长的月数。
	ExtensionMonths *int
	// AnnuityYears 是要计算年费的专利年度；空表示未提供。
	AnnuityYears []int
	// LateMonths 是年费超期月数。
	LateMonths *int
	// Reduction 是减缴请求。
	Reduction *ReductionRequest
}

// Line 是一条计价行。
type Line struct {
	// ID 是行标识。
	ID string
	// Name 是费用名称。
	Name string
	// Basis 是计数基准。
	Basis Basis
	// Quantity 是计价数量。
	Quantity int
	// QuantityBasis 是数量的来源说明。
	QuantityBasis string
	// UnitAmount 是单价（元）；nil 表示未转录。
	UnitAmount *string
	// Subtotal 是小计（元）；nil 表示无法计算。
	Subtotal *string
	// Payable 是减缴后应缴（元）；nil 表示无法计算。
	Payable *string
	// Status 是金额状态。
	Status AmountStatus
	// Reduction 是减缴处理结果。
	Reduction string
	// LegalBasis 是法定依据。
	LegalBasis string
	// SourceDoc 是金额来源。
	SourceDoc string
	// VerifiedOn 是核验日期。
	VerifiedOn *string
	// Notes 是该行的补充说明。
	Notes []string
}

// Pending 是需要案件输入才能计价的条目。
type Pending struct {
	// ID 是条目标识。
	ID string
	// Name 是费用名称。
	Name string
	// RequiredInput 是缺少的输入名。
	RequiredInput string
	// Reason 是面向用户的说明。
	Reason string
}

// Total 是合计结果。
type Total struct {
	// Amount 是合计（元）；nil 表示不给合计。
	Amount *string
	// Complete 表示所有适用行金额齐备且阶段均已收录。
	Complete bool
	// UnverifiedIDs 是导致合计不完整的行。
	UnverifiedIDs []string
	// UncoveredTriggers 是索引未收录的收费阶段。
	UncoveredTriggers []Trigger
	// Reason 是不给合计的原因。
	Reason string
}

// ReductionOutcome 是减缴请求的处理结果。
type ReductionOutcome struct {
	// Kind 是减缴类型。
	Kind string
	// Filed 表示已备案。
	Filed bool
	// ReductionPercent 是减缴比例；nil 表示未转录。
	ReductionPercent *int
	// Applied 表示至少一行实际应用了减缴。
	Applied bool
	// Reason 是处理说明。
	Reason string
}

// Report 是一次计价的完整结果。
type Report struct {
	// Currency 是币种。
	Currency string
	// Lines 是计价行。
	Lines []Line
	// Pending 是等待输入的条目。
	Pending []Pending
	// Reduction 是减缴处理结果。
	Reduction *ReductionOutcome
	// Total 是合计。
	Total Total
	// Notes 是整体说明。
	Notes []string
}

// computedLine 是带分金额的计价行。
type computedLine struct {
	line       Line
	payableFen *int64
}

// reductionContext 是解析后的减缴上下文。
type reductionContext struct {
	request *ReductionRequest
	rule    *Reduction
}

// Compute 按案件事实计价。
func (t *Table) Compute(query Query) (Report, error) {
	if err := validateQuery(query); err != nil {
		return Report{}, err
	}

	report := Report{Currency: t.Currency}
	reduction := t.resolveReduction(query.Reduction, &report.Notes)
	var lines []computedLine
	var pending []Pending
	var excludedByType []string

	for _, item := range t.Items {
		if !slices.Contains(query.Triggers, item.Trigger) {
			continue
		}
		if !item.supportsType(query.PatentType) {
			excludedByType = append(excludedByType, item.Name)
			continue
		}
		if item.Basis == BasisPerAnnuityYear {
			annuity, annuityPending := t.annuityLines(item, query, reduction)
			lines = append(lines, annuity...)
			pending = append(pending, annuityPending...)
			continue
		}
		counted, missing := countUnits(item, query)
		if missing != nil {
			pending = append(pending, *missing)
			continue
		}
		lines = append(lines, t.pricedLine(item, item.ID, item.Name, counted.quantity, counted.basis, item.Amount, reduction, 0))
	}

	if len(excludedByType) > 0 {
		report.Notes = append(report.Notes, fmt.Sprintf("已按专利类型过滤：%s 不适用本次类型，未列入本报告。", strings.Join(excludedByType, "、")))
	}

	uncovered := uncoveredTriggers(t, query.Triggers)
	if len(uncovered) > 0 {
		names := make([]string, len(uncovered))
		for i, trigger := range uncovered {
			names[i] = string(trigger)
		}
		report.Notes = append(report.Notes, fmt.Sprintf("索引未收录以下收费阶段：%s；本次合计不予给出。", strings.Join(names, "、")))
	}

	report.Lines = make([]Line, len(lines))
	for i := range lines {
		report.Lines[i] = lines[i].line
	}
	report.Pending = pending
	report.Total = totalOf(lines, uncovered, len(pending))
	report.Reduction = reductionOutcome(reduction, lines)
	return report, nil
}

// validateQuery 拒绝不在枚举内的输入。
func validateQuery(query Query) error {
	if !slices.Contains(validPatentTypes, query.PatentType) {
		return fmt.Errorf("fees: 未知专利类型 %q", query.PatentType)
	}
	if len(query.Triggers) == 0 {
		return fmt.Errorf("fees: 至少需要一个收费阶段")
	}
	for _, trigger := range query.Triggers {
		if !slices.Contains(validTriggers, trigger) {
			return fmt.Errorf("fees: 未知收费阶段 %q", trigger)
		}
	}
	if query.LateMonths != nil && *query.LateMonths < 0 {
		return fmt.Errorf("fees: 超期月数不能为负")
	}
	for _, year := range query.AnnuityYears {
		if year < 1 {
			return fmt.Errorf("fees: 专利年度必须为正整数，得到 %d", year)
		}
	}
	return nil
}

// resolveReduction 把减缴请求对照索引解析。
func (t *Table) resolveReduction(request *ReductionRequest, notes *[]string) *reductionContext {
	if request == nil {
		return nil
	}
	context := &reductionContext{request: request}
	for i := range t.Reductions {
		if t.Reductions[i].Kind == request.Kind {
			context.rule = &t.Reductions[i]
			break
		}
	}
	if context.rule == nil {
		*notes = append(*notes, fmt.Sprintf("费用索引没有登记「%s」减缴：本次不按减缴计算。", request.Kind))
	}
	if !request.Filed {
		*notes = append(*notes, "本次减缴请求未办理费减备案。")
	}
	return context
}

// counted 是计数结果。
type counted struct {
	quantity int
	basis    string
}

// countUnits 按基准计数，或报告缺少的案件输入。
func countUnits(item Item, query Query) (counted, *Pending) {
	freeUnits := item.FreeUnits
	switch item.Basis {
	case BasisPerCase:
		return counted{1, "每件"}, nil
	case BasisPerClaimBeyond:
		if query.Claims == nil {
			return counted{}, missingInput(item, "claims", "未给出权利要求总项数。")
		}
		return counted{
			quantity: max(0, *query.Claims-freeUnits),
			basis:    fmt.Sprintf("权利要求 %d 项 − 免费基数 %d 项", *query.Claims, freeUnits),
		}, nil
	case BasisPerPageBeyond:
		if query.SpecificationPages == nil {
			return counted{}, missingInput(item, "specificationPages", "未给出说明书总页数。")
		}
		return counted{
			quantity: max(0, *query.SpecificationPages-freeUnits),
			basis:    fmt.Sprintf("说明书 %d 页 − 免费基数 %d 页", *query.SpecificationPages, freeUnits),
		}, nil
	case BasisPerPriority:
		if query.PriorityClaims == nil {
			return counted{}, missingInput(item, "priorityClaims", "未给出优先权要求项数。")
		}
		return counted{quantity: *query.PriorityClaims, basis: fmt.Sprintf("%d 项优先权要求", *query.PriorityClaims)}, nil
	case BasisPerMonth:
		if query.ExtensionMonths == nil {
			return counted{}, missingInput(item, "extensionMonths", "未给出请求延长的月数。")
		}
		return counted{quantity: *query.ExtensionMonths, basis: fmt.Sprintf("%d 个月", *query.ExtensionMonths)}, nil
	default:
		return counted{}, missingInput(item, "basis", fmt.Sprintf("未知计数基准 %q。", item.Basis))
	}
}

// missingInput 构造一条等待输入的条目。
func missingInput(item Item, requiredInput, reason string) *Pending {
	return &Pending{ID: item.ID, Name: item.Name, RequiredInput: requiredInput, Reason: reason}
}

// pricedLine 按数量与单价计价。
func (t *Table) pricedLine(
	item Item,
	id, name string,
	quantity int,
	quantityBasis string,
	amount *string,
	reduction *reductionContext,
	year int,
) computedLine {
	notes := make([]string, 0, 2)
	unitFen, subtotalFen := lineAmounts(amount, quantity)
	payableFen, reductionLabel, reductionNotes := t.applyReduction(item, subtotalFen, reduction, year)
	notes = append(notes, reductionNotes...)

	line := Line{
		ID:            id,
		Name:          name,
		Basis:         item.Basis,
		Quantity:      quantity,
		QuantityBasis: quantityBasis,
		UnitAmount:    amount,
		Status:        statusOf(amount != nil, item.VerifiedOn != nil),
		Reduction:     reductionLabel,
		LegalBasis:    item.LegalBasis,
		SourceDoc:     item.SourceDoc,
		VerifiedOn:    item.VerifiedOn,
		Notes:         notes,
	}
	line.Subtotal = fenString(subtotalFen)
	line.Payable = fenString(payableFen)
	_ = unitFen
	return computedLine{line: line, payableFen: payableFen}
}

// lineAmounts 计算单价与小计。
func lineAmounts(amount *string, quantity int) (unitFen, subtotalFen *int64) {
	if amount == nil {
		return nil, nil
	}
	fen, err := ParseYuan(*amount)
	if err != nil {
		return nil, nil
	}
	subtotal := fen * int64(quantity)
	return &fen, &subtotal
}

// fenString 把分金额格式化为元；nil 返回 nil。
func fenString(fen *int64) *string {
	if fen == nil {
		return nil
	}
	text := FormatYuan(*fen)
	return &text
}

// statusOf 返回金额状态。
func statusOf(recorded, verified bool) AmountStatus {
	if !recorded {
		return StatusUnrecorded
	}
	if verified {
		return StatusVerified
	}
	return StatusUnverified
}

// applyReduction 对一行小计应用减缴。
func (t *Table) applyReduction(item Item, subtotalFen *int64, reduction *reductionContext, year int) (*int64, string, []string) {
	if reduction == nil {
		return subtotalFen, "", nil
	}
	if item.Reducible != nil && !*item.Reducible {
		return subtotalFen, "未减缴（不适用）", []string{"本项不属于费用减缴范围：按全额计。"}
	}
	if item.Reducible == nil {
		return subtotalFen, "未减缴（适用范围未登记）", []string{"减缴适用范围未登记：本项按全额计。"}
	}
	if year > 0 && item.ReductionMaxYears > 0 && year > item.ReductionMaxYears {
		return subtotalFen,
			fmt.Sprintf("未减缴（仅前 %d 年适用）", item.ReductionMaxYears),
			[]string{fmt.Sprintf("减缴只覆盖第 %d 年度及以前：本年度按全额计。", item.ReductionMaxYears)}
	}
	if reduction.rule == nil {
		return nil, "减缴未登记", []string{"费用索引没有登记该减缴：无法给出减缴后金额。"}
	}
	if reduction.rule.ReductionPercent == nil {
		return nil, "减缴比例未转录", []string{"减缴比例未转录：无法给出减缴后金额。"}
	}
	if reduction.rule.RequiresFiling && !reduction.request.Filed {
		return subtotalFen, "未减缴（未备案）", []string{"未办理费减备案：不得按减缴计算，本项按全额计。"}
	}
	label := fmt.Sprintf("减缴 %d%%", *reduction.rule.ReductionPercent)
	if subtotalFen == nil {
		return nil, label, nil
	}
	payable := ApplyPercent(*subtotalFen, 100-*reduction.rule.ReductionPercent)
	return &payable, label, nil
}

// annuityLines 为每个请求的专利年度计价，并补滞纳金行。
func (t *Table) annuityLines(item Item, query Query, reduction *reductionContext) ([]computedLine, []Pending) {
	if len(query.AnnuityYears) == 0 {
		return nil, []Pending{{
			ID:            item.ID,
			Name:          item.Name,
			RequiredInput: "annuityYears",
			Reason:        "未给出要计算年费的专利年度：年度序号由期限计算给出，本工具不自行推算。",
		}}
	}
	years := slices.Clone(query.AnnuityYears)
	slices.Sort(years)
	years = slices.Compact(years)

	lines := make([]computedLine, 0, len(years)*2)
	for _, year := range years {
		amount := annualFeeAmount(item, year)
		line := t.pricedLine(item, fmt.Sprintf("%s-%d", item.ID, year),
			fmt.Sprintf("%s（第 %d 年度）", item.Name, year), 1,
			fmt.Sprintf("第 %d 年度", year), amount, reduction, year)
		if len(item.Tiers) > 0 && amount == nil {
			line.line.Notes = append(line.line.Notes,
				fmt.Sprintf("第 %d 年度未落在已转录的分档内（已转录 %d 档）。", year, len(item.Tiers)))
		}
		lines = append(lines, line)
		if late, ok := t.latePaymentLine(item, year, query.LateMonths, amount); ok {
			lines = append(lines, late)
		}
	}
	return lines, nil
}

// annualFeeAmount 返回某年度的年费单价。
func annualFeeAmount(item Item, year int) *string {
	for _, tier := range item.Tiers {
		if year >= tier.FromYear && year <= tier.ToYear {
			return tier.Amount
		}
	}
	if len(item.Tiers) == 0 {
		return item.Amount
	}
	return nil
}

// latePaymentLine 计算某个年度的滞纳金。
func (t *Table) latePaymentLine(item Item, year int, lateMonths *int, annualAmount *string) (computedLine, bool) {
	rule := item.LatePayment
	if rule == nil || lateMonths == nil || *lateMonths <= 0 {
		return computedLine{}, false
	}
	notes := make([]string, 0, 1)
	withinWindow := *lateMonths <= rule.MaxMonths
	if !withinWindow {
		notes = append(notes, fmt.Sprintf(
			"超期 %d 个月已超出 %d 个月的补缴窗口：本项不再可缴，专利权自应当缴纳年费期满之日起终止。",
			*lateMonths, rule.MaxMonths))
	}

	verificationDate := item.VerifiedOn
	if verificationDate == nil {
		verificationDate = rule.VerifiedOn
	}
	sourceDoc := item.SourceDoc
	if rule.SourceDoc != "" {
		sourceDoc = rule.SourceDoc
	}

	var surcharge *int64
	var monthly *int64
	if annualAmount != nil {
		if fen, err := ParseYuan(*annualAmount); err == nil {
			value := ApplyPercent(fen, rule.MonthlyPercent)
			monthly = &value
			if withinWindow {
				total := value * int64(*lateMonths)
				surcharge = &total
			}
		}
	}

	line := Line{
		ID:            fmt.Sprintf("%s-%d-late-payment", item.ID, year),
		Name:          fmt.Sprintf("年费滞纳金（第 %d 年度）", year),
		Basis:         BasisPerMonth,
		Quantity:      *lateMonths,
		QuantityBasis: fmt.Sprintf("超期 %d 个月，每月加收当年全额年费的 %d%%", *lateMonths, rule.MonthlyPercent),
		UnitAmount:    fenString(monthly),
		Status:        statusOf(annualAmount != nil, annualAmount != nil && verificationDate != nil),
		LegalBasis:    rule.LegalBasis,
		SourceDoc:     sourceDoc,
		VerifiedOn:    verificationDate,
		Notes:         notes,
	}
	line.Subtotal = fenString(surcharge)
	line.Payable = fenString(surcharge)
	return computedLine{line: line, payableFen: surcharge}, true
}

// uncoveredTriggers 返回索引未收录的收费阶段。
func uncoveredTriggers(table *Table, triggers []Trigger) []Trigger {
	indexed := make(map[Trigger]struct{}, len(table.Items))
	for _, item := range table.Items {
		indexed[item.Trigger] = struct{}{}
	}
	uncovered := make([]Trigger, 0, len(triggers))
	for _, trigger := range triggers {
		if _, ok := indexed[trigger]; !ok && !slices.Contains(uncovered, trigger) {
			uncovered = append(uncovered, trigger)
		}
	}
	return uncovered
}

// totalOf 汇总合计；任何适用行金额缺失、存在等待输入的费种或阶段未收录时不给合计。
//
// 这里的口径比「只统计已有金额」更严：缺输入时给出部分合计会被读成「这就是应缴额」，
// 与「缺输入即拒算」的原则相悖（见 docs/design/fail-closed-matrix.md FC-5/FC-6）。
func totalOf(lines []computedLine, uncovered []Trigger, pendingCount int) Total {
	total := Total{UncoveredTriggers: uncovered}
	var sum int64
	verifiedCount := 0
	for _, computed := range lines {
		line := computed.line
		if line.Quantity > 0 && (line.Status != StatusVerified || computed.payableFen == nil) {
			total.UnverifiedIDs = append(total.UnverifiedIDs, line.ID)
			continue
		}
		if line.Status == StatusVerified && computed.payableFen != nil && line.Quantity > 0 {
			sum += *computed.payableFen
			verifiedCount++
		}
	}
	total.Complete = len(total.UnverifiedIDs) == 0 && len(uncovered) == 0 && pendingCount == 0
	switch {
	case len(uncovered) > 0:
		total.Reason = "索引未收录本次点名的一个或多个收费阶段，合计不予给出。"
	case len(total.UnverifiedIDs) > 0:
		total.Reason = "存在金额未转录或未核验的适用费用项，合计不予给出。"
	case pendingCount > 0:
		total.Reason = "存在等待案件输入的费种，合计不予给出。"
	case verifiedCount == 0:
		total.Reason = "本次没有需要缴费的项目。"
		amount := FormatYuan(0)
		total.Amount = &amount
	default:
		amount := FormatYuan(sum)
		total.Amount = &amount
	}
	return total
}

// reductionOutcome 汇总减缴处理结果。
func reductionOutcome(reduction *reductionContext, lines []computedLine) *ReductionOutcome {
	if reduction == nil {
		return nil
	}
	applied := false
	for _, computed := range lines {
		if strings.HasPrefix(computed.line.Reduction, "减缴 ") {
			applied = true
			break
		}
	}
	outcome := &ReductionOutcome{Kind: reduction.request.Kind, Filed: reduction.request.Filed, Applied: applied}
	switch {
	case reduction.rule == nil:
		outcome.Reason = fmt.Sprintf("未登记 %s 减缴。", reduction.request.Kind)
	case reduction.rule.ReductionPercent == nil:
		outcome.Reason = "减缴比例未转录：本次不给减缴后金额。"
	case !reduction.request.Filed && reduction.rule.RequiresFiling:
		outcome.Reason = "未办理费减备案：仅对不以此为前提的费种适用。"
	default:
		percent := *reduction.rule.ReductionPercent
		outcome.ReductionPercent = &percent
		if applied {
			outcome.Reason = fmt.Sprintf("已按减缴 %d%% 计算适用费种。", percent)
		} else {
			outcome.Reason = "本案没有落在减缴范围内的费用项。"
		}
	}
	return outcome
}
