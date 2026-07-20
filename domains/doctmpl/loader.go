// Package doctmpl provides patent/legal document template loading and
// variable resolution. Templates are Markdown files with YAML frontmatter
// under doc-templates/, organized by document type.
package doctmpl

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DocTemplate is one parsed document template.
type DocTemplate struct {
	Name             string           // frontmatter name
	Title            string           // 中文标题
	Category         string           // claims/specification/oa-response/disclosure/legal
	Description      string           // 一行描述
	Domain           string           // patent/legal
	Version          string           // semver
	Language         string           // G10: zh-CN/en-US（空=默认中文）
	StyleName        string           // G3: 关联风格名
	UseWhen          string           // G9: 适用场景描述
	SupportedFormats []OutputFormat   // G1: 支持的输出格式
	VarSchema        *VarSchema       // G4: 变量约束
	Changelog        []TemplateChange // G5: 变更历史
	SharedVars       []string         // G7: 跨模板共享变量
	Extends          []string         // G7: 继承的模板名
	FilePath         string           // absolute path to the template file
	Body             string           // Markdown body after frontmatter
}

// TemplateChange records a single version change entry.
type TemplateChange struct {
	Version     string `yaml:"version"`
	Date        string `yaml:"date"`
	Description string `yaml:"description"`
}

// templateFrontmatter is the structured YAML frontmatter parsed by yaml.v3.
type templateFrontmatter struct {
	Name        string           `yaml:"name"`
	Title       string           `yaml:"title"`
	Category    string           `yaml:"category"`
	Description string           `yaml:"description"`
	Domain      string           `yaml:"domain"`
	Version     string           `yaml:"version"`
	Language    string           `yaml:"language"`
	Style       string           `yaml:"style"`
	UseWhen     string           `yaml:"use_when"`
	Formats     []string         `yaml:"formats"`
	Vars        []VarDefinition  `yaml:"vars"`
	Changelog   []TemplateChange `yaml:"changelog"`
	SharedVars  []string         `yaml:"shared_vars"`
	Extends     []string         `yaml:"extends"`
}

// LoadDocTemplates reads all .md template files from the given root
// directories. Templates with the same name keep the first one found.
func LoadDocTemplates(roots ...string) ([]DocTemplate, error) {
	var all []DocTemplate
	seen := make(map[string]bool)
	for _, root := range roots {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || filepath.Ext(path) != ".md" {
				return nil
			}
			tmpl, err := loadDocFile(path)
			if err != nil {
				return fmt.Errorf("doc-template %s: %w", path, err)
			}
			if seen[tmpl.Name] {
				return nil
			}
			seen[tmpl.Name] = true
			all = append(all, *tmpl)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return all, nil
}

// LoadDocTemplatesFromFS reads all .md template files from an fs.FS rooted
// at root. The root should be the directory containing template subdirectories.
func LoadDocTemplatesFromFS(fsys fs.FS, root string) ([]DocTemplate, error) {
	var all []DocTemplate
	seen := make(map[string]bool)

	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		data, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		tmpl, err := parseDocTemplate(path, data)
		if err != nil {
			return err
		}
		if seen[tmpl.Name] {
			return nil
		}
		seen[tmpl.Name] = true
		all = append(all, *tmpl)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].Name < all[j].Name
	})
	return all, nil
}

// ResolveDoc replaces {{variable}} placeholders in the template body.
// Uses strings.NewReplacer for single-pass O(n) replacement.
func ResolveDoc(tmpl DocTemplate, vars map[string]string) string {
	if len(vars) == 0 {
		return tmpl.Body
	}
	pairs := make([]string, 0, len(vars)*2)
	for key, value := range vars {
		pairs = append(pairs, "{{"+key+"}}", value)
	}
	return strings.NewReplacer(pairs...).Replace(tmpl.Body)
}

// FindDocByCategory returns templates matching the given category.
func FindDocByCategory(templates []DocTemplate, category string) []DocTemplate {
	var matched []DocTemplate
	for _, t := range templates {
		if t.Category == category {
			matched = append(matched, t)
		}
	}
	return matched
}

// DocIndex returns a human-readable index grouped by category, in a
// deterministic order (claims → specification → oa-response → disclosure).
func DocIndex(templates []DocTemplate) string {
	if len(templates) == 0 {
		return ""
	}
	cats := make(map[string][]DocTemplate)
	for _, t := range templates {
		cats[t.Category] = append(cats[t.Category], t)
	}
	var b strings.Builder
	b.WriteString("Available document templates:\n\n")
	categoryOrder := []string{"claims", "specification", "oa-response", "disclosure", "legal"}
	for _, cat := range categoryOrder {
		items, ok := cats[cat]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "  [%s]\n", cat)
		for _, t := range items {
			fmt.Fprintf(&b, "    %-30s — %s\n", t.Name, t.Description)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func loadDocFile(path string) (*DocTemplate, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	return parseDocTemplate(abs, data)
}

// parseDocTemplate parses a single .md template from raw bytes and path.
// Frontmatter is parsed with yaml.v3 for full struct support (vars, formats, etc.).
func parseDocTemplate(path string, data []byte) (*DocTemplate, error) {
	raw := strings.ReplaceAll(string(data), "\r\n", "\n")
	header, body := extractFrontmatterRaw(raw)

	var fm templateFrontmatter
	if err := yaml.Unmarshal([]byte(header), &fm); err != nil {
		return nil, fmt.Errorf("%s: parse frontmatter: %w", path, err)
	}
	if fm.Name == "" {
		return nil, fmt.Errorf("%s: missing name in frontmatter", path)
	}

	formats := make([]OutputFormat, 0, len(fm.Formats))
	for _, f := range fm.Formats {
		of := OutputFormat(strings.TrimSpace(f))
		if of.IsValid() {
			formats = append(formats, of)
		}
	}
	if len(formats) == 0 {
		formats = []OutputFormat{FormatMarkdown}
	}

	return &DocTemplate{
		Name:             fm.Name,
		Title:            fm.Title,
		Category:         fm.Category,
		Description:      fm.Description,
		Domain:           fm.Domain,
		Version:          fm.Version,
		Language:         fm.Language,
		StyleName:        fm.Style,
		UseWhen:          fm.UseWhen,
		SupportedFormats: formats,
		VarSchema:        NewVarSchema(fm.Vars),
		Changelog:        fm.Changelog,
		SharedVars:       fm.SharedVars,
		Extends:          fm.Extends,
		FilePath:         path,
		Body:             strings.TrimSpace(body),
	}, nil
}

// extractFrontmatterRaw returns the raw YAML header string and body.
func extractFrontmatterRaw(raw string) (string, string) {
	const fence = "---\n"
	if !strings.HasPrefix(raw, fence) {
		return "", raw
	}
	rest := strings.TrimPrefix(raw, fence)
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		end = strings.Index(rest, "\n---")
	}
	if end < 0 {
		return "", raw
	}
	header := rest[:end]
	body := rest[end:]
	if strings.HasPrefix(body, "\n---\n") {
		body = body[5:]
	} else if strings.HasPrefix(body, "\n---") {
		body = body[4:]
	}
	return header, body
}
