// Package prompt provides prompt template loading and variable resolution
// for Mady's curated prompt template library.
package prompt

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// PromptTemplate is a curated prompt template loaded from a JSON file in
// the prompt/templates/ directory. Each template describes a reusable
// system + user prompt pair with trigger keywords and variable placeholders.
type PromptTemplate struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Domain      string   `json:"domain"`
	Category    string   `json:"category"`
	Model       string   `json:"model"`
	Triggers    []string `json:"triggers"`

	SystemPrompt       string `json:"system_prompt"`
	UserPromptTemplate string `json:"user_prompt_template"`

	Attribution struct {
		Source  string `json:"source"`
		License string `json:"license"`
	} `json:"attribution"`
}

// ResolvedPrompt contains the fully resolved system and user prompts
// with template variables replaced.
type ResolvedPrompt struct {
	Template     PromptTemplate
	SystemPrompt string
	UserPrompt   string
}

// LoadPrompts reads all prompt templates from the given root directories.
// Templates with the same name keep the first one found.
func LoadPrompts(roots ...string) ([]PromptTemplate, error) {
	var all []PromptTemplate
	seen := make(map[string]bool)
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || filepath.Ext(path) != ".json" {
				return nil
			}
			tmpl, err := loadPromptFile(path)
			if err != nil {
				return fmt.Errorf("prompt-template %s: %w", path, err)
			}
			if seen[tmpl.Name] {
				return nil // first wins
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

// ResolvePrompt fills template variables in a prompt template's user
// prompt. Variables use {{variable}} syntax (handlebars-style).
func ResolvePrompt(tmpl PromptTemplate, vars map[string]string) ResolvedPrompt {
	user := tmpl.UserPromptTemplate
	for key, value := range vars {
		placeholder := "{{" + key + "}}"
		user = strings.ReplaceAll(user, placeholder, value)
	}
	return ResolvedPrompt{
		Template:     tmpl,
		SystemPrompt: tmpl.SystemPrompt,
		UserPrompt:   user,
	}
}

// FindPromptByTrigger returns templates whose trigger list contains the
// given keyword. Matching is case-insensitive.
func FindPromptByTrigger(templates []PromptTemplate, keyword string) []PromptTemplate {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	var matched []PromptTemplate
	for _, tmpl := range templates {
		for _, trigger := range tmpl.Triggers {
			if strings.Contains(strings.ToLower(trigger), keyword) ||
				strings.Contains(keyword, strings.ToLower(trigger)) {
				matched = append(matched, tmpl)
				break
			}
		}
	}
	return matched
}

// FindPromptByName returns the template with the given name, if found.
func FindPromptByName(templates []PromptTemplate, name string) (PromptTemplate, bool) {
	for _, tmpl := range templates {
		if tmpl.Name == name {
			return tmpl, true
		}
	}
	return PromptTemplate{}, false
}

// PromptIndex returns a human-readable index of all templates, grouped by
// category.
func PromptIndex(templates []PromptTemplate) string {
	if len(templates) == 0 {
		return ""
	}
	categories := make(map[string][]PromptTemplate)
	for _, tmpl := range templates {
		categories[tmpl.Category] = append(categories[tmpl.Category], tmpl)
	}
	var b strings.Builder
	b.WriteString("Available prompt templates:\n\n")
	order := []string{"search", "analysis", "drafting", "oa", "disclosure"}
	for _, cat := range order {
		items, ok := categories[cat]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "  [%s]\n", cat)
		for _, tmpl := range items {
			fmt.Fprintf(&b, "    %-30s — %s\n", tmpl.Name, tmpl.Description)
		}
		b.WriteString("\n")
	}
	// Handle any categories not in the predefined order.
	for cat, items := range categories {
		found := false
		for _, o := range order {
			if o == cat {
				found = true
				break
			}
		}
		if found {
			continue
		}
		fmt.Fprintf(&b, "  [%s]\n", cat)
		for _, tmpl := range items {
			fmt.Fprintf(&b, "    %-30s — %s\n", tmpl.Name, tmpl.Description)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func loadPromptFile(path string) (*PromptTemplate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tmpl PromptTemplate
	if err := unmarshalPrompt(data, &tmpl); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &tmpl, nil
}

// unmarshalPrompt parses a single prompt template from JSON and applies
// the same validation used by both disk and embed.FS loaders.
func unmarshalPrompt(data []byte, tmpl *PromptTemplate) error {
	if err := json.Unmarshal(data, tmpl); err != nil {
		return err
	}
	if tmpl.Name == "" {
		return fmt.Errorf("missing name")
	}
	if tmpl.SystemPrompt == "" {
		return fmt.Errorf("missing system_prompt")
	}
	return nil
}

// LoadPromptsFromFS reads all prompt templates from an fs.FS rooted at root.
// It reuses the same parsing rules as LoadPrompts. Templates with duplicate
// names keep the first one found.
func LoadPromptsFromFS(fsys fs.FS, root string) ([]PromptTemplate, error) {
	var all []PromptTemplate
	seen := make(map[string]bool)

	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("prompt-template %s: %w", path, err)
		}
		var tmpl PromptTemplate
		if err := unmarshalPrompt(data, &tmpl); err != nil {
			return fmt.Errorf("prompt-template %s: %w", path, err)
		}
		if seen[tmpl.Name] {
			return nil // first wins
		}
		seen[tmpl.Name] = true
		all = append(all, tmpl)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return all, nil
}
