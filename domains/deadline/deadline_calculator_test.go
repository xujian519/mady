package deadline

import (
	"testing"
	"time"
)

// hasType 报告 deadlines 中是否包含指定类型的期限。
func hasType(ds []CalculatedDeadline, typ Type) bool {
	for _, d := range ds {
		if d.Type == typ {
			return true
		}
	}
	return false
}

// deadlineOfType 返回指定类型的第一个期限。
func deadlineOfType(ds []CalculatedDeadline, typ Type) CalculatedDeadline {
	for _, d := range ds {
		if d.Type == typ {
			return d
		}
	}
	return CalculatedDeadline{}
}

// fixedPastFiling 返回一个足够早的申请日，使全部法定期限均已到期，
// 从而保证结构断言与运行日期无关。
func fixedPastFiling() time.Time {
	return time.Date(2018, 3, 15, 0, 0, 0, 0, time.UTC)
}

func TestCalculatePatentDeadlines_Invention(t *testing.T) {
	ds := CalculatePatentDeadlines(fixedPastFiling(), "发明")
	for _, want := range []Type{
		DeadlinePriorityClaim,
		DeadlineSubstantiveExam,
		DeadlineDivisional,
		DeadlineOAResponse,
		DeadlineRegistration,
	} {
		if !hasType(ds, want) {
			t.Errorf("发明: 缺少期限类型 %q", want)
		}
	}
	// 2018 年申请的多年期年费均已到期，不应出现未来年费。
	if hasType(ds, DeadlineAnnFee) {
		t.Errorf("发明: 2018 年申请不应出现未来年费")
	}
}

func TestCalculatePatentDeadlines_UtilityModel(t *testing.T) {
	ds := CalculatePatentDeadlines(fixedPastFiling(), "实用新型")
	if !hasType(ds, DeadlinePriorityClaim) {
		t.Errorf("实用新型: 缺少优先权期限")
	}
	if !hasType(ds, DeadlineDivisional) {
		t.Errorf("实用新型: 缺少分案申请期限")
	}
	if hasType(ds, DeadlineSubstantiveExam) {
		t.Errorf("实用新型: 实质审查请求仅限发明专利，不应出现")
	}
}

func TestCalculatePatentDeadlines_Design(t *testing.T) {
	ds := CalculatePatentDeadlines(fixedPastFiling(), "外观设计")
	if !hasType(ds, DeadlinePriorityClaim) {
		t.Errorf("外观设计: 缺少优先权期限")
	}
	if hasType(ds, DeadlineSubstantiveExam) {
		t.Errorf("外观设计: 实质审查请求仅限发明专利，不应出现")
	}
	if hasType(ds, DeadlineDivisional) {
		t.Errorf("外观设计: 分案申请不应出现")
	}
}

func TestCalculatePatentDeadlines_PriorityWindow(t *testing.T) {
	filing := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// 发明专利优先权期限 = 申请日 + 12 个月。
	if inv := deadlineOfType(CalculatePatentDeadlines(filing, "发明"), DeadlinePriorityClaim); inv.DueDate != "2025-01-01" {
		t.Errorf("发明优先权期限 = %q, 期望 2025-01-01", inv.DueDate)
	}
	// 外观设计优先权期限 = 申请日 + 6 个月。
	if des := deadlineOfType(CalculatePatentDeadlines(filing, "外观设计"), DeadlinePriorityClaim); des.DueDate != "2024-07-01" {
		t.Errorf("外观设计优先权期限 = %q, 期望 2024-07-01", des.DueDate)
	}
}

func TestCalculatePatentDeadlines_NonDateLast(t *testing.T) {
	ds := CalculatePatentDeadlines(fixedPastFiling(), "发明")
	seenNonDate := false
	for _, d := range ds {
		if isNonDateDue(d) {
			seenNonDate = true
		} else if seenNonDate {
			t.Errorf("非日期占位（参见通知书/授权通知书）应排在所有具日期期限之后，但在 %q 之后出现 %q",
				d.DueDate, d.Label)
		}
	}
}

func TestNewDeadline_StatusAndDays(t *testing.T) {
	utc := time.UTC
	today := time.Date(2026, 8, 23, 0, 0, 0, 0, utc)

	// 未来 45 天 → normal。
	if d := newDeadline(DeadlineAnnFee, "年费", today.AddDate(0, 0, 45), today, "法条"); d.Status != "normal" || d.DaysRemaining != 45 {
		t.Errorf("45天: 状态=%q 天数=%d, 期望 normal/45", d.Status, d.DaysRemaining)
	}
	// 未来 30 天 → urgent（边界 <=30）。
	if d := newDeadline(DeadlineAnnFee, "年费", today.AddDate(0, 0, 30), today, "法条"); d.Status != "urgent" || d.DaysRemaining != 30 {
		t.Errorf("30天: 状态=%q 天数=%d, 期望 urgent/30", d.Status, d.DaysRemaining)
	}
	// 当天到期 → 0 天, 仍计 urgent（<=30）。
	if d := newDeadline(DeadlineAnnFee, "年费", today, today, "法条"); d.Status != "urgent" || d.DaysRemaining != 0 {
		t.Errorf("当天: 状态=%q 天数=%d, 期望 urgent/0", d.Status, d.DaysRemaining)
	}
	// 逾期 5 天 → overdue。
	if d := newDeadline(DeadlineAnnFee, "年费", today.AddDate(0, 0, -5), today, "法条"); d.Status != "overdue" || d.DaysRemaining != -5 {
		t.Errorf("-5天: 状态=%q 天数=%d, 期望 overdue/-5", d.Status, d.DaysRemaining)
	}
}

// TestDaysBetween 验证按整日历天差计算，修复原先因本地零点/UTC Parse
// 偏移或小数天截断导致的"差一天"偏差。
func TestDaysBetween(t *testing.T) {
	utc := time.UTC

	// 同一日历日的不同时刻 → 0 天。
	if got := daysBetween(time.Date(2026, 8, 23, 0, 0, 0, 0, utc), time.Date(2026, 8, 23, 23, 0, 0, 0, utc)); got != 0 {
		t.Errorf("同日不同时刻期望 0, 得到 %d", got)
	}
	// 次日凌晨 → 1 天。
	if got := daysBetween(time.Date(2026, 8, 23, 0, 0, 0, 0, utc), time.Date(2026, 8, 24, 0, 0, 0, 0, utc)); got != 1 {
		t.Errorf("次日期望 1, 得到 %d", got)
	}
	// 前一日 → -1 天。
	if got := daysBetween(time.Date(2026, 8, 24, 0, 0, 0, 0, utc), time.Date(2026, 8, 23, 0, 0, 0, 0, utc)); got != -1 {
		t.Errorf("前日期望 -1, 得到 %d", got)
	}
	// 跨月（2026 非闰年，2 月 28 天）: 01-31 → 03-01 = 29 天。
	if got := daysBetween(time.Date(2026, 1, 31, 0, 0, 0, 0, utc), time.Date(2026, 3, 1, 0, 0, 0, 0, utc)); got != 29 {
		t.Errorf("01-31→03-01 期望 29, 得到 %d", got)
	}
	// 跨年。
	if got := daysBetween(time.Date(2025, 12, 31, 0, 0, 0, 0, utc), time.Date(2026, 1, 1, 0, 0, 0, 0, utc)); got != 1 {
		t.Errorf("12-31→01-01 期望 1, 得到 %d", got)
	}
}

// TestDaysBetween_UtcVsLocalSkew 复现修复点：当申请日按 UTC 解析、而
// today 取本地零点时，若直接用 due.Sub(today) 会因小时差被截断；归一化
// 到各自日历日期的 UTC 零点后应得到正确的整日差。
func TestDaysBetween_UtcVsLocalSkew(t *testing.T) {
	// filing 按 time.Parse("2006-01-02") 得到 UTC 零点。
	filing := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC) // 近似 UTC 解析结果
	due := filing.AddDate(0, 0, 1)                         // 次日 UTC 零点

	// 模拟本地零点在 UTC+8：t2026-08-23 00:00+08:00 ≡ 2026-08-22 16:00 UTC。
	localToday := time.Date(2026, 8, 23, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))

	// 旧实现 int(due.Sub(localToday).Hours()/24) 会得到 16h/24 → 0，而非 1。
	if got := daysBetween(localToday, due); got != 1 {
		t.Errorf("UTC 解析 + 本地零点场景期望 1, 得到 %d", got)
	}
}
