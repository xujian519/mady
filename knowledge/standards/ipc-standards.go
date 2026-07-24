// Package standards provides access to IPC-based patent examination standards
// extracted from the Baochen knowledge base (宝宸知识库). These standards encode
// examination practice for each IPC section × legal article combination, serving
// as structured domain knowledge for the reasoning framework.
//
// Source: 宝宸知识库/复审无效/ — ~138 examination standard cards organized by
// IPC category (A/B/C/E/F/G/H) and legal topic (inventiveness/novelty/
// specification/claims/amendment/design).
package standards

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed ipc-standards.yaml
var standardsYAML []byte

// IPCStandard represents a single IPC examination standard entry.
//
// Each entry maps an IPC field × legal article combination to its key
// examination points and practical tips extracted from the knowledge base.
type IPCStandard struct {
	ID         string   `yaml:"id"         json:"id"`
	Article    string   `yaml:"article"    json:"article"`
	IPCSection string   `yaml:"ipcSection" json:"ipcSection"`
	IPCDetail  string   `yaml:"ipcDetail"  json:"ipcDetail"`
	Name       string   `yaml:"name"       json:"name"`
	KeyPoints  []string `yaml:"keyPoints"  json:"keyPoints"`
	Tips       []string `yaml:"tips"       json:"tips"`
	Source     string   `yaml:"source"     json:"source"`
}

// standardsFile is the top-level YAML structure.
type standardsFile struct {
	Standards []IPCStandard `yaml:"standards"`
}

var (
	once    sync.Once
	loaded  []IPCStandard
	loadErr error
)

// LoadStandards loads all IPC examination standards from the embedded YAML.
// The result is cached after the first call.
func LoadStandards() ([]IPCStandard, error) {
	once.Do(func() {
		var sf standardsFile
		if err := yaml.Unmarshal(standardsYAML, &sf); err != nil {
			loadErr = fmt.Errorf("unmarshal ipc-standards.yaml: %w", err)
			return
		}
		loaded = sf.Standards
	})
	return loaded, loadErr
}

// MustLoadStandards loads standards and panics on error. Convenient for
// one-time initialization in server startup or tests.
func MustLoadStandards() []IPCStandard {
	s, err := LoadStandards()
	if err != nil {
		panic(err)
	}
	return s
}

// FindByIPCSection filters standards by IPC section letter (A/B/C/E/F/G/H/ALL).
// Returns all matching standards, or nil if none match.
func FindByIPCSection(section string) ([]IPCStandard, error) {
	all, err := LoadStandards()
	if err != nil {
		return nil, err
	}
	section = strings.ToUpper(section)
	var result []IPCStandard
	for _, s := range all {
		if s.IPCSection == section || s.IPCSection == "ALL" {
			result = append(result, s)
		}
	}
	return result, nil
}

// FindByArticle filters standards by legal article ID (e.g. "patent-law-a22.3").
// Returns all matching standards, or nil if none match.
func FindByArticle(article string) ([]IPCStandard, error) {
	all, err := LoadStandards()
	if err != nil {
		return nil, err
	}
	var result []IPCStandard
	for _, s := range all {
		if s.Article == article {
			result = append(result, s)
		}
	}
	return result, nil
}

// FindByIPCDetail filters standards by IPC detail code (e.g. "G06", "A61").
// Returns all matching standards, or nil if none match.
func FindByIPCDetail(detail string) ([]IPCStandard, error) {
	all, err := LoadStandards()
	if err != nil {
		return nil, err
	}
	detail = strings.ToUpper(detail)
	var result []IPCStandard
	for _, s := range all {
		if s.IPCDetail == detail || s.IPCDetail == "ALL" {
			result = append(result, s)
		}
	}
	return result, nil
}

// Search searches standards by keyword across names, article IDs, and key points.
// Matching is case-insensitive and partial. Useful for quick lookup in interactive
// contexts or when the caller has a free-text query rather than a structured filter.
func Search(query string) ([]IPCStandard, error) {
	all, err := LoadStandards()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var result []IPCStandard
	for _, s := range all {
		if containsLower(s.Name, q) || containsLower(s.Article, q) {
			result = append(result, s)
			continue
		}
		for _, kp := range s.KeyPoints {
			if containsLower(kp, q) {
				result = append(result, s)
				break
			}
		}
	}
	return result, nil
}

// FormatAsContext formats a list of standards into a text block suitable for
// LLM context injection. The output includes name, legal article, IPC info,
// key examination points, and up to 3 practical tips per standard.
func FormatAsContext(standards []IPCStandard) string {
	if len(standards) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## IPC 审查标准参考\n\n")
	for _, s := range standards {
		b.WriteString(fmt.Sprintf("### %s\n", s.Name))
		b.WriteString(fmt.Sprintf("- 法律条款：%s\n", s.Article))
		b.WriteString(fmt.Sprintf("- IPC：%s (%s)\n", s.IPCDetail, s.IPCSection))
		b.WriteString(fmt.Sprintf("- 来源：%s\n", s.Source))
		if len(s.KeyPoints) > 0 {
			b.WriteString("- 审查要点：\n")
			for _, kp := range s.KeyPoints {
				if kp != "" {
					b.WriteString(fmt.Sprintf("  - %s\n", kp))
				}
			}
		}
		if len(s.Tips) > 0 {
			b.WriteString("- 实务提示：\n")
			for i, t := range s.Tips {
				if i >= 3 {
					b.WriteString(fmt.Sprintf("  - ...等 %d 条提示\n", len(s.Tips)))
					break
				}
				b.WriteString(fmt.Sprintf("  - %s\n", t))
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// containsLower reports whether s contains substr (both lowered).
func containsLower(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
