package evidence

import (
	"fmt"
	"strings"
	"time"
)

// dateFormats 支持解析的日期格式列表。
var dateFormats = []string{
	"2006-01-02",
	"2006/01/02",
	"2006.01.02",
	"20060102",
	"2006-01",
	"2006/01",
	"2006年1月2日",
	"2006年01月02日",
	"2006年1月",
	"Jan 2, 2006",
	"January 2, 2006",
	"02-Jan-2006",
	"2 January 2006",
}

// DeterminePublicationDate 确定证据的公开日期。
// 返回 dateStr 和对应的 time.Time 值（解析失败时 time.Time{}）。
func DeterminePublicationDate(dateStr string) (string, time.Time) {
	if dateStr == "" {
		return "", time.Time{}
	}

	trimmed := strings.TrimSpace(dateStr)

	for _, layout := range dateFormats {
		if t, err := time.Parse(layout, trimmed); err == nil {
			return trimmed, t
		}
	}

	return trimmed, time.Time{}
}

// DetermineInternetPublicationDate 确定互联网公开证据的日期。
// 互联网证据的公开日期以其首次公开日为准。
func DetermineInternetPublicationDate(urlStr string, claimedDate string) *DateDetermination {
	result := &DateDetermination{
		SourceDate: claimedDate,
		Method:     "internet_publication",
	}

	if urlStr == "" && claimedDate == "" {
		result.Determined = "unknown"
		result.IsPriorArt = false
		return result
	}

	determined, parsed := DeterminePublicationDate(claimedDate)
	result.Determined = determined

	if !parsed.IsZero() {
		result.IsPriorArt = true
	} else {
		result.IsPriorArt = false
	}

	return result
}

// isBeforeFilingDate 判断公开日期是否在申请日之前。
// 使用 validCount 风格命名以保持一致。
func isBeforeFilingDate(pubDate, filingDate string) (bool, string) {
	if pubDate == "" || filingDate == "" {
		return false, "日期不能为空"
	}

	pubParsed, errPub := parseDateFlexible(pubDate)
	filingParsed, errFiling := parseDateFlexible(filingDate)

	if errPub != nil || errFiling != nil {
		var errs []string
		if errPub != nil {
			errs = append(errs, fmt.Sprintf("公开日解析失败: %v", errPub))
		}
		if errFiling != nil {
			errs = append(errs, fmt.Sprintf("申请日解析失败: %v", errFiling))
		}
		return false, strings.Join(errs, "; ")
	}

	if pubParsed.Before(filingParsed) {
		return true, fmt.Sprintf("公开日 %s 早于申请日 %s", pubParsed.Format("2006-01-02"), filingParsed.Format("2006-01-02"))
	}

	return false, fmt.Sprintf("公开日 %s 不早于申请日 %s", pubParsed.Format("2006-01-02"), filingParsed.Format("2006-01-02"))
}

// parseDateFlexible 尝试多种格式解析日期字符串。
// 包含斜杠格式回退（如 2006/01/02）。
func parseDateFlexible(dateStr string) (time.Time, error) {
	trimmed := strings.TrimSpace(dateStr)

	for _, layout := range dateFormats {
		if t, err := time.Parse(layout, trimmed); err == nil {
			return t, nil
		}
	}

	// 斜杠格式回退：尝试将斜杠替换为短横线再解析
	if strings.Contains(trimmed, "/") {
		replaced := strings.ReplaceAll(trimmed, "/", "-")
		if t, err := time.Parse("2006-01-02", replaced); err == nil {
			return t, nil
		}
		if t, err := time.Parse("2006-01", replaced); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("无法识别的日期格式: %s", dateStr)
}
