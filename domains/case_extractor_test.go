package domains

import "testing"

func TestExtractFromConfirmation(t *testing.T) {
	text := `
专利申请确认书
申请人：张三科技有限公司
发明名称：一种锂电池正极材料及其制备方法
发明人：李四；王五
专利类型：发明专利
2024年3月
`
	info := ExtractFromConfirmation(text)

	if info.ClientName != "张三科技有限公司" {
		t.Errorf("ClientName = %q", info.ClientName)
	}
	if info.PatentTitle != "一种锂电池正极材料及其制备方法" {
		t.Errorf("PatentTitle = %q", info.PatentTitle)
	}
	if info.PatentType != "发明专利" {
		t.Errorf("PatentType = %q", info.PatentType)
	}
	if info.Year != 2024 {
		t.Errorf("Year = %d, want 2024", info.Year)
	}
	if info.SourceDocType != DocConfirmation {
		t.Errorf("SourceDocType = %q", info.SourceDocType)
	}
}

func TestExtractFromAcceptance(t *testing.T) {
	text := `
专利申请受理通知书
申请号：2024101234567
申请日：2024年3月15日
申请人：张三科技
`
	info := ExtractFromAcceptance(text)

	if info.FilingNumber != "2024101234567" {
		t.Errorf("FilingNumber = %q", info.FilingNumber)
	}
	if info.FilingDate != "2024-03-15" {
		t.Errorf("FilingDate = %q", info.FilingDate)
	}
}

func TestExtractFromAcceptance_PCT(t *testing.T) {
	text := `
受理通知书
PCT/CN2024/123456
`
	info := ExtractFromAcceptance(text)
	if info.FilingNumber != "PCT/CN2024/123456" {
		t.Errorf("PCT FilingNumber = %q", info.FilingNumber)
	}
}

func TestExtractFromPublication(t *testing.T) {
	text := `
发明专利申请公布
公开号：CN117890001A
`
	info := ExtractFromPublication(text)
	if info.PublicationNumber != "CN117890001A" {
		t.Errorf("PublicationNumber = %q", info.PublicationNumber)
	}
}

func TestExtractFromOfficeAction(t *testing.T) {
	text := `
审查意见通知书
申请号：2024101234567
权利要求1-3不具备创造性
`
	info := ExtractFromOfficeAction(text)
	if info.OaRejectionType != "inventiveness" {
		t.Errorf("OaRejectionType = %q", info.OaRejectionType)
	}
	if info.FilingNumber != "2024101234567" {
		t.Errorf("FilingNumber = %q", info.FilingNumber)
	}
}

func TestMergeExtractions(t *testing.T) {
	base := ExtractedCaseInfo{
		ClientName:  "张三",
		PatentTitle: "锂电池",
		Year:        2024,
	}
	updates := []ExtractedCaseInfo{
		{FilingNumber: "2024101234567"},
		{PatentType: "发明专利"},
	}
	merged := MergeExtractions(base, updates...)

	if merged.FilingNumber != "2024101234567" {
		t.Errorf("FilingNumber = %q", merged.FilingNumber)
	}
	if merged.PatentType != "发明专利" {
		t.Errorf("PatentType = %q", merged.PatentType)
	}
	if merged.ClientName != "张三" {
		t.Errorf("ClientName should be preserved = %q", merged.ClientName)
	}
}

func TestExtractFromFilingDoc(t *testing.T) {
	text := `
发明名称：一种伸缩支架

权利要求书
1. 一种伸缩支架，其特征在于包括套管。
2. 根据权利要求1所述的支架，其特征在于还包括锁紧机构。

摘要
本发明涉及一种伸缩支架，通过套管和锁紧机构实现长度调节。
`
	info := ExtractFromFilingDoc(text)

	if info.PatentTitle != "一种伸缩支架" {
		t.Errorf("PatentTitle = %q", info.PatentTitle)
	}
	if info.IndependentCount < 1 {
		t.Errorf("IndependentCount = %d, want >= 1", info.IndependentCount)
	}
	if info.Abstract == "" {
		t.Error("Abstract should not be empty")
	}
}
