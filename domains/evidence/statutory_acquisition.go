package evidence

// acquisitionStatutoryContributions 是取证场景轴（正交轴 2）的规则。
func acquisitionStatutoryContributions(in StatutoryEvidenceInput) []statutoryContribution {
	var out []statutoryContribution
	check := func(c statutoryCheck, state declaredBool) {
		out = append(out, c.contribution(state))
	}

	switch in.Acquisition {
	case EvTypeInternetPublication:
		check(statutoryCheck{
			axis: AxisAcquisition, dimension: judgmentAuthenticity, id: "LEGAL-EVI-ACQ-001",
			legalBasis: "专利审查指南第二部分第三章2.1.2.2", severity: "warning",
			deltaTrue: 0.10, deltaFalse: -0.10,
			reasonTrue: "互联网公开时间明确", reasonFalse: "互联网公开时间不明确",
			reasonUnknown: "未声明互联网公开时间是否明确",
		}, boolState(in.PublicDateClear))
		check(statutoryCheck{
			axis: AxisAcquisition, dimension: judgmentAuthenticity, id: "LEGAL-EVI-ACQ-002",
			legalBasis: "专利审查指南第二部分第三章2.1.2.2", severity: "warning",
			deltaTrue: 0.10, deltaFalse: -0.10,
			reasonTrue: "内容处于公众可获取状态", reasonFalse: "内容不处于公众可获取状态",
			reasonUnknown: "未声明内容是否处于公众可获取状态",
		}, boolState(in.PubliclyAccessible))

	case EvTypePublicUse:
		check(statutoryCheck{
			axis: AxisAcquisition, dimension: judgmentAuthenticity, id: "LEGAL-EVI-ACQ-003",
			legalBasis: "专利审查指南第二部分第三章2.1.2.3", severity: "warning",
			deltaTrue: 0.15, deltaFalse: -0.15,
			reasonTrue: "使用公开四要件齐备", reasonFalse: "使用公开四要件不齐备",
			reasonUnknown: "未声明使用公开四要件是否齐备",
		}, boolState(in.PublicUseFourElements))

	case EvTypeOverseas:
		check(statutoryCheck{
			axis: AxisAcquisition, dimension: judgmentLegality, id: "LEGAL-EVI-ACQ-004",
			legalBasis: "最高人民法院关于民事诉讼证据的若干规定第16条",
			severity:   judgmentCritical, deltaTrue: 0.20, deltaFalse: -0.20,
			reasonTrue: "域外证据已办理公证认证手续", reasonFalse: "域外证据未办理公证认证手续",
			reasonUnknown: "未声明域外证据是否办理公证认证手续",
		}, boolState(in.Notarized))

	case EvTypeForeignLang:
		check(statutoryCheck{
			axis: AxisAcquisition, dimension: judgmentLegality, id: "LEGAL-EVI-ACQ-005",
			legalBasis: "民事诉讼法第73条", severity: "warning",
			deltaTrue: 0.15, deltaFalse: -0.20,
			reasonTrue: "外文证据附有中文译本", reasonFalse: "外文证据未附中文译本",
			reasonUnknown: "未声明外文证据是否附有中文译本",
		}, boolState(in.Translated))

	case EvTypeNotarial:
		check(statutoryCheck{
			axis: AxisAcquisition, dimension: judgmentAuthenticity, id: "LEGAL-EVI-ACQ-006",
			legalBasis: "民事诉讼法第72条", severity: "warning",
			deltaTrue: 0.20, deltaFalse: -0.15,
			reasonTrue: "已经法定程序公证证明", reasonFalse: "未经法定程序公证证明",
			reasonUnknown: "未声明是否经过公证证明",
		}, boolState(in.Notarized))
	}
	return out
}
