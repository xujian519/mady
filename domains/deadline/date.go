package deadline

import "time"

// 日期运算一律以「日」为单位，时间部分归零到 UTC 零点，避免时区与夏令时把
// 天数算错。中国专利期限按自然日计算，不存在夏令时问题，用 UTC 零点最稳。

// DateOnly 把时间截断为当日零点（UTC）。
func DateOnly(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// AddDays 在日期上加减自然日。
func AddDays(t time.Time, days int) time.Time {
	return DateOnly(t).AddDate(0, 0, days)
}

// AddMonths 在日期上加减月数，目标月没有该日时取当月最后一日。
//
// 这是《专利法实施细则》第五条对以月/年计算的期限的规则：
// 1999-12-31 加两个月是 2000-02-29，不是 2000-03-02。
func AddMonths(t time.Time, months int) time.Time {
	base := DateOnly(t)
	year, month, day := base.Date()
	total := year*12 + int(month) - 1 + months
	targetYear := total / 12
	targetMonth := time.Month(total%12 + 1)
	if total < 0 {
		targetYear = (total - 11) / 12
		targetMonth = time.Month(total - targetYear*12 + 1)
	}
	lastDay := daysInMonth(targetYear, targetMonth)
	if day > lastDay {
		day = lastDay
	}
	return time.Date(targetYear, targetMonth, day, 0, 0, 0, 0, time.UTC)
}

// AddYears 在日期上加减年数，2 月 29 日落到非闰年时取 2 月 28 日。
func AddYears(t time.Time, years int) time.Time {
	return AddMonths(t, years*12)
}

// DaysBetween 返回两个日期之间的整天数（to − from）。
func DaysBetween(from, to time.Time) int {
	return int(DateOnly(to).Sub(DateOnly(from)).Hours() / 24)
}

// daysInMonth 返回某年某月的天数。
func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
