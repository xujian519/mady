package agentcore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// PluginManifest describes a composable patent/legal workflow plugin.
// Plugins are discovered from the plugins/ directory and consist of a
// SKILL.md (the agent contract) plus a plugin.json (the manifest).
//
// This is the Mady equivalent of Open Design's open-design.json plugin
// system — portable, file-based, composable workflow units.
type PluginManifest struct {
	// Name is the plugin's unique identifier. Must match [a-z0-9]+(-[a-z0-9]+)*.
	Name string `json:"name"`

	// Version is semver (e.g., "0.1.0").
	Version string `json:"version"`

	// Domain is the functional domain this plugin serves.
	// Valid values: patent / legal / assistant / chat
	Domain string `json:"domain"`

	// GuardrailLevel controls the safety guardrail strictness.
	// Valid values: light / standard / strict
	GuardrailLevel string `json:"guardrail_level,omitempty"`

	// Description is a human-readable summary of the workflow.
	Description string `json:"description"`

	// Pipeline describes the ordered stages of the workflow.
	Pipeline PluginPipeline `json:"pipeline"`

	// AllowedSources lists the agent names that may trigger this plugin
	// via Handoff. Empty means "not callable via handoff".
	AllowedSources []string `json:"allowed_sources,omitempty"`

	// HandoffTargets lists agent names this plugin may delegate to.
	HandoffTargets []string `json:"handoff_targets,omitempty"`

	// SkillPath is the relative path to the SKILL.md file within the
	// plugin directory. Auto-detected if empty.
	SkillPath string `json:"skill_path,omitempty"`
}

// PluginPipeline describes a sequence of workflow stages.
type PluginPipeline struct {
	Stages []PluginStage `json:"stages"`
}

// PluginStage is a single step in a plugin workflow pipeline.
type PluginStage struct {
	// ID is a unique stage identifier within the pipeline (e.g., "search").
	ID string `json:"id"`

	// Tool is the tool name used in this stage (e.g., "patent_search", "reasoning").
	// When Atom is set, Tool is optional — the Atom provides the implementation.
	Tool string `json:"tool,omitempty"`

	// Atom references a registered pipeline atom by name. When set, the stage
	// delegates to the atom's implementation rather than a raw tool call.
	// Valid values: search, extract, compare, reasoning, approval-gate.
	Atom string `json:"atom,omitempty"`

	// Description describes what this stage does.
	Description string `json:"description"`
}

var pluginNameRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidatePlugin validates a PluginManifest's fields.
func ValidatePlugin(p PluginManifest) error {
	if p.Name == "" {
		return NewFatalError("plugin", "name is required", nil)
	}
	if len(p.Name) > 64 {
		return NewFatalError("plugin", fmt.Sprintf("name %q exceeds 64 characters", p.Name), nil)
	}
	if !pluginNameRE.MatchString(p.Name) {
		return NewFatalError("plugin", fmt.Sprintf("name %q must match [a-z0-9-]+", p.Name), nil)
	}
	if p.Domain == "" {
		return NewFatalError("plugin", fmt.Sprintf("%q: domain is required", p.Name), nil)
	}
	if p.Description == "" {
		return NewFatalError("plugin", fmt.Sprintf("%q: description is required", p.Name), nil)
	}
	if len(p.Pipeline.Stages) == 0 {
		return NewFatalError("plugin", fmt.Sprintf("%q: pipeline must have at least one stage", p.Name), nil)
	}
	// Validate stage IDs are unique.
	stageIDs := make(map[string]bool)
	for _, s := range p.Pipeline.Stages {
		if s.ID == "" {
			return NewFatalError("plugin", fmt.Sprintf("%q: stage id is required", p.Name), nil)
		}
		if stageIDs[s.ID] {
			return NewFatalError("plugin", fmt.Sprintf("%q: duplicate stage id %q", p.Name, s.ID), nil)
		}
		stageIDs[s.ID] = true
		// Validate atom reference if set.
		if s.Atom != "" && LookupAtom(s.Atom) == nil {
			return NewFatalError("plugin",
				fmt.Sprintf("%q: stage %q references unknown atom %q", p.Name, s.ID, s.Atom), nil)
		}
		if s.Tool == "" && s.Atom == "" {
			return NewFatalError("plugin",
				fmt.Sprintf("%q: stage %q must have either tool or atom set", p.Name, s.ID), nil)
		}
	}
	// Validate guardrail level.
	if p.GuardrailLevel != "" {
		validGuardrailLevelsMu.RLock()
		ok := validGuardrailLevels[p.GuardrailLevel]
		validGuardrailLevelsMu.RUnlock()
		if !ok {
			return NewFatalError("plugin",
				fmt.Sprintf("%q: invalid guardrail_level %q", p.Name, p.GuardrailLevel), nil)
		}
	}
	return nil
}

// LoadPlugin reads a plugin.json file and returns the parsed manifest.
func LoadPlugin(path string) (*PluginManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("plugin: read %s: %w", path, err)
	}
	var p PluginManifest
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("plugin: parse %s: %w", path, err)
	}
	if err := ValidatePlugin(p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ScanPlugins discovers all plugin.json files under the given root
// directories. Plugins with the same name keep the first one found.
func ScanPlugins(roots ...string) ([]PluginManifest, error) {
	var all []PluginManifest
	seen := make(map[string]bool)
	for _, root := range roots {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || filepath.Base(path) != "plugin.json" {
				return nil
			}
			p, err := LoadPlugin(path)
			if err != nil {
				return fmt.Errorf("plugin: %s: %w", path, err)
			}
			if seen[p.Name] {
				return nil // first wins
			}
			seen[p.Name] = true
			// Resolve skill path relative to plugin directory.
			if p.SkillPath == "" {
				p.SkillPath = filepath.Join(filepath.Dir(path), "SKILL.md")
			} else if !filepath.IsAbs(p.SkillPath) {
				p.SkillPath = filepath.Join(filepath.Dir(path), p.SkillPath)
			}
			all = append(all, *p)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return all, nil
}
