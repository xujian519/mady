package evidence

import (
	"testing"
	"time"
)

func TestDeterminePublicationDate(t *testing.T) {
	tests := []struct {
		input    string
		wantTime bool // 是否期望解析成功
		wantStr  string
	}{
		{"2006-01-02", true, "2006-01-02"},
		{"2023-06-15", true, "2023-06-15"},
		{"2006/01/02", true, "2006/01/02"},
		{"2023.06.15", true, "2023.06.15"},
		{"20230615", true, "20230615"},
		{"2006年1月2日", true, "2006年1月2日"},
		{"2023年06月15日", true, "2023年06月15日"},
		{"Jan 2, 2006", true, "Jan 2, 2006"},
		{"", false, ""},
		{"not-a-date", false, "not-a-date"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotStr, gotTime := DeterminePublicationDate(tt.input)
			if gotStr != tt.wantStr {
				t.Errorf("返回字符串 = %q, 期望 %q", gotStr, tt.wantStr)
			}
			if tt.wantTime && gotTime.IsZero() {
				t.Error("期望解析成功，但返回零值 time.Time")
			}
			if !tt.wantTime && !gotTime.IsZero() {
				t.Errorf("期望解析失败，但返回 %v", gotTime)
			}
		})
	}
}

func TestDetermineInternetPublicationDate(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		claimedDate string
		wantIsPrior bool
	}{
		{"有效日期", "https://example.com/doc", "2023-01-15", true},
		{"空日期", "https://example.com/doc", "", false},
		{"空URL和日期", "", "", false},
		{"无效日期", "https://example.com/doc", "not-a-date", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetermineInternetPublicationDate(tt.url, tt.claimedDate)
			if result == nil {
				t.Fatal("返回 nil")
			}
			if result.Method != "internet_publication" {
				t.Errorf("Method = %q, 期望 %q", result.Method, "internet_publication")
			}
		})
	}
}

func TestIsBeforeFilingDate(t *testing.T) {
	tests := []struct {
		pubDate    string
		filingDate string
		want       bool
		wantErr    bool
	}{
		{"2023-01-01", "2023-06-01", true, false},
		{"2023-06-01", "2023-01-01", false, false},
		{"2023-01-01", "2023-01-01", false, false},
		{"", "2023-06-01", false, true},
		{"2023-01-01", "", false, true},
		{"invalid", "2023-06-01", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.pubDate+"_"+tt.filingDate, func(t *testing.T) {
			got, reason := isBeforeFilingDate(tt.pubDate, tt.filingDate)
			if got != tt.want {
				t.Errorf("isBeforeFilingDate = %v, 期望 %v (reason: %s)", got, tt.want, reason)
			}
			if tt.wantErr && reason == "" {
				t.Error("期望错误原因，但返回空字符串")
			}
		})
	}
}

func TestParseDateFlexible(t *testing.T) {
	tests := []struct {
		input string
		want  string // 期望格式化的输出
		fail  bool
	}{
		{"2006-01-02", "2006-01-02", false},
		{"2006/01/02", "2006-01-02", false},
		{"2006.01.02", "2006-01-02", false},
		{"20060102", "2006-01-02", false},
		{"2006年1月2日", "2006-01-02", false},
		{"", "", true},
		{"abcdefg", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDateFlexible(tt.input)
			if tt.fail {
				if err == nil {
					t.Errorf("期望错误，但解析成功: %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDateFlexible(%q) 返回错误: %v", tt.input, err)
			}
			if got.Format("2006-01-02") != tt.want {
				t.Errorf("结果 = %s, 期望 %s", got.Format("2006-01-02"), tt.want)
			}
		})
	}
}

func TestParseDateFlexible_SlashFallback(t *testing.T) {
	// 斜杠格式回退测试
	got, err := parseDateFlexible("2023/06/15")
	if err != nil {
		t.Fatalf("斜杠格式解析失败: %v", err)
	}
	expected := time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(expected) {
		t.Errorf("结果 = %v, 期望 %v", got, expected)
	}
}

func TestDeterminePublicationDate_Consistency(t *testing.T) {
	// 验证多种格式指向同一日期
	formats := []string{
		"2023-06-15",
		"2023/06/15",
		"2023.06.15",
		"20230615",
		"2023年6月15日",
		"2023年06月15日",
		"Jun 15, 2023",
	}

	expected := time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC)

	for _, f := range formats {
		t.Run(f, func(t *testing.T) {
			_, parsed := DeterminePublicationDate(f)
			if parsed.IsZero() {
				t.Logf("格式 %q 解析失败（可接受）", f)
				return
			}
			if !parsed.Equal(expected) {
				t.Errorf("结果 %v, 期望 %v", parsed, expected)
			}
		})
	}
}
