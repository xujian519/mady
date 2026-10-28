package adversary

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// 法条目录与驳回类型映射：把通知书驳回类型、正文条款写法统一成 A22.3 这类编号。

// StatuteLabels 是法条编号 → 中文标签。
var StatuteLabels = map[string]string{
	"A2.2":  "专利法第2条第2款（技术方案客体）",
	"A2":    "专利法第2条（发明创造定义）",
	"A5":    "专利法第5条（违反法律/公序良俗）",
	"A9":    "专利法第9条（禁止重复授权）",
	"A22":   "专利法第22条（新颖性/创造性/实用性）",
	"A22.2": "专利法第22条第2款（新颖性）",
	"A22.3": "专利法第22条第3款（创造性）",
	"A22.4": "专利法第22条第4款（实用性）",
	"A22.5": "专利法第22条第5款（现有技术）",
	"A24":   "专利法第24条（宽限期）",
	"A25":   "专利法第25条（不授予专利权的客体）",
	"A26":   "专利法第26条（说明书与权利要求）",
	"A26.3": "专利法第26条第3款（充分公开）",
	"A26.4": "专利法第26条第4款（清楚/支持）",
	"A31":   "专利法第31条（单一性）",
	"A31.1": "专利法第31条第1款（单一性）",
	"A33":   "专利法第33条（修改超范围）",
	"A59":   "专利法第59条（保护范围）",
	"A62":   "专利法第62条（现有技术抗辩）",
}

// GroundToStatute 是驳回类型 → 法条编号。
// 同时接受 oa-parse 的驳回类型（clarity/support/scope）与程序理由标签。
var GroundToStatute = map[string]string{
	"novelty":              "A22.2",
	"inventiveness":        "A22.3",
	"utility":              "A22.4",
	"disclosure":           "A26.3",
	"clarity":              "A26.4",
	"support":              "A26.4",
	"claim-clarity":        "A26.4",
	"necessary-features":   "A26.4",
	"scope":                "A33",
	"amendment":            "A33",
	"divisional-amendment": "A33",
	"subject-matter-art2":  "A2.2",
	"subject-matter-art5":  "A5",
	"subject-matter-art25": "A25",
	"double-patenting":     "A9",
}

// StatuteToGround 是法条编号 → 主要驳回类型。
var StatuteToGround = map[string]string{
	"A22.2": "novelty",
	"A22.3": "inventiveness",
	"A22.4": "utility",
	"A26.3": "disclosure",
	"A26.4": "claim-clarity",
	"A33":   "amendment",
}

const (
	maxCandidates = 12
	maxClaims     = 50
)

var (
	statuteArticleRE = regexp.MustCompile(`(?:专利法)?\s*第\s*([0-9]{1,2}|[一二三四五六七八九十]{1,3})\s*条(?:\s*第\s*([0-9一二三四五六七八九十]{1,2})\s*款)?`)
	statuteShortRE   = regexp.MustCompile(`\bA\s*([0-9]{1,2})(?:\.([0-9]))?\b`)
	claimNumbersRE   = regexp.MustCompile(`权利要求\s*([0-9]+(?:\s*[-–~至]\s*[0-9]+)?(?:[\s、,，和及]*[0-9]+(?:\s*[-–~至]\s*[0-9]+)?)*)`)
	claimRangeRE     = regexp.MustCompile(`^([0-9]+)\s*[-–~至]\s*([0-9]+)$`)
)

var chineseDigits = map[string]int{
	"一": 1, "二": 2, "三": 3, "四": 4, "五": 5,
	"六": 6, "七": 7, "八": 8, "九": 9,
}

// chineseNumber 把条号/款号文本转成数字（支持「二十六」「三十三」这类写法）。
func chineseNumber(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	if number, err := strconv.Atoi(value); err == nil {
		return number, true
	}
	if value == "十" {
		return 10, true
	}
	tenIndex := strings.Index(value, "十")
	if tenIndex >= 0 {
		tens := 1
		if tenIndex > 0 {
			digit, ok := chineseDigits[value[:tenIndex]]
			if !ok {
				return 0, false
			}
			tens = digit
		}
		ones := 0
		if rest := value[tenIndex+len("十"):]; rest != "" {
			digit, ok := chineseDigits[rest]
			if !ok {
				return 0, false
			}
			ones = digit
		}
		return tens*10 + ones, true
	}
	digit, ok := chineseDigits[value]
	return digit, ok
}

// statuteKey 把条号/款号规范化成法条编号；未收录的条号返回空。
func statuteKey(article int, paragraph *int) string {
	key := "A" + strconv.Itoa(article)
	if paragraph != nil {
		key += "." + strconv.Itoa(*paragraph)
	}
	if _, ok := StatuteLabels[key]; ok {
		return key
	}
	return ""
}

// ScanDisputeText 从自由文本收集法条候选（去重、限长）。
func ScanDisputeText(text string) []string {
	seen := make(map[string]bool)
	var statutes []string
	add := func(key string) {
		if key == "" || seen[key] || len(statutes) >= maxCandidates {
			return
		}
		seen[key] = true
		statutes = append(statutes, key)
	}
	for _, match := range statuteArticleRE.FindAllStringSubmatch(text, -1) {
		article, ok := chineseNumber(match[1])
		if !ok {
			continue
		}
		var paragraph *int
		if match[2] != "" {
			if value, ok := chineseNumber(match[2]); ok {
				paragraph = &value
			}
		}
		add(statuteKey(article, paragraph))
	}
	for _, match := range statuteShortRE.FindAllStringSubmatch(text, -1) {
		article, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		var paragraph *int
		if match[2] != "" {
			if value, err := strconv.Atoi(match[2]); err == nil {
				paragraph = &value
			}
		}
		add(statuteKey(article, paragraph))
	}
	return statutes
}

// ScanClaimNumbers 从文本收集权利要求号，展开 `1-3` 这类区间。
func ScanClaimNumbers(text string) []int {
	claims := make(map[int]bool)
	for _, match := range claimNumbersRE.FindAllStringSubmatch(text, -1) {
		for _, part := range splitClaimList(match[1]) {
			if part == "" {
				continue
			}
			if rangeMatch := claimRangeRE.FindStringSubmatch(part); rangeMatch != nil {
				start, _ := strconv.Atoi(rangeMatch[1])
				end, _ := strconv.Atoi(rangeMatch[2])
				if end >= start && end-start <= maxClaims {
					for n := start; n <= end; n++ {
						claims[n] = true
					}
				}
				continue
			}
			if number, err := strconv.Atoi(part); err == nil {
				claims[number] = true
			}
		}
	}
	return normalizeClaims(claims)
}

// splitClaimList 按空白与列举分隔符切分权项号片段。
func splitClaimList(captured string) []string {
	fields := strings.FieldsFunc(captured, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '、', ',', '，', '和', '及':
			return true
		}
		return false
	})
	return fields
}

// normalizeClaims 去重、升序、限长；非正整数被丢弃。
func normalizeClaims(set map[int]bool) []int {
	if len(set) == 0 {
		return nil
	}
	claims := make([]int, 0, len(set))
	for value := range set {
		if value > 0 {
			claims = append(claims, value)
		}
	}
	sort.Ints(claims)
	if len(claims) > maxClaims {
		claims = claims[:maxClaims]
	}
	return claims
}
