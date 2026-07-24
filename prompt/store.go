package prompt

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// ListOptions filters prompt template listings.
type ListOptions struct {
	Category string // 按类别筛选（空=全部）
	Domain   string // 按领域筛选（空=全部）
	Query    string // 按 name/title/description 模糊搜索
}

// PromptStore is a thread-safe cache of loaded PromptTemplates. It is
// populated at startup from the embedded template filesystem and optional
// user directories (user templates override embedded ones by name).
type PromptStore struct {
	mu        sync.RWMutex
	templates []PromptTemplate
	byName    map[string]int // name → index in templates
}

// NewPromptStore creates a store by loading embedded templates from the
// prompt package's //go:embed filesystem, then overlaying any templates
// found under userRoots.
func NewPromptStore(userRoots ...string) (*PromptStore, error) {
	store := &PromptStore{
		byName: make(map[string]int),
	}

	// Load embedded templates first.
	embedded, err := LoadPromptsFromFS(embeddedPromptsFS, embeddedPromptsDir)
	if err != nil {
		return nil, fmt.Errorf("prompt: load embedded templates: %w", err)
	}
	for i := range embedded {
		store.add(&embedded[i])
	}

	// Overlay user templates.
	if len(userRoots) > 0 {
		userTemplates, err := LoadPrompts(userRoots...)
		if err != nil {
			slog.Warn("prompt: failed to load user templates, using embedded templates only", "error", err)
		} else {
			for i := range userTemplates {
				store.add(&userTemplates[i])
			}
		}
	}

	return store, nil
}

// add inserts or overwrites a template by name (thread-unsafe; caller must
// hold the lock or call during init).
func (s *PromptStore) add(tmpl *PromptTemplate) {
	if idx, ok := s.byName[tmpl.Name]; ok {
		// Overwrite: user templates take priority.
		s.templates[idx] = *tmpl
	} else {
		s.byName[tmpl.Name] = len(s.templates)
		s.templates = append(s.templates, *tmpl)
	}
}

// FindByName returns the template with the given name. The second return
// value is false when not found.
func (s *PromptStore) FindByName(name string) (PromptTemplate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	idx, ok := s.byName[name]
	if !ok {
		return PromptTemplate{}, false
	}
	return s.templates[idx], true
}

// Resolve fills template variables in the named template's user prompt and
// returns the resolved system and user prompts. The second return value is
// false when the template is not found.
func (s *PromptStore) Resolve(name string, vars map[string]string) (ResolvedPrompt, bool) {
	tmpl, ok := s.FindByName(name)
	if !ok {
		return ResolvedPrompt{}, false
	}
	return ResolvePrompt(tmpl, vars), true
}

// FindByTrigger returns templates whose trigger list contains the given
// keyword. Matching is case-insensitive.
func (s *PromptStore) FindByTrigger(keyword string) []PromptTemplate {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return FindPromptByTrigger(s.templates, keyword)
}

// List returns templates matching the given options. All filters are
// optional — zero-value ListOptions returns all templates.
func (s *PromptStore) List(opts ListOptions) []PromptTemplate {
	s.mu.RLock()
	defer s.mu.RUnlock()

	q := strings.ToLower(strings.TrimSpace(opts.Query))
	var result []PromptTemplate
	for _, t := range s.templates {
		if opts.Category != "" && t.Category != opts.Category {
			continue
		}
		if opts.Domain != "" && t.Domain != opts.Domain {
			continue
		}
		if q != "" {
			if !strings.Contains(strings.ToLower(t.Name), q) &&
				!strings.Contains(strings.ToLower(t.Title), q) &&
				!strings.Contains(strings.ToLower(t.Description), q) {
				continue
			}
		}
		result = append(result, t)
	}
	return result
}

// Count returns the total number of templates in the store.
func (s *PromptStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.templates)
}

// Index returns a human-readable index of all templates in the store,
// grouped by category.
func (s *PromptStore) Index() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return PromptIndex(s.templates)
}
