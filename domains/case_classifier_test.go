package domains

import "testing"

func TestClassifyDocument(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"专利申请确认书.docx", "专利申请确认书\n申请人：张三科技", DocConfirmation},
		{"申请确认书.pdf", "专利申请确认书", DocConfirmation},
		{"权利要求书.docx", "权利要求书\n1. 一种装置，其特征在于", DocFiling},
		{"说明书定稿.docx", "技术领域\n背景技术\n发明内容", DocFiling},
		{"受理通知书.pdf", "专利申请受理通知书\n申请号：2024101234567", DocAcceptance},
		{"第一次审查意见通知书.pdf", "审查意见通知书\n不具备创造性", DocOfficeAction},
		{"授权通知书.pdf", "授权通知书\n办理登记手续", DocGrant},
		{"驳回决定.pdf", "驳回决定\n予以驳回", DocRejection},
		{"readme.txt", "some random content", DocOther},
		{"照片.jpg", "", DocOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyDocument(tt.name, tt.content)
			if got != tt.want {
				t.Errorf("ClassifyDocument(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestClassifyDocument_ContentKeywords(t *testing.T) {
	// File name is generic but content has keywords
	got := ClassifyDocument("scan001.pdf", "第一次审查意见通知书\n申请人不具备创造性")
	if got != DocOfficeAction {
		t.Errorf("content-based classification = %q, want %q", got, DocOfficeAction)
	}
}

func TestDocTypeLabel(t *testing.T) {
	tests := []struct {
		docType string
		want    string
	}{
		{DocConfirmation, "申请确认书"},
		{DocFiling, "申请文件"},
		{DocAcceptance, "受理通知书"},
		{DocOfficeAction, "审查意见"},
		{DocGrant, "授权通知"},
		{DocRejection, "驳回决定"},
		{DocOther, "其他文件"},
	}
	for _, tt := range tests {
		if got := DocTypeLabel(tt.docType); got != tt.want {
			t.Errorf("DocTypeLabel(%q) = %q, want %q", tt.docType, got, tt.want)
		}
	}
}

func TestIsAuthorityDoc(t *testing.T) {
	authority := []string{DocConfirmation, DocFiling, DocAcceptance, DocPublication, DocOfficeAction, DocGrant, DocRejection}
	for _, d := range authority {
		if !IsAuthorityDoc(d) {
			t.Errorf("IsAuthorityDoc(%q) = false, want true", d)
		}
	}
	if IsAuthorityDoc(DocOther) {
		t.Error("IsAuthorityDoc(DocOther) should be false")
	}
}
