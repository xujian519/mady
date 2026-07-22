package domains

import (
	"path/filepath"
	"strings"
)

// ClassifyDocument 根据文件名和内容特征判断文档类型。
// 返回 DocConfirmation/DocFiling/DocAcceptance/DocPublication/DocOfficeAction/
// DocGrant/DocRejection/DocOther 之一。
func ClassifyDocument(filename, content string) string {
	name := strings.ToLower(filepath.Base(filename))
	// 取内容前 2000 字符做关键词匹配，避免全文扫描
	preview := strings.ToLower(content)
	if len(preview) > 2000 {
		preview = preview[:2000]
	}

	// --- 官文类（按标题/关键词精确匹配） ---

	officeActionKeywords := []string{
		"审查意见通知书", "第一次审查意见", "office action",
	}
	if containsAny(name, officeActionKeywords) || containsAny(preview, officeActionKeywords) {
		return DocOfficeAction
	}

	grantKeywords := []string{
		"授权通知书", "授予专利权", "办理登记手续通知书",
	}
	if containsAny(name, grantKeywords) || containsAny(preview, grantKeywords) {
		return DocGrant
	}

	rejectionKeywords := []string{
		"驳回决定", "予以驳回",
	}
	if containsAny(name, rejectionKeywords) || containsAny(preview, rejectionKeywords) {
		return DocRejection
	}

	acceptanceKeywords := []string{
		"专利申请受理通知书", "受理通知书",
	}
	if containsAny(name, acceptanceKeywords) || containsAny(preview, acceptanceKeywords) {
		return DocAcceptance
	}

	publicationKeywords := []string{
		"发明专利申请公布", "公布公告", "实用新型专利授权公告",
	}
	if containsAny(name, publicationKeywords) || containsAny(preview, publicationKeywords) {
		return DocPublication
	}

	// --- 确认书 ---
	confirmationKeywords := []string{
		"专利申请确认书", "申请确认书",
	}
	if containsAny(name, confirmationKeywords) || containsAny(preview, confirmationKeywords) {
		return DocConfirmation
	}

	// --- 申请文件（定稿） ---
	// 特征：文件名含"权利要求"/"说明书"/"申请文件"，或内容含权利要求书结构
	filingNameKeywords := []string{
		"权利要求", "说明书", "申请文件", "定稿",
	}
	filingContentKeywords := []string{
		"权利要求书", "说明书", "技术领域", "背景技术",
	}
	if containsAny(name, filingNameKeywords) || containsAll(preview, filingContentKeywords[:2]) {
		return DocFiling
	}

	return DocOther
}

// DocTypeLabel 返回文档类型的中文标签。
func DocTypeLabel(docType string) string {
	switch docType {
	case DocConfirmation:
		return "申请确认书"
	case DocFiling:
		return "申请文件"
	case DocAcceptance:
		return "受理通知书"
	case DocPublication:
		return "公开公告"
	case DocOfficeAction:
		return "审查意见"
	case DocGrant:
		return "授权通知"
	case DocRejection:
		return "驳回决定"
	default:
		return "其他文件"
	}
}

// IsAuthorityDoc 判断文档是否为权威信息来源（影响案件标识或状态）。
func IsAuthorityDoc(docType string) bool {
	switch docType {
	case DocConfirmation, DocFiling, DocAcceptance,
		DocPublication, DocOfficeAction, DocGrant, DocRejection:
		return true
	default:
		return false
	}
}

func containsAny(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

func containsAll(s string, keywords []string) bool {
	for _, kw := range keywords {
		if !strings.Contains(s, kw) {
			return false
		}
	}
	return true
}
