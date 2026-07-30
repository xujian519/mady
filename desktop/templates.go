//go:build darwin

package main

// templates.go — 文档模板库绑定。
//
// 扫描 doc-templates/ 目录，解析 Markdown 模板文件的名称/描述/分类/内容，
// 供前端 TemplatesView 渲染。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DocTemplateEntry 描述一个文档模板。
type DocTemplateEntry struct {
	Name          string `json:"name"`
	Category      string `json:"category"`
	CategoryLabel string `json:"categoryLabel"`
	Description   string `json:"description"`
	Content       string `json:"content"`
}

// categoryLabels 将目录名映射为中文显示名。
var categoryLabels = map[string]string{
	"claims":        "权利要求书",
	"specification": "说明书",
	"disclosure":    "技术交底书",
	"oa-response":   "OA答复函",
	"legal":         "法律分析",
}

// ListDocTemplates 扫描项目 doc-templates/ 目录，返回所有模板。
func (a *App) ListDocTemplates() ([]DocTemplateEntry, error) {
	cwd, err := a.resolveProjectDir()
	if err != nil {
		return nil, fmt.Errorf("listDocTemplates: %w", err)
	}

	templatesDir := filepath.Join(cwd, "doc-templates")
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []DocTemplateEntry{}, nil
		}
		return nil, fmt.Errorf("listDocTemplates: %w", err)
	}

	var result []DocTemplateEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cat := entry.Name()
		catLabel := categoryLabels[cat]
		if catLabel == "" {
			catLabel = cat
		}

		catDir := filepath.Join(templatesDir, cat)
		files, err := os.ReadDir(catDir)
		if err != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir() {
				continue
			}
			if !strings.HasSuffix(f.Name(), ".md") && !strings.HasSuffix(f.Name(), ".markdown") {
				continue
			}

			path := filepath.Join(catDir, f.Name())
			data, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				continue
			}

			content := string(data)
			name := stripTemplateExtension(f.Name())
			desc := extractDescription(content, name)

			result = append(result, DocTemplateEntry{
				Name:          name,
				Category:      cat,
				CategoryLabel: catLabel,
				Description:   desc,
				Content:       content,
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Category != result[j].Category {
			return result[i].Category < result[j].Category
		}
		return result[i].Name < result[j].Name
	})

	if result == nil {
		return []DocTemplateEntry{}, nil
	}
	return result, nil
}

// stripTemplateExtension 去掉文件名后缀，将连字符转为空格以便展示。
func stripTemplateExtension(name string) string {
	name = strings.TrimSuffix(name, ".md")
	name = strings.TrimSuffix(name, ".markdown")
	// 去掉 .example 后缀
	name = strings.TrimSuffix(name, ".example")
	// 连字符 → 空格
	name = strings.ReplaceAll(name, "-", " ")
	return name
}

// extractDescription 从模板内容中提取描述（取首个段落或标题）。
func extractDescription(content string, fallback string) string {
	// 尝试提取 YAML frontmatter 中的 description 字段
	if strings.HasPrefix(content, "---") {
		if idx := strings.Index(content[3:], "---"); idx >= 0 {
			frontmatter := content[3 : 3+idx]
			for _, line := range strings.Split(frontmatter, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "description:") {
					desc := strings.TrimSpace(line[len("description:"):])
					desc = strings.Trim(desc, `"'`)
					if desc != "" {
						return desc
					}
				}
			}
		}
	}
	// 尝试提取第一个 Markdown 标题后的内容
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") && i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			if next != "" && !strings.HasPrefix(next, "#") {
				if len(next) > 120 {
					next = next[:120] + "…"
				}
				return next
			}
		}
	}
	return fallback
}
