package claimdrafting

import (
	"github.com/xujian519/mady/domains/rulekit"
	"regexp"
	"strconv"
	"strings"
)

// Commonly repeated strings in this file.
const (
	genFastener      = "紧固件"
	genDrive         = "驱动装置"
	genElastic       = "弹性元件"
	genTransmission  = "传动件"
	genController    = "控制器"
	genSwitch        = "开关元件"
	genDetector      = "检测装置"
	genFixedJoint    = "固定连接"
	genMetal         = "金属材料"
	genPolymer       = "高分子材料"
	claimIndependent = "independent"
	claimDependent   = "dependent"
)

// generalizationHints 按技术领域组织的上位概念映射表。
// 键为下位具体术语，值为对应的上位概括建议。
// 用于在独立权利要求中检测可上位概括的具体表述。
var generalizationHints = map[string]string{
	// ===== 机械领域（15 组）=====
	"螺栓":   genFastener,
	"螺钉":   genFastener,
	"铆钉":   genFastener,
	"螺母":   genFastener,
	"销钉":   genFastener,
	"电机":   genDrive,
	"马达":   genDrive,
	"气缸":   genDrive,
	"液压缸":  genDrive,
	"电动缸":  genDrive,
	"步进电机": genDrive,
	"伺服电机": genDrive,
	"弹簧":   genElastic,
	"弹片":   genElastic,
	"发条":   genElastic,
	"齿轮":   genTransmission,
	"蜗轮":   genTransmission,
	"皮带轮":  genTransmission,
	"链轮":   genTransmission,
	"轴承":   "支撑件",
	"滚珠":   "滚动体",
	"轴":    "旋转体",
	"转轴":   "旋转体",
	"连杆":   "连接杆件",
	"活塞":   "运动件",
	"壳体":   "外壳体",
	"机壳":   "外壳体",
	"支架":   "支撑架",
	"基座":   "基体",
	"底座":   "基体",

	// ===== 电学领域（10 组）=====
	"单片机":   genController,
	"PLC":   genController,
	"微处理器":  genController,
	"DSP":   genController,
	"FPGA":  genController,
	"电阻":    "阻抗元件",
	"电容":    "容性元件",
	"电感":    "感性元件",
	"变压器":   "电压变换器",
	"继电器":   genSwitch,
	"晶体管":   genSwitch,
	"MOS管":  genSwitch,
	"二极管":   "整流元件",
	"整流器":   "整流元件",
	"逆变器":   "功率变换器",
	"变频器":   "功率变换器",
	"温度传感器": genDetector,
	"压力传感器": genDetector,
	"位移传感器": genDetector,
	"速度传感器": genDetector,

	// ===== 连接/固定方式（5 组）=====
	"焊接":   genFixedJoint,
	"粘接":   genFixedJoint,
	"铆接":   genFixedJoint,
	"螺纹连接": "可拆卸连接",
	"卡扣连接": "可拆卸连接",

	// ===== 材料领域（5 组）=====
	"不锈钢":  genMetal,
	"铝合金":  genMetal,
	"铸铁":   genMetal,
	"尼龙":   genPolymer,
	"聚乙烯":  genPolymer,
	"聚丙烯":  genPolymer,
	"聚氯乙烯": genPolymer,
	"陶瓷":   "无机非金属材料",
	"玻璃":   "无机非金属材料",
	"碳纤维":  "纤维增强材料",
	"玻璃纤维": "纤维增强材料",
}

var numericSpecificPattern = regexp.MustCompile(`\d+\.?\d*\s*(mm|cm|m|μm|nm|℃|°C|MPa|kPa|N|rpm|Hz|V|A|W)`)

// =============================================================================
// 保护范围规则：金字塔型布局提示
// =============================================================================

// scopePyramidRule 提醒申请人采用"从宽到窄"的金字塔型从属权利要求布局策略。
// 依据：审查指南——独立权利要求限定最宽范围，从属权利要求逐层递进限定。
type scopePyramidRule struct{ rulekit.BaseRule }

func (r *scopePyramidRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	depCount := 0
	maxDepth := 0

	for _, c := range claims {
		if c.Kind == claimDependent {
			depCount++
			// 计算引用链深度（通过考察DependsOn是否指向独立权利要求以外的权利要求）
			depth := 1
			for _, dep := range c.DependsOn {
				// 简单估计链深度：如果引用的不是独立权利要求，深度+1
				for _, p := range claims {
					if p.Number == dep {
						if p.Kind != claimIndependent {
							depth++
						}
						break
					}
				}
			}
			if depth > maxDepth {
				maxDepth = depth
			}
		}
	}

	// 从属权利要求太少且没有递进链
	if depCount > 0 && maxDepth <= 1 && depCount >= 3 {
		violations = append(violations, Violation{
			RuleName:    r.Name(),
			RuleBasis:   r.LegalBasis(),
			Severity:    SeverityInfo,
			ClaimNumber: 0,
			Message:     "从属权利要求全部直接引用独立权利要求，缺乏'从宽到窄'的金字塔型多层次保护",
			Suggestion:  "建议构建多层级从属权利要求：第1层限定最关键的改进特征（直接引用独权）；第2层限定附加特征（引用第1层从权）；第3层限定细节特征（引用第2层从权），形成递进保护",
		})
	}

	// 从属权利要求总数过少
	if depCount < 3 && depCount > 0 {
		violations = append(violations, Violation{
			RuleName:    r.Name(),
			RuleBasis:   r.LegalBasis(),
			Severity:    SeverityInfo,
			ClaimNumber: 0,
			Message:     "从属权利要求数量较少（仅" + strconv.Itoa(depCount) + "项），可能无法构建充分的金字塔型保护层次",
			Suggestion:  "建议增加从属权利要求，为区别技术特征的不同实现方式和等同替换预留空间",
		})
	}

	return violations
}

type scopeOverSpecificationRule struct{ rulekit.BaseRule }

func (r *scopeOverSpecificationRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	for _, c := range claims {
		if c.Kind != claimIndependent {
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

type scopeEquivalentsCoverageRule struct{ rulekit.BaseRule }

func (r *scopeEquivalentsCoverageRule) Check(claims []Claim, _ DraftInput) []Violation {
	var violations []Violation
	indCount, depCount, totalHints := 0, 0, 0
	for _, c := range claims {
		if c.Kind == claimIndependent {
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
