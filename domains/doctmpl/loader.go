// Package doctmpl provides patent/legal document template loading and
// variable resolution. Templates are Markdown files with YAML frontmatter
// under doc-templates/, organized by document type.
package doctmpl

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DocTemplate is one parsed document template.
type DocTemplate struct {
	Name        string
	Title       string
	Category    string
	Description string
	Domain      string
	Version     string
	FilePath    string // absolute path to the template file
	Body        string // Markdown body after frontmatter
}

// LoadDocTemplates reads all .md template files from the given root
// directories. Templates with the same name keep the first one found.
func LoadDocTemplates(roots ...string) ([]DocTemplate, error) {
	var all []DocTemplate
	seen := make(map[string]bool)
	for _, root := range roots {
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
func ResolveDoc(tmpl DocTemplate, vars map[string]string) string {
	result := tmpl.Body
	for key, value := range vars {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
	}
	return result
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
	categoryOrder := []string{"claims", "specification", "oa-response", "disclosure"}
	for _, cat := range categoryOrder {
		items, ok := cats[cat]
		if !ok {
			continue
		}
		b.WriteString(fmt.Sprintf("  [%s]\n", cat))
		for _, t := range items {
			b.WriteString(fmt.Sprintf("    %-30s — %s\n", t.Name, t.Description))
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
func parseDocTemplate(path string, data []byte) (*DocTemplate, error) {
	raw := strings.ReplaceAll(string(data), "\r\n", "\n")
	fm, body := extractFrontmatter(raw)
	if fm["name"] == "" {
		return nil, fmt.Errorf("%s: missing name in frontmatter", path)
	}
	return &DocTemplate{
		Name:        fm["name"],
		Title:       fm["title"],
		Category:    fm["category"],
		Description: fm["description"],
		Domain:      fm["domain"],
		Version:     fm["version"],
		FilePath:    path,
		Body:        strings.TrimSpace(body),
	}, nil
}

func extractFrontmatter(raw string) (map[string]string, string) {
	const fence = "---\n"
	if !strings.HasPrefix(raw, fence) {
		return map[string]string{}, raw
	}
	rest := strings.TrimPrefix(raw, fence)
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		end = strings.Index(rest, "\n---")
	}
	if end < 0 {
		return map[string]string{}, raw
	}
	header := rest[:end]
	body := rest[end:]
	if strings.HasPrefix(body, "\n---\n") {
		body = body[5:]
	} else if strings.HasPrefix(body, "\n---") {
		body = body[4:]
	}
	fields := parseSimpleYAML(header)
	return fields, body
}

func parseSimpleYAML(header string) map[string]string {
	fields := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(header))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, val, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.ToLower(key))
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		fields[key] = val
	}
	return fields
}
