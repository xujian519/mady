package rules

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// OaRejectionType classifies the rejection category found in an Office Action.
type OaRejectionType string

const (
	OaNovelty       OaRejectionType = "novelty"
	OaInventiveness OaRejectionType = "inventiveness"
	OaClarity       OaRejectionType = "clarity"
	OaSupport       OaRejectionType = "support"
	OaDisclosure    OaRejectionType = "disclosure"
	OaScope         OaRejectionType = "scope"
	OaFormal        OaRejectionType = "formal"
	OaOther         OaRejectionType = "other"
)

// CitedReference is a prior-art document referenced in an Office Action.
type CitedReference struct {
	DocumentNumber string
	Relevancy      string
	ClaimsAffected []int
}

// ParsedOfficeAction holds the structured result of parsing an Office Action text.
type ParsedOfficeAction struct {
	RejectionType     OaRejectionType
	Citations         []CitedReference
	AffectedClaims    []int
	ExaminerArguments []string
}

var oaRejectionPatterns = []struct {
	typ      OaRejectionType
	patterns []string
}{
	{OaInventiveness, []string{"创造性", "显而易见", "22条第3款", "不具备创造性"}},
	{OaNovelty, []string{"新颖性", "不具备新颖性", "22条第2款", "技术方案被公开"}},
	{OaClarity, []string{"不清楚", "26条第4款", "简明"}},
	{OaSupport, []string{"不支持", "得不到说明书支持"}},
	{OaDisclosure, []string{"公开不充分", "26条第3款", "无法实现"}},
	{OaScope, []string{"保护范围", "33条", "修改超范围"}},
	{OaFormal, []string{"形式", "格式", "书写", "明显错误"}},
}

// DetectOaRejectionType inspects text and returns the matching rejection category.
func DetectOaRejectionType(text string) OaRejectionType {
	t := strings.ToLower(text)
	for _, entry := range oaRejectionPatterns {
		for _, p := range entry.patterns {
			if strings.Contains(t, strings.ToLower(p)) {
				return entry.typ
			}
		}
	}
	return OaOther
}

var allPatentRe = regexp.MustCompile(`(?:CN|US|WO|EP|JP|KR)\d{6,}[A-Z]?`)

// ExtractCitations finds all unique patent document references in the text.
func ExtractCitations(text string) []CitedReference {
	matches := allPatentRe.FindAllString(text, -1)
	seen := make(map[string]bool)
	var citations []CitedReference
	for _, docNum := range matches {
		if !seen[docNum] {
			seen[docNum] = true
			relevancy := "A"
			if strings.Contains(text, "X"+docNum) || strings.Contains(text, docNum+"X") {
				relevancy = "X"
			}
			citations = append(citations, CitedReference{
				DocumentNumber: docNum,
				Relevancy:      relevancy,
			})
		}
	}
	return citations
}

var claimRe = regexp.MustCompile(`权利要求\s*(\d+)`)
var claimRangeRe = regexp.MustCompile(`第\s*(\d+)\s*[-至到]\s*(\d+)\s*项`)

// ExtractAffectedClaims collects all claim numbers mentioned in the text,
// expanding ranges like "第1-5项" into individual numbers.
func ExtractAffectedClaims(text string) []int {
	claims := make(map[int]bool)

	for _, m := range claimRe.FindAllStringSubmatch(text, -1) {
		n, _ := strconv.Atoi(m[1])
		if n > 0 {
			claims[n] = true
		}
	}

	for _, m := range claimRangeRe.FindAllStringSubmatch(text, -1) {
		start, _ := strconv.Atoi(m[1])
		end, _ := strconv.Atoi(m[2])
		if start <= end {
			for i := start; i <= end; i++ {
				claims[i] = true
			}
		}
	}

	out := make([]int, 0, len(claims))
	for n := range claims {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// ParseOfficeAction is the top-level entry point that combines all extraction steps.
func ParseOfficeAction(text string) ParsedOfficeAction {
	return ParsedOfficeAction{
		RejectionType:     DetectOaRejectionType(text),
		Citations:         ExtractCitations(text),
		AffectedClaims:    ExtractAffectedClaims(text),
		ExaminerArguments: []string{},
	}
}

// FormatOaSummary renders a ParsedOfficeAction as a human-readable summary string.
func FormatOaSummary(oa ParsedOfficeAction) string {
	claims := "无"
	if len(oa.AffectedClaims) > 0 {
		parts := make([]string, len(oa.AffectedClaims))
		for i, c := range oa.AffectedClaims {
			parts[i] = fmt.Sprintf("%d", c)
		}
		claims = strings.Join(parts, ", ")
	}

	cits := "无"
	if len(oa.Citations) > 0 {
		parts := make([]string, len(oa.Citations))
		for i, c := range oa.Citations {
			parts[i] = c.DocumentNumber
		}
		cits = strings.Join(parts, ", ")
	}

	return fmt.Sprintf("驳回类型: %s\n影响权利要求: %s\n引用文献: %s", oa.RejectionType, claims, cits)
}
