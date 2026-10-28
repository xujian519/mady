package deadline

import (
	"embed"
	"fmt"
	"sync"
	"time"

	"github.com/xujian519/mady/pkg/indexasset"
)

// 随包工作日历：法定节假日（非工作日）与调休上班日（原本是周末但上班）。
//
// 依据《专利法实施细则》第五条：期限届满日是法定休假日的，以休假日后的第一个
// 工作日为届满日；审查指南第五部分第七章 2.3 节把「移用周休息日」一并纳入。
//
//go:embed data/cn-holidays.yaml
var bundledCalendar embed.FS

// CalendarCoverageError 表示工作日历没有覆盖该年份。
//
// 缺年份不能按「非休假日」处理：那会给出一个法律上错误的届满日。
type CalendarCoverageError struct {
	// Year 是缺少安排的年份。
	Year int
}

// Error 返回可定位的错误文本。
func (e *CalendarCoverageError) Error() string {
	return fmt.Sprintf("工作日历未覆盖 %d 年：无法判断该日是否为法定休假日，故不能断定期限届满日是否需要顺延；请补充该年度节假日安排", e.Year)
}

// WorkCalendar 是加载后的工作日历。
type WorkCalendar struct {
	years    map[int]struct{}
	restDays map[string]struct{}
	workdays map[string]struct{}
}

var (
	bundledCalOnce sync.Once
	bundledCal     *WorkCalendar
	bundledCalErr  error
)

// BundledWorkCalendar 返回随包工作日历（进程内只解析一次）。
func BundledWorkCalendar() (*WorkCalendar, error) {
	bundledCalOnce.Do(func() {
		data, err := bundledCalendar.ReadFile("data/cn-holidays.yaml")
		if err != nil {
			bundledCalErr = err
			return
		}
		bundledCal, bundledCalErr = LoadWorkCalendar(data, "data/cn-holidays.yaml")
	})
	return bundledCal, bundledCalErr
}

// LoadWorkCalendar 解析工作日历资产。
func LoadWorkCalendar(data []byte, origin string) (*WorkCalendar, error) {
	fail := indexasset.FailFor(origin)
	root, err := indexasset.ParseYAMLMapping(data, "工作日历", fail)
	if err != nil {
		return nil, err
	}
	if err := indexasset.RequireKnownFields(root, []string{"years"}, "工作日历根节点", fail); err != nil {
		return nil, err
	}
	entries, ok := root["years"].([]any)
	if !ok || len(entries) == 0 {
		return nil, fail("字段 years 必须是非空数组")
	}

	calendar := &WorkCalendar{
		years:    make(map[int]struct{}, len(entries)),
		restDays: make(map[string]struct{}),
		workdays: make(map[string]struct{}),
	}
	for _, raw := range entries {
		entry, err := indexasset.ReadMapping(raw, "years 的每一项", fail)
		if err != nil {
			return nil, err
		}
		if err := indexasset.RequireKnownFields(entry,
			[]string{"year", "source", "holidays", "workdays"}, "年度条目", fail); err != nil {
			return nil, err
		}
		yearValue, err := indexasset.ReadOptionalCount(entry, "year", fail)
		if err != nil || yearValue == nil {
			return nil, fail("年度条目缺少 year")
		}
		year := *yearValue
		if _, dup := calendar.years[year]; dup {
			return nil, fail(fmt.Sprintf("年度 %d 重复登记", year))
		}
		calendar.years[year] = struct{}{}
		if err := calendar.addDates(entry["holidays"], calendar.restDays, year, "holidays", fail); err != nil {
			return nil, err
		}
		if err := calendar.addDates(entry["workdays"], calendar.workdays, year, "workdays", fail); err != nil {
			return nil, err
		}
	}
	return calendar, nil
}

// addDates 把日期列表写入目标集合，并校验年份归属。
func (c *WorkCalendar) addDates(value any, target map[string]struct{}, year int, field string, fail indexasset.Fail) error {
	if value == nil {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return fail(fmt.Sprintf("字段 %s 必须是数组", field))
	}
	for _, raw := range items {
		date, err := toCalendarDate(raw)
		if err != nil {
			return fail(fmt.Sprintf("字段 %s 含无法解析的日期：%v", field, raw))
		}
		if date.Year() != year {
			return fail(fmt.Sprintf("字段 %s 的日期 %s 不属于 %d 年", field, date.Format("2006-01-02"), year))
		}
		target[date.Format("2006-01-02")] = struct{}{}
	}
	return nil
}

// toCalendarDate 把资产里的日期（字符串或 yaml 时间戳）转成日期。
func toCalendarDate(value any) (time.Time, error) {
	switch typed := value.(type) {
	case string:
		return time.Parse("2006-01-02", typed)
	case time.Time:
		return DateOnly(typed), nil
	default:
		return time.Time{}, fmt.Errorf("不支持的日期类型 %T", value)
	}
}

// Covers 返回日历是否覆盖该年份。
func (c *WorkCalendar) Covers(year int) bool {
	if c == nil {
		return false
	}
	_, ok := c.years[year]
	return ok
}

// IsWorkday 判断某日是否为工作日。
//
// 调休上班日优先于周末判定；法定节假日为休息日。年份未覆盖时返回
// *CalendarCoverageError，而不是猜测「不是休假日」。
func (c *WorkCalendar) IsWorkday(date time.Time) (bool, error) {
	day := DateOnly(date)
	if c == nil || !c.Covers(day.Year()) {
		return false, &CalendarCoverageError{Year: day.Year()}
	}
	key := day.Format("2006-01-02")
	if _, ok := c.workdays[key]; ok {
		return true, nil
	}
	if _, ok := c.restDays[key]; ok {
		return false, nil
	}
	switch day.Weekday() {
	case time.Saturday, time.Sunday:
		return false, nil
	default:
		return true, nil
	}
}

// ExtendToWorkday 把日期顺延到当天或之后第一个工作日。
//
// 当天已是工作日时原样返回。
func (c *WorkCalendar) ExtendToWorkday(date time.Time) (time.Time, error) {
	day := DateOnly(date)
	for range 60 {
		workday, err := c.IsWorkday(day)
		if err != nil {
			return time.Time{}, err
		}
		if workday {
			return day, nil
		}
		day = day.AddDate(0, 0, 1)
	}
	return time.Time{}, fmt.Errorf("deadline: 从 %s 起 60 天内没有工作日，请检查日历", DateOnly(date).Format("2006-01-02"))
}
