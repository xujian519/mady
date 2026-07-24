package evidence

import (
	"fmt"
	"net/url"
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
// 根据多个日期源推定：页面标注日期 > HTTP 头 > Wayback Machine > 域名注册 > 主张方声称。
// 返回带有可靠度评级的 DateDetermination。
func DetermineInternetPublicationDate(urlStr string, claimedDate string) *DateDetermination {
	result := &DateDetermination{
		SourceDate:  claimedDate,
		Method:      "internet_publication",
		Reliability: RelLow,
		SourceType:  SrcInferred,
	}

	if urlStr == "" && claimedDate == "" {
		result.Determined = "unknown"
		result.IsPriorArt = false
		return result
	}

	// 第 1 优先级：页面标注日期（精确到日的格式）
	if claimedDate != "" {
		determined, parsed := DeterminePublicationDate(claimedDate)
		result.Determined = determined

		if !parsed.IsZero() {
			result.IsPriorArt = true
			// 根据日期精度判断可靠度
			switch {
			case isPreciseDate(determined):
				result.Reliability = RelHigh
				result.SourceType = SrcExactPage
			case isMonthOnlyDate(determined):
				// 月级日期按月末推定（较为保守）
				result.Reliability = RelMedium
				result.SourceType = SrcClaimed
				// 将月级日期推定到月末最后一天
				result.Determined = inferredMonthEnd(parsed)
			default:
				result.Reliability = RelMedium
				result.SourceType = SrcClaimed
			}
			return result
		}
	}

	// 第 2 优先级：从 URL 推断（如 Wayback Machine 嵌入的日期）
	if urlStr != "" {
		// 清理自定义 evidence URI scheme 前缀后解析
		cleanURL := urlStr
		prefixes := []string{"web_pub:", "http_archive:", "web:", "pub_use:", "public_use:", "witness:", "patent:", "prior_art:"}
		for _, p := range prefixes {
			if strings.HasPrefix(cleanURL, p) {
				cleanURL = cleanURL[len(p):]
				break
			}
		}
		if wmDate := extractWaybackMachineDate(cleanURL); wmDate != "" {
			determined, parsed := DeterminePublicationDate(wmDate)
			if !parsed.IsZero() {
				result.Determined = determined
				result.IsPriorArt = true
				result.Reliability = RelMedium
				result.SourceType = SrcWaybackMachine
				return result
			}
		}

		// 第 3 优先级：域名注册日期推断
		if domDate := estimateDomainRegistrationDate(cleanURL); domDate != "" {
			result.Determined = domDate
			result.IsPriorArt = true
			result.Reliability = RelLow
			result.SourceType = SrcDomainReg
			return result
		}
	}

	// 无法确定日期
	result.Determined = "unknown"
	result.IsPriorArt = false
	return result
}

// DeterminePublicUseDate 确定使用公开证据的日期。
// 从证据描述文本和主张日期中提取首次公开使用的日期。
func DeterminePublicUseDate(description string, claimedDate string, filingDate string) *DateDetermination {
	result := &DateDetermination{
		SourceDate:  claimedDate,
		Method:      "public_use",
		FilingDate:  filingDate,
		Reliability: RelLow,
		SourceType:  SrcClaimed,
	}

	if claimedDate == "" && description == "" {
		result.Determined = "unknown"
		result.IsPriorArt = false
		return result
	}

	// 尝试从主张的日期解析
	if claimedDate != "" {
		determined, parsed := DeterminePublicationDate(claimedDate)
		if !parsed.IsZero() {
			result.IsPriorArt = isBeforeFilingBool(determined, filingDate)
			// 使用公开通常需要旁证印证，日期可靠度默认中等
			switch {
			case isPreciseDate(determined):
				result.Determined = determined
				result.Reliability = RelMedium
			case isMonthOnlyDate(determined):
				// 月级日期推定到月末
				result.Determined = inferredMonthEnd(parsed)
				result.Reliability = RelLow
			default:
				result.Determined = determined
				result.Reliability = RelLow
			}
			result.SourceType = SrcClaimed
			return result
		}
	}

	// 尝试从描述文本中提取日期
	if description != "" {
		if extractedDate := extractDateFromText(description); extractedDate != "" {
			determined, parsed := DeterminePublicationDate(extractedDate)
			if !parsed.IsZero() {
				result.SourceDate = extractedDate
				result.IsPriorArt = isBeforeFilingBool(determined, filingDate)
				result.Reliability = RelLow
				result.SourceType = SrcInferred
				switch {
				case isPreciseDate(determined):
					result.Determined = determined
				case isMonthOnlyDate(determined):
					result.Determined = inferredMonthEnd(parsed)
				default:
					result.Determined = determined
				}
				return result
			}
		}
	}

	result.Determined = "unknown"
	result.IsPriorArt = false
	return result
}

// isPreciseDate 判断日期字符串是否精确到日（包含日信息）。
func isPreciseDate(dateStr string) bool {
	// 精确到日的格式通常包含三位日期分量
	formatsWithDay := []string{
		"2006-01-02",
		"2006/01/02",
		"2006.01.02",
		"20060102",
		"2006年1月2日",
		"2006年01月02日",
		"Jan 2, 2006",
		"January 2, 2006",
		"02-Jan-2006",
		"2 January 2006",
	}
	for _, layout := range formatsWithDay {
		if _, err := time.Parse(layout, dateStr); err == nil {
			return true
		}
	}
	return false
}

// isMonthOnlyDate 判断日期字符串是否只有年月（无日信息）。
func isMonthOnlyDate(dateStr string) bool {
	monthOnlyFormats := []string{
		"2006-01",
		"2006/01",
		"2006年1月",
	}
	for _, layout := range monthOnlyFormats {
		if _, err := time.Parse(layout, dateStr); err == nil {
			return true
		}
	}
	return false
}

// inferredMonthEnd 将月级日期推定到月末最后一天。
// 专利实践中，仅知月份时一般推定到当月最后一天作为公开日。
func inferredMonthEnd(t time.Time) string {
	// 获取下个月的第 0 天（即本月最后一天）
	firstOfNextMonth := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	lastDay := firstOfNextMonth.AddDate(0, 0, -1)
	return lastDay.Format("2006-01-02")
}

// extractWaybackMachineDate 从 Wayback Machine URL 中提取存档日期。
// 支持格式：web.archive.org/web/20230615000000/...
func extractWaybackMachineDate(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	hostname := strings.ToLower(parsed.Hostname())
	if !strings.Contains(hostname, "web.archive.org") &&
		!strings.Contains(hostname, "archive.org") {
		return ""
	}

	// 路径格式：/web/YYYYMMDDhhmmss/...
	path := parsed.Path
	parts := strings.Split(strings.TrimPrefix(path, "/web/"), "/")
	if len(parts) < 1 {
		return ""
	}

	timestamp := parts[0]
	if len(timestamp) >= 8 {
		// 将 YYYYMMDDhhmmss 转为 YYYY-MM-DD
		formatted := timestamp[:4] + "-" + timestamp[4:6] + "-" + timestamp[6:8]
		if _, err := time.Parse("2006-01-02", formatted); err == nil {
			return formatted
		}
	}

	return ""
}

// estimateDomainRegistrationDate 根据域名后缀估计注册日期（粗略推断）。
// 仅作参考，不能作为确切的公开日期依据。
func estimateDomainRegistrationDate(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	hostname := strings.ToLower(parsed.Hostname())
	// 某些顶级域名有明确的注册制度，但无法从 URL 直接获取注册日期
	// 此处不返回具体日期，而是返回空字符串，提示需要额外查询
	_ = hostname
	return ""
}

// extractDateFromText 从描述文本中尝试提取日期。
// 支持 "XXXX年XX月XX日"、"YYYY年MM月"、"YYYY-MM-DD" 等日期表达。
// 返回找到的最精确的日期表达。
func extractDateFromText(text string) string {
	if text == "" {
		return ""
	}

	// 策略一：按空白和标点分割后逐一尝试
	lines := strings.Fields(text)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if _, err := parseDateFlexible(line); err == nil {
			return line
		}
	}

	separators := []string{"，", "。", "；", "、", ",", ".", ";", " "}
	for _, sep := range separators {
		parts := strings.Split(text, sep)
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if _, err := parseDateFlexible(part); err == nil {
				return part
			}
		}
	}

	// 策略二：扫描完整的中文日期 "XXXX年XX月XX日"（优先于仅到月的日期）
	// 使用 rune 切片处理，安全支持多字节中文字符
	runes := []rune(text)
	var bestCandidate string
	var bestScore int // 0=none, 1=month-only, 2=with-day, 3=exact YYYY-MM-DD

	for idx := 0; idx < len(runes); idx++ {
		// 查找 "年" 字符
		if runes[idx] != '年' {
			continue
		}
		// 向前取4位数字作为年份
		if idx < 4 {
			continue
		}
		yearRunes := runes[idx-4 : idx]
		if !isAllRunesDigits(yearRunes) {
			continue
		}

		restRunes := runes[idx+1:]

		// 找 "月" 和 "日"
		monthPos := -1
		for j, r := range restRunes {
			if r == '月' {
				monthPos = j
				break
			}
		}
		if monthPos < 0 {
			continue
		}

		// 检查月份部分是否为数字
		monthPart := restRunes[:monthPos]
		if !isAllRunesDigits(monthPart) {
			continue
		}

		// 找 "日"
		dayPos := -1
		for j, r := range restRunes[monthPos+1:] {
			if r == '日' {
				dayPos = monthPos + 1 + j
				break
			}
		}
		if dayPos >= 0 {
			dayPart := restRunes[monthPos+1 : dayPos]
			if isAllRunesDigits(dayPart) && bestScore < 2 {
				// 提取到 "日" 的完整日期
				candidateRunes := runes[idx-4 : idx+1+dayPos+1]
				candidate := string(candidateRunes)
				if _, err := parseDateFlexible(candidate); err == nil {
					bestCandidate = candidate
					bestScore = 2
				}
			}
		} else if bestScore < 1 {
			// 仅有 "月"
			candidateRunes := runes[idx-4 : idx+1+monthPos+1]
			candidate := string(candidateRunes)
			if _, err := parseDateFlexible(candidate); err == nil {
				bestCandidate = candidate
				bestScore = 1
			}
		}
	}
	if bestScore >= 2 {
		return bestCandidate
	}
	if bestScore == 1 {
		return bestCandidate
	}

	// 策略三：扫描 YYYY-MM-DD 和 YYYY/MM/DD 模式（ASCII 安全，可用 byte）
	for i := 0; i <= len(text)-10; i++ {
		sub := text[i : i+10]
		if _, err := time.Parse("2006-01-02", sub); err == nil {
			return sub
		}
		if _, err := time.Parse("2006/01/02", sub); err == nil {
			return sub
		}
	}

	// 策略四：滑动窗口扫描，寻找可解析的日期子串
	for window := 10; window >= 4; window-- {
		for i := 0; i <= len(text)-window; i++ {
			sub := text[i : i+window]
			if _, err := parseDateFlexible(sub); err == nil {
				return sub
			}
		}
	}

	return bestCandidate
}

// isAllRunesDigits 检查 rune 切片是否全部为数字字符。
func isAllRunesDigits(rs []rune) bool {
	if len(rs) == 0 {
		return false
	}
	for _, r := range rs {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isAllDigits 检查字符串是否全部为 ASCII 数字字符。
func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// isBeforeFilingBool 判断公开日期是否在申请日之前（返回 bool）。
func isBeforeFilingBool(pubDate, filingDate string) bool {
	if pubDate == "" || filingDate == "" {
		return false
	}

	pubParsed, errPub := parseDateFlexible(pubDate)
	filingParsed, errFiling := parseDateFlexible(filingDate)

	if errPub != nil || errFiling != nil {
		return false
	}

	return pubParsed.Before(filingParsed)
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
