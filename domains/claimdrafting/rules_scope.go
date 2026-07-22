package claimdrafting

import (
	"regexp"
	"strings"
)

var generalizationHints = map[string]string{
	"螺栓": "紧固件", "螺钉": "紧固件", "铆钉": "紧固件",
	"电机": "驱动装置", "马达": "驱动装置", "气缸": "驱动装置", "液压缸": "驱动装置",
	"弹簧": "弹性元件", "齿轮": "传动件",
	"单片机": "控制器", "PLC": "控制器", "微处理器": "控制器",
	"焊接": "固定连接", "粘接": "固定连接", "铆接": "固定连接",
	"温度传感器": "检测装置", "压力传感器": "检测装置",
	"不锈钢": "金属材料", "铝合金": "金属材料",
	"尼龙": "高分子材料", "聚乙烯": "高分子材料",
}

var numericSpecificPattern = regexp.MustCompile(`\d+\.?\d*\s*(mm|cm|m|μm|nm|℃|°C|MPa|kPa|N|rpm|Hz|V|A|W)`)

type scopeOverSpecificationRule struct{ baseRule }

func (r *scopeOverSpecificationRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	for _, c := range claims {
		if c.Kind != "independent" {
			continue
		}
		text := c.Preamble + " " + c.Characterized
		if text == "" {
			continue
		}
		var hints []string
		for specific, general := range generalizationHints {
			if strings.Contains(text, specific) {
				hints = append(hints, "可将\""+specific+"\"上位概括为\""+general+"\"")
			}
		}
		if numericSpecificPattern.MatchString(text) {
			hints = append(hints, "检测到具体数值描述，若非必要建议使用范围或相对值")
		}
		if len(hints) > 0 {
			if len(hints) > 3 {
				hints = hints[:3]
			}
			violations = append(violations, Violation{
				RuleName: r.Name(), RuleBasis: r.LegalBasis(),
				Severity: SeverityInfo, ClaimNumber: c.Number,
				Message:    "独立权利要求中存在可上位概括的具体术语，可能不必要地缩小保护范围",
				Suggestion: "建议： " + strings.Join(hints, "；"),
			})
		}
	}
	return violations
}

type scopeEquivalentsCoverageRule struct{ baseRule }

func (r *scopeEquivalentsCoverageRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	indCount, depCount, totalHints := 0, 0, 0
	for _, c := range claims {
		if c.Kind == "independent" {
			indCount++
			for s := range generalizationHints {
				if strings.Contains(c.Preamble+" "+c.Characterized, s) {
					totalHints++
					break
				}
			}
		} else {
			depCount++
		}
	}
	if indCount > 0 && depCount < 3 && totalHints > 0 {
		violations = append(violations, Violation{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity: SeverityInfo, ClaimNumber: 0,
			Message:    "从属权利要求数量较少，可能无法充分覆盖等同替换情形",
			Suggestion: "建议增加从属权利要求，为各技术特征的不同实现方式预留保护空间",
		})
	}
	return violations
}
