package deadline

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func mustDate(t *testing.T, text string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", text)
	if err != nil {
		t.Fatalf("解析日期 %q: %v", text, err)
	}
	return DateOnly(parsed)
}

func mustCalendar(t *testing.T) *WorkCalendar {
	t.Helper()
	calendar, err := BundledWorkCalendar()
	if err != nil {
		t.Fatalf("加载随包日历: %v", err)
	}
	return calendar
}

func format(d time.Time) string { return d.Format("2006-01-02") }

func TestAddMonthsClampsToMonthEnd(t *testing.T) {
	cases := []struct {
		start  string
		months int
		want   string
	}{
		{"1999-12-31", 2, "2000-02-29"},
		{"2026-01-31", 1, "2026-02-28"},
		{"2024-02-29", 12, "2025-02-28"},
		{"2026-03-31", 1, "2026-04-30"},
		{"2026-03-31", -1, "2026-02-28"},
	}
	for _, tc := range cases {
		got := AddMonths(mustDate(t, tc.start), tc.months)
		if format(got) != tc.want {
			t.Errorf("%s %+d 个月 = %s, want %s", tc.start, tc.months, format(got), tc.want)
		}
	}
}

func TestWorkCalendarHolidaysAndMovedWorkdays(t *testing.T) {
	calendar := mustCalendar(t)
	cases := []struct {
		date string
		want bool
		why  string
	}{
		{"2026-01-01", false, "元旦"},
		{"2026-01-04", true, "元旦调休上班（周日）"},
		{"2026-02-14", true, "春节调休上班（周六）"},
		{"2026-02-18", false, "春节"},
		{"2026-10-03", false, "国庆"},
		{"2026-10-08", true, "国庆后首个工作日"},
		{"2026-10-10", true, "国庆调休上班（周六）"},
		{"2026-03-07", false, "普通周六"},
		{"2026-03-09", true, "普通周一"},
	}
	for _, tc := range cases {
		got, err := calendar.IsWorkday(mustDate(t, tc.date))
		if err != nil {
			t.Fatalf("%s: %v", tc.date, err)
		}
		if got != tc.want {
			t.Errorf("%s（%s）= %v, want %v", tc.date, tc.why, got, tc.want)
		}
	}
}

func TestWorkCalendarUncoveredYearFailsLoud(t *testing.T) {
	calendar := mustCalendar(t)
	_, err := calendar.IsWorkday(mustDate(t, "2027-01-04"))
	var coverage *CalendarCoverageError
	if !errors.As(err, &coverage) {
		t.Fatalf("want CalendarCoverageError, got %v", err)
	}
	if coverage.Year != 2027 {
		t.Fatalf("year = %d", coverage.Year)
	}
}

func TestExtendToWorkday(t *testing.T) {
	calendar := mustCalendar(t)
	cases := []struct{ date, want string }{
		{"2026-10-03", "2026-10-08"}, // 国庆假期中 → 节后首个工作日
		{"2026-01-03", "2026-01-04"}, // 周六假日，次日调休上班
		{"2026-02-14", "2026-02-14"}, // 调休上班日当天
		{"2026-03-07", "2026-03-09"}, // 普通周六 → 周一
		{"2026-03-09", "2026-03-09"}, // 工作日当天
	}
	for _, tc := range cases {
		got, err := calendar.ExtendToWorkday(mustDate(t, tc.date))
		if err != nil {
			t.Fatalf("%s: %v", tc.date, err)
		}
		if format(got) != tc.want {
			t.Errorf("%s 顺延 = %s, want %s", tc.date, format(got), tc.want)
		}
	}
}

func TestResolveDelivery(t *testing.T) {
	dispatch := mustDate(t, "2026-03-06")
	entered := mustDate(t, "2026-03-09")
	actual := mustDate(t, "2026-03-10")
	handed := mustDate(t, "2026-03-06")
	published := mustDate(t, "2026-03-31")

	cases := []struct {
		name string
		req  DeliveryRequest
		want string
	}{
		{"电子送达按发文日", DeliveryRequest{Mode: DeliveryElectronic, DispatchDate: &dispatch}, "2026-03-06"},
		{"电子送达按进入系统日", DeliveryRequest{Mode: DeliveryElectronic, DispatchDate: &dispatch, EnteredDate: &entered}, "2026-03-09"},
		{"未指定方式按电子送达", DeliveryRequest{DispatchDate: &dispatch}, "2026-03-06"},
		{"邮寄推定加15日", DeliveryRequest{Mode: DeliveryPostal, DispatchDate: &dispatch}, "2026-03-21"},
		{"邮寄实际收到日优先", DeliveryRequest{Mode: DeliveryPostal, DispatchDate: &dispatch, ActualReceiptDate: &actual}, "2026-03-10"},
		{"直接送交按交付日", DeliveryRequest{Mode: DeliveryPersonal, HandedOverDate: &handed}, "2026-03-06"},
		{"公告送达加1个月", DeliveryRequest{Mode: DeliveryPublication, PublicationDate: &published}, "2026-04-30"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.req.Resolve()
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if format(got.Date) != tc.want {
				t.Fatalf("送达日 = %s, want %s", format(got.Date), tc.want)
			}
			if got.Basis == "" {
				t.Fatal("缺少依据说明")
			}
		})
	}
}

func TestResolveDeliveryMissingInput(t *testing.T) {
	cases := []DeliveryRequest{
		{Mode: DeliveryElectronic},
		{Mode: DeliveryPostal},
		{Mode: DeliveryPersonal},
		{Mode: DeliveryPublication},
		{Mode: "email"},
	}
	for i, req := range cases {
		if _, err := req.Resolve(); err == nil {
			t.Fatalf("case %d 应报错", i)
		}
	}
}

func TestNoticeDeadline(t *testing.T) {
	calendar := mustCalendar(t)
	dispatch := mustDate(t, "2026-03-06")
	cases := []struct {
		name   string
		notice Notice
		want   string
		months int
	}{
		{"一通实审4个月", Notice{Kind: NoticeOfficeActionFirst, Stage: StageSubstantive, Delivery: DeliveryRequest{DispatchDate: &dispatch}}, "2026-07-06", 4},
		{"一通初审2个月", Notice{Kind: NoticeOfficeActionFirst, Stage: StagePreliminary, Delivery: DeliveryRequest{DispatchDate: &dispatch}}, "2026-05-06", 2},
		{"驳回决定3个月（周末顺延）", Notice{Kind: NoticeRejectionDecision, Delivery: DeliveryRequest{DispatchDate: &dispatch}}, "2026-06-08", 3},
		{"后续OA2个月", Notice{Kind: NoticeOfficeActionSubsequent, Delivery: DeliveryRequest{DispatchDate: &dispatch}}, "2026-05-06", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NoticeDeadline(tc.notice, calendar)
			if err != nil {
				t.Fatalf("NoticeDeadline: %v", err)
			}
			if format(got.End) != tc.want {
				t.Fatalf("届满日 = %s, want %s（顺延前 %s）", format(got.End), tc.want, format(got.PeriodEnd))
			}
			if got.Months != tc.months {
				t.Fatalf("月数 = %d, want %d", got.Months, tc.months)
			}
		})
	}
}

func TestNoticeDesignatedMonthsOverride(t *testing.T) {
	calendar := mustCalendar(t)
	dispatch := mustDate(t, "2026-03-06")
	designated := 1
	got, err := NoticeDeadline(Notice{
		Kind:             NoticeOfficeActionSubsequent,
		Delivery:         DeliveryRequest{DispatchDate: &dispatch},
		DesignatedMonths: &designated,
	}, calendar)
	if err != nil {
		t.Fatalf("NoticeDeadline: %v", err)
	}
	// 03-06 + 1 个月 = 04-06，落在清明假期（04-04 至 04-06），顺延到 04-07。
	if format(got.End) != "2026-04-07" {
		t.Fatalf("届满日 = %s, want 2026-04-07（清明顺延）", format(got.End))
	}
	if format(got.PeriodEnd) != "2026-04-06" || !got.Extended {
		t.Fatalf("顺延前 = %s, extended = %v", format(got.PeriodEnd), got.Extended)
	}
}

func TestApplicationFeeWindowTakesEarlier(t *testing.T) {
	calendar := mustCalendar(t)
	filing := mustDate(t, "2026-01-05")

	// 受理通知较早：02-10 + 15 日 = 02-25，早于申请日起 2 个月的 03-05。
	earlyReceipt := mustDate(t, "2026-02-10")
	window, err := ApplicationFeeWindow(filing, &earlyReceipt, calendar)
	if err != nil {
		t.Fatalf("ApplicationFeeWindow: %v", err)
	}
	if format(window.End) != "2026-02-25" {
		t.Fatalf("届满日 = %s, want 2026-02-25（择先）", format(window.End))
	}
	if window.FromNotice == nil || format(*window.FromNotice) != "2026-02-25" {
		t.Fatalf("FromNotice = %v", window.FromNotice)
	}

	// 申请日较早：02-25 + 15 日 = 03-12，晚于 03-05。
	lateReceipt := mustDate(t, "2026-02-25")
	window, err = ApplicationFeeWindow(filing, &lateReceipt, calendar)
	if err != nil {
		t.Fatalf("ApplicationFeeWindow: %v", err)
	}
	if format(window.End) != "2026-03-05" {
		t.Fatalf("届满日 = %s, want 2026-03-05（择先）", format(window.End))
	}

	// 未提供受理日时，只能按申请日起 2 个月，并说明无法比较。
	window, err = ApplicationFeeWindow(filing, nil, calendar)
	if err != nil {
		t.Fatalf("ApplicationFeeWindow: %v", err)
	}
	if format(window.End) != "2026-03-05" || window.FromNotice != nil {
		t.Fatalf("window = %+v", window)
	}
	if window.Basis == "" || !strings.Contains(window.Basis, "无法比较") {
		t.Fatalf("basis = %q", window.Basis)
	}
}

func TestApplicationFeeWindowExtendsEachSide(t *testing.T) {
	calendar := mustCalendar(t)
	// 申请日 2026-01-05 + 2 个月 = 03-05（周四，工作日）。
	filing := mustDate(t, "2026-01-05")
	// 受理日 2026-02-20 + 15 日 = 03-07（周六）→ 顺延 03-09；比 03-05 晚，取 03-05。
	receipt := mustDate(t, "2026-02-20")
	window, err := ApplicationFeeWindow(filing, &receipt, calendar)
	if err != nil {
		t.Fatalf("ApplicationFeeWindow: %v", err)
	}
	if format(window.End) != "2026-03-05" {
		t.Fatalf("届满日 = %s", format(window.End))
	}
	if window.FromNotice == nil || format(*window.FromNotice) != "2026-03-09" {
		t.Fatalf("FromNotice = %v", window.FromNotice)
	}
}
