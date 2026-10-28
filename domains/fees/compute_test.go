package fees

import (
	"os"
	"strings"
	"testing"
)

func loadTable(t *testing.T) *Table {
	t.Helper()
	data, err := os.ReadFile("data/cn-fees.yaml")
	if err != nil {
		t.Fatalf("读取费用索引: %v", err)
	}
	table, err := LoadTable(data, "data/cn-fees.yaml")
	if err != nil {
		t.Fatalf("LoadTable: %v", err)
	}
	return table
}

func lineByID(t *testing.T, report Report, id string) Line {
	t.Helper()
	for _, line := range report.Lines {
		if line.ID == id {
			return line
		}
	}
	t.Fatalf("未找到计价行 %s；已有 %v", id, lineIDs(report))
	return Line{}
}

func lineIDs(report Report) []string {
	ids := make([]string, len(report.Lines))
	for i, line := range report.Lines {
		ids[i] = line.ID
	}
	return ids
}

func deref(t *testing.T, value *string) string {
	t.Helper()
	if value == nil {
		t.Fatal("期望有值，实际为 nil")
	}
	return *value
}

// 发明申请阶段：申请费 + 公布印刷费，合计 950。
func TestInventionFilingTotal(t *testing.T) {
	table := loadTable(t)
	report, err := table.Compute(Query{
		PatentType:         PatentInvention,
		Triggers:           []Trigger{TriggerFiling},
		Claims:             new(10),
		SpecificationPages: new(30),
		PriorityClaims:     new(0),
	})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if got := deref(t, lineByID(t, report, "application-fee").Subtotal); got != "900.00" {
		t.Fatalf("申请费 = %s", got)
	}
	if got := deref(t, lineByID(t, report, "publication-printing-fee").Subtotal); got != "50.00" {
		t.Fatalf("公布印刷费 = %s", got)
	}
	if !report.Total.Complete {
		t.Fatalf("合计应完整，未核验行 %v", report.Total.UnverifiedIDs)
	}
	if got := deref(t, report.Total.Amount); got != "950.00" {
		t.Fatalf("合计 = %s", got)
	}
}

// 实用新型申请只缴 500，发明申请费按类型过滤。
func TestUtilityModelFiling(t *testing.T) {
	table := loadTable(t)
	report, err := table.Compute(Query{
		PatentType:         PatentUtilityModel,
		Triggers:           []Trigger{TriggerFiling},
		Claims:             new(10),
		SpecificationPages: new(30),
		PriorityClaims:     new(0),
	})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	for _, line := range report.Lines {
		if line.ID == "application-fee" {
			t.Fatal("发明专利的申请费条目不应出现在实用新型报告中")
		}
	}
	if got := deref(t, lineByID(t, report, "application-fee-utility-design").Payable); got != "500.00" {
		t.Fatalf("实用新型申请费 = %s", got)
	}
}

// 个人减缴已备案：申请费 900 → 135，公布印刷费不减缴。
func TestIndividualReductionFiled(t *testing.T) {
	table := loadTable(t)
	report, err := table.Compute(Query{
		PatentType:         PatentInvention,
		Triggers:           []Trigger{TriggerFiling},
		Claims:             new(10),
		SpecificationPages: new(30),
		PriorityClaims:     new(0),
		Reduction:          &ReductionRequest{Kind: "individual", Filed: true},
	})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if got := deref(t, lineByID(t, report, "application-fee").Payable); got != "135.00" {
		t.Fatalf("减缴后申请费 = %s", got)
	}
	if got := deref(t, lineByID(t, report, "publication-printing-fee").Payable); got != "50.00" {
		t.Fatalf("公布印刷费 = %s", got)
	}
	if report.Reduction == nil || !report.Reduction.Applied {
		t.Fatalf("减缴未标记为已应用：%+v", report.Reduction)
	}
}

// 减缴未备案：按全额计，并给出说明。
func TestReductionWithoutFiling(t *testing.T) {
	table := loadTable(t)
	report, err := table.Compute(Query{
		PatentType:         PatentInvention,
		Triggers:           []Trigger{TriggerFiling},
		Claims:             new(10),
		SpecificationPages: new(30),
		PriorityClaims:     new(0),
		Reduction:          &ReductionRequest{Kind: "individual", Filed: false},
	})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	line := lineByID(t, report, "application-fee")
	if got := deref(t, line.Payable); got != "900.00" {
		t.Fatalf("未备案应按全额，实际 = %s", got)
	}
	if line.Reduction != "未减缴（未备案）" {
		t.Fatalf("减缴标记 = %q", line.Reduction)
	}
}

// 年费分档：发明第 1 年 900、第 4 年 1200、第 10 年 4000。
func TestAnnuityTiers(t *testing.T) {
	table := loadTable(t)
	report, err := table.Compute(Query{
		PatentType:   PatentInvention,
		Triggers:     []Trigger{TriggerAnnualFee},
		AnnuityYears: []int{10, 1, 4},
	})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	cases := map[string]string{
		"annual-fee-1":  "900.00",
		"annual-fee-4":  "1200.00",
		"annual-fee-10": "4000.00",
	}
	for id, want := range cases {
		if got := deref(t, lineByID(t, report, id).Payable); got != want {
			t.Errorf("%s = %s, want %s", id, got, want)
		}
	}
	if !report.Total.Complete {
		t.Fatalf("合计应完整：%v", report.Total.UnverifiedIDs)
	}
	if got := deref(t, report.Total.Amount); got != "6100.00" {
		t.Fatalf("合计 = %s", got)
	}
}

// 超出已转录分档：不给金额并说明，合计不给。
func TestAnnuityBeyondTiers(t *testing.T) {
	table := loadTable(t)
	report, err := table.Compute(Query{
		PatentType:   PatentInvention,
		Triggers:     []Trigger{TriggerAnnualFee},
		AnnuityYears: []int{21},
	})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	line := lineByID(t, report, "annual-fee-21")
	if line.Payable != nil {
		t.Fatalf("超出分档不应给金额：%s", *line.Payable)
	}
	if line.Status != StatusUnrecorded {
		t.Fatalf("status = %s", line.Status)
	}
	if !containsNote(line.Notes, "未落在已转录的分档内") {
		t.Fatalf("notes = %v", line.Notes)
	}
	if report.Total.Amount != nil {
		t.Fatalf("合计应扣留，实际 = %s", *report.Total.Amount)
	}
}

// 滞纳金：超期 2 个月，每月加收 900 的 5% = 45，合计 90。
func TestLatePayment(t *testing.T) {
	table := loadTable(t)
	report, err := table.Compute(Query{
		PatentType:   PatentInvention,
		Triggers:     []Trigger{TriggerAnnualFee},
		AnnuityYears: []int{1},
		LateMonths:   new(2),
	})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	late := lineByID(t, report, "annual-fee-1-late-payment")
	if got := deref(t, late.Payable); got != "90.00" {
		t.Fatalf("滞纳金 = %s", got)
	}
	if late.Quantity != 2 {
		t.Fatalf("月数 = %d", late.Quantity)
	}
	if got := deref(t, report.Total.Amount); got != "990.00" {
		t.Fatalf("合计 = %s", got)
	}
}

// 超期 7 个月超出 6 个月窗口：滞纳金不给金额，并提示专利权终止。
func TestLatePaymentBeyondWindow(t *testing.T) {
	table := loadTable(t)
	report, err := table.Compute(Query{
		PatentType:   PatentInvention,
		Triggers:     []Trigger{TriggerAnnualFee},
		AnnuityYears: []int{1},
		LateMonths:   new(7),
	})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	late := lineByID(t, report, "annual-fee-1-late-payment")
	if late.Payable != nil {
		t.Fatalf("超窗口不应给金额：%s", *late.Payable)
	}
	if !containsNote(late.Notes, "专利权自应当缴纳年费期满之日起终止") {
		t.Fatalf("notes = %v", late.Notes)
	}
	if report.Total.Amount != nil {
		t.Fatalf("合计应扣留")
	}
}

// 未给年费年度：进入 pending，合计不给。
func TestAnnuityRequiresYears(t *testing.T) {
	table := loadTable(t)
	report, err := table.Compute(Query{
		PatentType: PatentInvention,
		Triggers:   []Trigger{TriggerAnnualFee},
	})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if len(report.Pending) != 1 || report.Pending[0].RequiredInput != "annuityYears" {
		t.Fatalf("pending = %+v", report.Pending)
	}
	if report.Total.Amount != nil {
		t.Fatalf("合计应扣留")
	}
}

// 说明书附加费金额未转录：该行 unrecorded，合计不给。
func TestUnrecordedSpecificationSurcharge(t *testing.T) {
	table := loadTable(t)
	report, err := table.Compute(Query{
		PatentType:         PatentInvention,
		Triggers:           []Trigger{TriggerFiling},
		Claims:             new(10),
		SpecificationPages: new(40),
		PriorityClaims:     new(0),
	})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	line := lineByID(t, report, "specification-surcharge")
	if line.Quantity != 10 {
		t.Fatalf("页数超出数量 = %d", line.Quantity)
	}
	if line.Status != StatusUnrecorded || line.Payable != nil {
		t.Fatalf("status=%s payable=%v", line.Status, line.Payable)
	}
	if report.Total.Amount != nil {
		t.Fatalf("合计应扣留")
	}
	if !containsNote(report.Notes, "合计不予给出") && report.Total.Reason == "" {
		t.Fatalf("应说明扣留原因")
	}
}

// 缺少计数输入：进入 pending，而不是按 0 计价。
func TestMissingCountInputIsPending(t *testing.T) {
	table := loadTable(t)
	report, err := table.Compute(Query{
		PatentType: PatentInvention,
		Triggers:   []Trigger{TriggerFiling},
	})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	inputs := make(map[string]bool, len(report.Pending))
	for _, pending := range report.Pending {
		inputs[pending.RequiredInput] = true
	}
	for _, want := range []string{"claims", "specificationPages", "priorityClaims"} {
		if !inputs[want] {
			t.Fatalf("缺少 %s 的 pending；实际 %+v", want, report.Pending)
		}
	}
}

// 非法输入直接报错，不静默按默认值计价。
func TestInvalidQuery(t *testing.T) {
	table := loadTable(t)
	cases := []Query{
		{PatentType: "plant", Triggers: []Trigger{TriggerFiling}},
		{PatentType: PatentInvention},
		{PatentType: PatentInvention, Triggers: []Trigger{"unknown-stage"}},
		{PatentType: PatentInvention, Triggers: []Trigger{TriggerAnnualFee}, AnnuityYears: []int{0}},
		{PatentType: PatentInvention, Triggers: []Trigger{TriggerAnnualFee}, LateMonths: new(-1)},
	}
	for i, query := range cases {
		if _, err := table.Compute(query); err == nil {
			t.Fatalf("case %d 应报错", i)
		}
	}
}

// 无证据表明索引被改动：抽查条目数与 null 金额数量。
func TestIndexShape(t *testing.T) {
	table := loadTable(t)
	if len(table.Items) != 19 {
		t.Fatalf("items = %d", len(table.Items))
	}
	nulls := 0
	for _, item := range table.Items {
		if item.Amount == nil {
			nulls++
		}
	}
	if nulls != 5 {
		t.Fatalf("未转录金额条目 = %d, want 5", nulls)
	}
	if len(table.Reductions) != 2 {
		t.Fatalf("reductions = %d", len(table.Reductions))
	}
}

func containsNote(notes []string, fragment string) bool {
	for _, note := range notes {
		if strings.Contains(note, fragment) {
			return true
		}
	}
	return false
}
