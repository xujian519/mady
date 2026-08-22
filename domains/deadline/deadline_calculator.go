package deadline

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Type categorizes a patent lifecycle deadline.
type Type string

// Patent deadline type constants.
const (
	DeadlineOAResponse      Type = "oa_response"      // 答复审查意见
	DeadlinePriorityClaim   Type = "priority_claim"   // 优先权期限
	DeadlineSubstantiveExam Type = "substantive_exam" // 请求实质审查
	DeadlineRegistration    Type = "registration"     // 办理登记手续
	DeadlineAnnFee          Type = "annual_fee"       // 年费
	DeadlineDivisional      Type = "divisional"       // 分案申请
	DeadlineReexamination   Type = "reexamination"    // 复审请求
	DeadlineInvalResponse   Type = "inval_response"   // 无效宣告答复
)

// 非日期占位：无法精确计算、需参见通知书的期限（DueDate 的哨兵值）。
const (
	dueDatePendingOA  = "参见通知书"
	dueDatePendingReg = "参见授权通知书"
)

// CalculatedDeadline is a computed deadline for display.
type CalculatedDeadline struct {
	Type          Type   `json:"type"`
	Label         string `json:"label"`
	DueDate       string `json:"due_date"`       // ISO 8601
	DaysRemaining int    `json:"days_remaining"` // positive = remaining, negative = overdue
	LegalBasis    string `json:"legal_basis"`
	Status        string `json:"status"` // "urgent" / "normal" / "overdue" / "pending"
}

// isNonDateDue 报告期限的 DueDate 是否为"参见通知书/授权通知书"这类非日期
// 占位，即无法精确计算、只能以待处理展示的期限。
func isNonDateDue(d CalculatedDeadline) bool {
	return d.DueDate == dueDatePendingOA || d.DueDate == dueDatePendingReg
}

// CalculatePatentDeadlines computes all statutory deadlines based on a filing
// date (or priority date). Returns deadlines in chronological order.
//
// The calculator covers the complete Chinese patent lifecycle.
func CalculatePatentDeadlines(filingDate time.Time, caseType string) []CalculatedDeadline {
	caseType = strings.TrimSpace(caseType)
	isInvention := strings.Contains(caseType, "发明")
	isUtilityModel := strings.Contains(caseType, "实用新型")
	isDesign := strings.Contains(caseType, "外观")

	var deadlines []CalculatedDeadline
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// 1. Priority claim deadlines.
	if isInvention || isUtilityModel {
		due := filingDate.AddDate(0, 12, 0)
		deadlines = append(deadlines, newDeadline(
			DeadlinePriorityClaim,
			"提交在先申请文件副本（12个月优先权期限）",
			due, today,
			"专利法第29条",
		))
	} else if isDesign {
		due := filingDate.AddDate(0, 6, 0)
		deadlines = append(deadlines, newDeadline(
			DeadlinePriorityClaim,
			"提交在先申请文件副本（6个月优先权期限）",
			due, today,
			"专利法第29条",
		))
	}

	// 2. Substantive examination request (invention only, 3 years).
	if isInvention {
		due := filingDate.AddDate(3, 0, 0)
		deadlines = append(deadlines, newDeadline(
			DeadlineSubstantiveExam,
			"请求实质审查并缴纳实审费",
			due, today,
			"专利法第35条",
		))
	}

	// 3. Divisional application deadline.
	if isInvention || isUtilityModel {
		due := filingDate.AddDate(2, 0, 0)
		deadlines = append(deadlines, newDeadline(
			DeadlineDivisional,
			"提出分案申请（应在办理授权登记手续之前）",
			due, today,
			"专利法实施细则第42条",
		))
	}

	// 4. OA response — cannot calculate without actual OA date.
	if today.After(filingDate.AddDate(0, 6, 0)) {
		deadlines = append(deadlines, CalculatedDeadline{
			Type:       DeadlineOAResponse,
			Label:      "答复审查意见通知书（具体期限参见通知书）——通常为发文日+15天+指定期限（2或4个月）",
			DueDate:    dueDatePendingOA,
			LegalBasis: "专利法第37条",
			Status:     "pending",
		})
	}

	// 5. Registration deadline — cannot calculate without notification date.
	if today.After(filingDate.AddDate(1, 0, 0)) {
		deadlines = append(deadlines, CalculatedDeadline{
			Type:       DeadlineRegistration,
			Label:      "办理专利登记手续、缴纳登记费、印花税及当年年费（期限以授权通知书为准，通常为发文日+2个月）",
			DueDate:    dueDatePendingReg,
			LegalBasis: "专利法实施细则第97条",
			Status:     "pending",
		})
	}

	// 6. Annual fee reminders for the next 3 years.
	for year := 1; year <= 3; year++ {
		due := filingDate.AddDate(year, 0, 0)
		if due.After(today) {
			deadlines = append(deadlines, newDeadline(
				DeadlineAnnFee,
				fmt.Sprintf("缴纳第%d年专利年费", year),
				due, today,
				"专利法第43条",
			))
		}
	}

	// Sort by due date, placing non-date entries at the end.
	sort.Slice(deadlines, func(i, j int) bool {
		if isNonDateDue(deadlines[i]) {
			return false
		}
		if isNonDateDue(deadlines[j]) {
			return true
		}
		return deadlines[i].DueDate < deadlines[j].DueDate
	})

	return deadlines
}

// newDeadline creates a CalculatedDeadline with computed days remaining.
func newDeadline(typ Type, label string, due, today time.Time, legalBasis string) CalculatedDeadline {
	dueStr := due.Format("2006-01-02")
	// 按整日历天差计算，避免时区/时刻偏移导致的截断偏差。
	daysRemaining := daysBetween(today, due)

	status := "normal"
	if daysRemaining < 0 {
		status = "overdue"
	} else if daysRemaining <= 30 {
		status = "urgent"
	}

	return CalculatedDeadline{
		Type:          typ,
		Label:         label,
		DueDate:       dueStr,
		DaysRemaining: daysRemaining,
		LegalBasis:    legalBasis,
		Status:        status,
	}
}

// daysBetween 返回从 from 到 to 的整日历天差（to - from），忽略时刻与
// 时区偏移。两侧归一到各自日历日期的 UTC 零点，结果恒为整天，不会因
// 本地零点与 UTC Parse 的偏移或小数天截断而偏差一天。
func daysBetween(from, to time.Time) int {
	fy, fm, fd := from.Date()
	ty, tm, td := to.Date()
	fromDay := time.Date(fy, fm, fd, 0, 0, 0, 0, time.UTC)
	toDay := time.Date(ty, tm, td, 0, 0, 0, 0, time.UTC)
	return int(toDay.Sub(fromDay).Hours() / 24)
}

// FormatDeadlineReport generates a Markdown deadline report.
func FormatDeadlineReport(deadlines []CalculatedDeadline) string {
	if len(deadlines) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("### 案件期限状态\n\n")
	b.WriteString("| 期限事项 | 截止日期 | 剩余天数 | 状态 | 法律依据 |\n")
	b.WriteString("|----------|----------|----------|------|----------|\n")

	urgentCount, overdueCount := 0, 0
	for _, d := range deadlines {
		statusIcon := ""
		switch d.Status {
		case "urgent":
			statusIcon = "🔴 紧急"
			urgentCount++
		case "overdue":
			statusIcon = "⛔ 已逾期"
			overdueCount++
		case "normal":
			statusIcon = "🟢 正常"
		default:
			statusIcon = "⚪ 待定"
		}

		daysStr := fmt.Sprintf("%d天", d.DaysRemaining)
		if isNonDateDue(d) {
			daysStr = "—"
		} else if d.DaysRemaining < 0 {
			daysStr = fmt.Sprintf("已逾期%d天", -d.DaysRemaining)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
			d.Label, d.DueDate, daysStr, statusIcon, d.LegalBasis)
	}

	b.WriteString("\n")
	if overdueCount > 0 {
		fmt.Fprintf(&b, "> ⛔ **警告**：%d 个期限已逾期，请立即处理！\n\n", overdueCount)
	}
	if urgentCount > 0 {
		fmt.Fprintf(&b, "> 🔴 **注意**：%d 个期限在30天内到期，请优先处理。\n\n", urgentCount)
	}
	return b.String()
}

// Summary is a JSON-serializable summary of deadlines for the Agent.
type Summary struct {
	Deadlines    []CalculatedDeadline `json:"deadlines"`
	UrgentCount  int                  `json:"urgent_count"`
	OverdueCount int                  `json:"overdue_count"`
	NextDeadline *CalculatedDeadline  `json:"next_deadline,omitempty"`
}

// SummarizeDeadlines creates a summary for quick status checks.
func SummarizeDeadlines(deadlines []CalculatedDeadline) Summary {
	s := Summary{Deadlines: deadlines}
	for _, d := range deadlines {
		if d.Status == "urgent" {
			s.UrgentCount++
		}
		if d.Status == "overdue" {
			s.OverdueCount++
		}
	}
	for _, d := range deadlines {
		if d.DaysRemaining > 0 && !isNonDateDue(d) {
			s.NextDeadline = &d
			break
		}
	}
	return s
}

// SerializeDeadlineSummary serializes the deadline summary as JSON.
func SerializeDeadlineSummary(summary Summary) string {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}
