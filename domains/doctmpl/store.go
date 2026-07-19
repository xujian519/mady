package doctmpl

import (
	"encoding/json"
	"fmt"
	"sync"
)

// ListOptions filters template listings.
type ListOptions struct {
	Category string // 按类别筛选（空=全部）
	Domain   string // 按领域筛选（空=全部）
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
}

// NewTemplateStore creates a store by loading embedded templates from the
// doctmpl package's //go:embed filesystem, then overlaying any templates
// found under userRoots. A default MarkdownRenderer is registered.
func NewTemplateStore(userRoots ...string) (*TemplateStore, error) {
	store := &TemplateStore{
		byName:    make(map[string]int),
		renderers: NewRendererRegistry(),
	}
	store.renderers.Register(&MarkdownRenderer{})

	// Load embedded templates first.
	embedded, err := LoadDocTemplatesFromFS(embeddedTemplatesFS, embeddedTemplatesDir)
	if err != nil {
		return nil, fmt.Errorf("doctmpl: load embedded templates: %w", err)
	}
	for i := range embedded {
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

// List returns templates matching the given options. Both filters are
// optional — zero-value ListOptions returns all templates.
func (s *TemplateStore) List(opts ListOptions) []DocTemplate {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []DocTemplate
	for _, t := range s.templates {
		if opts.Category != "" && t.Category != opts.Category {
			continue
		}
		if opts.Domain != "" && t.Domain != opts.Domain {
			continue
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

	resolved := ResolveDoc(tmpl, vars)
	return s.renderers.Render(format, resolved, meta)
}

// RendererRegistry returns the store's renderer registry for external
// registration of additional renderers.
func (s *TemplateStore) RendererRegistry() *RendererRegistry {
	return s.renderers
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
