package patent

import (
	"fmt"
	"strings"
)

// extractClaimsFromText parses claim text, identifying individual claims and
// whether they are independent or dependent.
func extractClaimsFromText(text string) []InvClaimNode {
	var claims []InvClaimNode

	// Split by standard claim numbering patterns: "1.", "2.", "权利要求1"
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 10 {
			continue
		}

		// Match "1." or "权利要求1" at the start.
		claimNum := 0
		if strings.HasPrefix(line, "权利要求") {
			// Extract number after "权利要求"
			rest := line[12:] // len("权利要求") in bytes (3 chars × 3 bytes + possible space)
			rest = strings.TrimLeft(rest, " ：:、")
			if _, err := fmt.Sscanf(rest, "%d", &claimNum); err != nil {
				continue
			}
		} else if len(line) > 2 && line[0] >= '1' && line[0] <= '9' && (line[1] == '.' || line[1] == ' ' || strings.HasPrefix(line[1:], "、")) {
			if _, err := fmt.Sscanf(line, "%d", &claimNum); err != nil {
				continue
			}
		}

		if claimNum > 0 {
			isIndependent := !strings.HasPrefix(line, "权利要求"+fmt.Sprintf("%d", claimNum)+"引用") &&
				!strings.Contains(line[:min(len(line), 30)], "根据") &&
				!strings.Contains(line[:min(len(line), 30)], "如权利要求")
			claims = append(claims, InvClaimNode{
				Number:        claimNum,
				IsIndependent: isIndependent,
				Text:          line,
			})
		}
	}

	// If no structured claims found, treat entire input as claim 1.
	if len(claims) == 0 {
		claims = append(claims, InvClaimNode{
			Number:        1,
			IsIndependent: true,
			Text:          truncate(text, 500),
		})
	}
	return claims
}

// invalidationGroundRules is the pattern table for invalidation ground
// identification. Order matters: earlier entries take priority on overlap.
var invalidationGroundRules = []groundPattern{
	{TypeKey: string(GroundNovelty), Article: "专利法第22条第2款",
		Desc:     "新颖性无效（不具备新颖性）",
		Patterns: []string{"22条第2款", "22.2", termNovelty, "不具备新颖"}},
	{TypeKey: string(GroundInventiveness), Article: "专利法第22条第3款",
		Desc:     "创造性无效（不具备创造性）",
		Patterns: []string{"22条第3款", "22.3", termInventiveness, "不具备创造"}},
	{TypeKey: string(GroundDisclosure), Article: "专利法第26条第3款",
		Desc:     "公开不充分无效",
		Patterns: []string{"26条第3款", "26.3", "公开充分", termSufficientDisclosure, "能够实现"}},
	{TypeKey: string(GroundClaimClarity), Article: "专利法第26条第4款",
		Desc:     "权利要求不清楚/得不到支持无效",
		Patterns: []string{"26条第4款", "26.4", "清楚", "支持"}},
	{TypeKey: string(GroundAmendment), Article: "专利法第33条",
		Desc:     "修改超范围无效",
		Patterns: []string{"第33条", "A33", termAmendmentExceed, "超出原"}},
}

// identifyInvalidationGrounds scans the input text for invalidation ground
// references (e.g. "第22条第2款", "A22.2", termNovelty, termInventiveness, "公开充分").
func identifyInvalidationGrounds(text string) []InvGround {
	matched := scanGrounds(text, invalidationGroundRules)
	var grounds []InvGround
	for _, r := range matched {
		grounds = append(grounds, InvGround{
			Type:        InvalidationGroundType(r.TypeKey),
			Article:     r.Article,
			Description: r.Desc,
		})
	}

	// If no specific grounds found, default to comprehensive analysis.
	if len(grounds) == 0 {
		grounds = append(grounds, InvGround{
			Type:        GroundNovelty,
			Article:     "专利法第22条第2款",
			Description: "新颖性无效（默认分析维度）",
		})
	}

	return grounds
}
