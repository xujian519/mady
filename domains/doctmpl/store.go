package doctmpl

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// ListOptions filters template listings.
type ListOptions struct {
	Category string // 按类别筛选（空=全部）
	Domain   string // 按领域筛选（空=全部）
	Language string // G10: 按语言筛选（空=全部）
	Query    string // G9: 按 name/title/description/use_when 模糊搜索
}

// TemplateStore is a thread-safe cache of loaded DocTemplates plus a
// RendererRegistry. It is populated once at startup from the embedded
// template filesystem and optional user directories (user templates
// override embedded ones by name — first loaded wins).
type TemplateStore struct {
	mu        sync.RWMutex
	templates []DocTemplate
	byName    map[string]int // name → index in templates
	renderers *RendererRegistry

	// embeddedVersions tracks version strings of embedded templates at load
	// time, used for conflict detection (G5).
	embeddedVersions map[string]string
}

// NewTemplateStore creates a store by loading embedded templates from the
// doctmpl package's //go:embed filesystem, then overlaying any templates
// found under userRoots. A default MarkdownRenderer is registered.
func NewTemplateStore(userRoots ...string) (*TemplateStore, error) {
	store := &TemplateStore{
		byName:           make(map[string]int),
		renderers:        NewRendererRegistry(),
		embeddedVersions: make(map[string]string),
	}
	store.renderers.Register(&MarkdownRenderer{})

	// Load embedded templates first.
	embedded, err := LoadDocTemplatesFromFS(embeddedTemplatesFS, embeddedTemplatesDir)
	if err != nil {
		return nil, fmt.Errorf("doctmpl: load embedded templates: %w", err)
	}
	for i := range embedded {
		store.embeddedVersions[embedded[i].Name] = embedded[i].Version
		store.add(&embedded[i])
	}

	// Overlay user templates.
	if len(userRoots) > 0 {
		userTemplates, err := LoadDocTemplates(userRoots...)
		if err != nil {
			return nil, fmt.Errorf("doctmpl: load user templates: %w", err)
		}
		for i := range userTemplates {
			store.add(&userTemplates[i])
		}
	}

	return store, nil
}

// add inserts or overwrites a template by name (thread-unsafe; caller must
// hold the lock or call during init).
func (s *TemplateStore) add(tmpl *DocTemplate) {
	if idx, ok := s.byName[tmpl.Name]; ok {
		// Overwrite: user templates take priority.
		s.templates[idx] = *tmpl
	} else {
		s.byName[tmpl.Name] = len(s.templates)
		s.templates = append(s.templates, *tmpl)
	}
}

// List returns templates matching the given options. All filters are
// optional — zero-value ListOptions returns all templates.
func (s *TemplateStore) List(opts ListOptions) []DocTemplate {
	s.mu.RLock()
	defer s.mu.RUnlock()

	q := strings.ToLower(strings.TrimSpace(opts.Query))
	var result []DocTemplate
	for _, t := range s.templates {
		if opts.Category != "" && t.Category != opts.Category {
			continue
		}
		if opts.Domain != "" && t.Domain != opts.Domain {
			continue
		}
		if opts.Language != "" && t.Language != opts.Language {
			continue
		}
		if q != "" {
			if !strings.Contains(strings.ToLower(t.Name), q) &&
				!strings.Contains(strings.ToLower(t.Title), q) &&
				!strings.Contains(strings.ToLower(t.Description), q) &&
				!strings.Contains(strings.ToLower(t.UseWhen), q) {
				continue
			}
		}
		result = append(result, t)
	}
	return result
}

// FindByName returns the template with the given name. The second return
// value is false when not found.
func (s *TemplateStore) FindByName(name string) (DocTemplate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	idx, ok := s.byName[name]
	if !ok {
		return DocTemplate{}, false
	}
	return s.templates[idx], true
}

// Render resolves variables in the named template and renders it in the
// requested format. It chains FindByName → ResolveDoc → renderer.Render.
func (s *TemplateStore) Render(name string, vars map[string]string, format OutputFormat, meta RenderMeta) ([]byte, error) {
	tmpl, ok := s.FindByName(name)
	if !ok {
		return nil, fmt.Errorf("doctmpl: template %q not found", name)
	}

	// Check format support.
	supported := false
	for _, f := range tmpl.SupportedFormats {
		if f == format {
			supported = true
			break
		}
	}
	if !supported {
		return nil, fmt.Errorf("doctmpl: template %q does not support format %q (supports %v)",
			name, format, tmpl.SupportedFormats)
	}

	resolved := ValidatedResolve(tmpl, vars).Output
	return s.renderers.Render(format, resolved, meta)
}

// RendererRegistry returns the store's renderer registry for external
// registration of additional renderers.
func (s *TemplateStore) RendererRegistry() *RendererRegistry {
	return s.renderers
}

// FindByNameAndLang finds a template by name and optional language filter.
// When lang is empty, behaves identically to FindByName. Note: the current
// store model stores one entry per unique name (user override semantics),
// so multi-language variants are represented as separate templates with
// distinct names (e.g., "method-claim-zh" / "method-claim-en").
func (s *TemplateStore) FindByNameAndLang(name string, lang string) (DocTemplate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	idx, ok := s.byName[name]
	if !ok {
		return DocTemplate{}, false
	}
	tmpl := s.templates[idx]
	if lang != "" && tmpl.Language != lang {
		return DocTemplate{}, false
	}
	return tmpl, true
}

// VersionConflict describes a version discrepancy between embedded and
// user-overridden templates.
type VersionConflict struct {
	TemplateName string // 模板名
	EmbeddedVer  string // 内嵌版本
	UserVer      string // 用户版本
	Severity     string // "info" | "warn"
}

// ListConflicts returns templates where the user's override has a different
// version than the embedded template.
func (s *TemplateStore) ListConflicts() []VersionConflict {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var conflicts []VersionConflict
	for _, t := range s.templates {
		embeddedVer, exists := s.embeddedVersions[t.Name]
		if !exists || t.Version == embeddedVer {
			continue
		}
		sev := "warn"
		if t.Version < embeddedVer {
			sev = "warn"
		} else {
			sev = "info" // user is ahead of or equal to embedded
		}
		conflicts = append(conflicts, VersionConflict{
			TemplateName: t.Name,
			EmbeddedVer:  embeddedVer,
			UserVer:      t.Version,
			Severity:     sev,
		})
	}
	return conflicts
}

// MergedVarContext is the merged variable space of multiple templates with
// shared variables.
type MergedVarContext struct {
	Templates  []DocTemplate   // 参与合并的模板
	SharedVars []string        // 跨模板共享的变量名
	AllVars    []VarDefinition // 去重合并后的全部变量
}

// MergeVarContext merges the variable spaces of the named templates.
// Shared variables (declared via shared_vars in the frontmatter) are
// automatically identified and deduplicated.
func (s *TemplateStore) MergeVarContext(names []string) (*MergedVarContext, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var templates []DocTemplate
	seenVars := make(map[string]bool)
	sharedVars := make(map[string]int) // var name → count across templates

	for _, name := range names {
		idx, ok := s.byName[name]
		if !ok {
			return nil, fmt.Errorf("doctmpl: template %q not found", name)
		}
		tmpl := s.templates[idx]
		templates = append(templates, tmpl)

		if tmpl.VarSchema != nil {
			for _, v := range tmpl.VarSchema.Definitions {
				sharedVars[v.Name]++
			}
		}
		for _, sv := range tmpl.SharedVars {
			// Only increment for shared_vars NOT already in Definitions
			// (avoids double-counting from the same template).
			if tmpl.VarSchema == nil {
				sharedVars[sv]++
			} else if _, inDefs := tmpl.VarSchema.byName[sv]; !inDefs {
				sharedVars[sv]++
			}
		}
	}

	ctx := &MergedVarContext{
		Templates: templates,
	}

	// Variables appearing in multiple templates are shared.
	for name, count := range sharedVars {
		if count > 1 {
			ctx.SharedVars = append(ctx.SharedVars, name)
		}
	}

	// Merge all var definitions (dedup by name, last wins).
	for _, tmpl := range templates {
		if tmpl.VarSchema == nil {
			continue
		}
		for _, d := range tmpl.VarSchema.Definitions {
			seenVars[d.Name] = true
		}
	}

	// Build merged definitions from last template.
	for name := range seenVars {
		for i := len(templates) - 1; i >= 0; i-- {
			if templates[i].VarSchema == nil {
				continue
			}
			if def, ok := templates[i].VarSchema.Get(name); ok {
				ctx.AllVars = append(ctx.AllVars, def)
				break
			}
		}
	}

	return ctx, nil
}

// Count returns the total number of templates in the store.
func (s *TemplateStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.templates)
}

// DocIndex returns a human-readable index of all templates in the store,
// grouped by category.
func (s *TemplateStore) DocIndex() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return DocIndex(s.templates)
}

// tmplSummary is a lightweight view of a template for listing tools.
type tmplSummary struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Domain      string   `json:"domain"`
	Version     string   `json:"version"`
	Language    string   `json:"language,omitempty"`
	Style       string   `json:"style,omitempty"`
	UseWhen     string   `json:"use_when,omitempty"`
	Formats     []string `json:"formats"`
}

func (s *TemplateStore) toSummaries(templates []DocTemplate) []tmplSummary {
	out := make([]tmplSummary, len(templates))
	for i, t := range templates {
		fmts := make([]string, len(t.SupportedFormats))
		for j, f := range t.SupportedFormats {
			fmts[j] = string(f)
		}
		out[i] = tmplSummary{
			Name:        t.Name,
			Title:       t.Title,
			Category:    t.Category,
			Description: t.Description,
			Domain:      t.Domain,
			Version:     t.Version,
			Language:    t.Language,
			Style:       t.StyleName,
			UseWhen:     t.UseWhen,
			Formats:     fmts,
		}
	}
	return out
}

// listResult wraps template summaries for JSON tool output.
type listResult struct {
	Templates []tmplSummary `json:"templates"`
	Count     int           `json:"count"`
}

// renderResult wraps rendered output for JSON tool output.
type renderResult struct {
	Template string `json:"template"`
	Format   string `json:"format"`
	Content  string `json:"content"`
}

// toJSON is a helper that marshals v to a JSON string.
func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"error":"marshal failed"}`
	}
	return string(b)
}
