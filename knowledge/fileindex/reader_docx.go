package fileindex

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// docxDocument represents the body of a Word document's document.xml.
type docxDocument struct {
	XMLName xml.Name `xml:"document"`
	Body    docxBody `xml:"body"`
}

type docxBody struct {
	Paragraphs []docxParagraph `xml:"p"`
}

type docxParagraph struct {
	Runs []docxRun `xml:"r"`
}

type docxRun struct {
	Text string `xml:"t"`
}

// readDocx extracts text from a .docx file.
// DOCX is a ZIP archive containing XML; we parse word/document.xml
// for paragraph text and word/header*.xml / word/footer*.xml for headers.
func (fr *FileReader) readDocx(_ context.Context, path string) (*FileReadResult, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("打开 docx 文件失败: %w", err)
	}
	defer r.Close()

	// Extract text from the main document body.
	bodyText := extractDocxBodyText(r.File)
	headerText := extractDocxHeaderFooterText(r.File, "header")
	footerText := extractDocxHeaderFooterText(r.File, "footer")

	var allParts []string
	if headerText != "" {
		allParts = append(allParts, "[页眉]", headerText)
	}
	if bodyText != "" {
		allParts = append(allParts, bodyText)
	}
	if footerText != "" {
		allParts = append(allParts, "[页脚]", footerText)
	}

	fullText := strings.Join(allParts, "\n\n")

	// Split into sections by paragraph.
	paragraphs := strings.Split(fullText, "\n\n")
	sections := make([]Section, 0, len(paragraphs))
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p != "" {
			sections = append(sections, Section{Content: p})
		}
	}

	if fullText == "" {
		return &FileReadResult{
			Content:    "",
			Confidence: 0.5,
			Metadata:   map[string]string{"warning": "未提取到文档内容"},
		}, nil
	}

	return &FileReadResult{
		Content:    fullText,
		Confidence: 1.0,
		Sections:   sections,
		Metadata: map[string]string{
			"chars":    fmt.Sprintf("%d", len(fullText)),
			"sections": fmt.Sprintf("%d", len(sections)),
		},
	}, nil
}

// extractDocxBodyText reads word/document.xml and returns all paragraph text.
func extractDocxBodyText(files []*zip.File) string {
	for _, f := range files {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return ""
			}

			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return ""
			}
			return parseDocxBody(data)
		}
	}
	return ""
}

// parseDocxBody parses word/document.xml body content from raw bytes.
func parseDocxBody(data []byte) string {
	var doc docxDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return ""
	}

	var paragraphs []string
	for _, p := range doc.Body.Paragraphs {
		var line strings.Builder
		for _, r := range p.Runs {
			line.WriteString(r.Text)
		}
		text := strings.TrimSpace(line.String())
		if text != "" {
			paragraphs = append(paragraphs, text)
		}
	}
	return strings.Join(paragraphs, "\n\n")
}

// extractDocxHeaderFooterText reads header/footer XML files and returns text.
func extractDocxHeaderFooterText(files []*zip.File, prefix string) string {
	var allText []string
	for _, f := range files {
		if strings.HasPrefix(f.Name, "word/"+prefix) && strings.HasSuffix(f.Name, ".xml") {
			text := readDocxXMLFile(f)
			if text != "" {
				allText = append(allText, text)
			}
		}
	}
	return strings.Join(allText, "\n\n")
}

// readDocxXMLFile reads paragraph text from a single DOCX XML file.
func readDocxXMLFile(f *zip.File) string {
	rc, err := f.Open()
	if err != nil {
		return ""
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return ""
	}

	var doc docxDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return ""
	}

	var paragraphs []string
	for _, p := range doc.Body.Paragraphs {
		var line strings.Builder
		for _, r := range p.Runs {
			line.WriteString(r.Text)
		}
		text := strings.TrimSpace(line.String())
		if text != "" {
			paragraphs = append(paragraphs, text)
		}
	}
	return strings.Join(paragraphs, "\n\n")
}
