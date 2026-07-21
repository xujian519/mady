// Package pluginsys provides portable, file-based plugin manifests for
// composable patent/legal workflow units.
//
// A plugin consists of a plugin.json manifest file plus a SKILL.md agent
// contract. The manifest declares the pipeline stages, domain, guardrail
// level, and handoff permissions.
//
// This package is independent of agentcore — it only handles data types,
// file scanning, and structural validation. agentcore-specific concerns
// (atom lookup, guardrail level registry) are passed via ValidateOptions.
package pluginsys

import (
	"fmt"
	"regexp"
)

// PluginManifest describes a composable patent/legal workflow plugin.
// Plugins are discovered from the plugins/ directory and consist of a
// SKILL.md (the agent contract) plus a plugin.json (the manifest).
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

// ValidateOptions provides agentcore-specific validation hooks.
// When nil, agentcore-specific checks (atom lookup, guardrail level) are
// skipped; only structural validation is performed.
type ValidateOptions struct {
	// AtomLookupFn checks whether a named atom is registered.
	// Called for each stage that has a non-empty Atom field.
	AtomLookupFn func(name string) bool

	// IsValidGuardrailLevel checks whether a guardrail level name is valid.
	IsValidGuardrailLevel func(level string) bool
}

// ValidationError is a plugin validation failure.
type ValidationError struct {
	PluginName string
	Message    string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("plugin %q: %s", e.PluginName, e.Message)
}

// ValidateErrors collects multiple validation errors.
type ValidateErrors []error

func (ve ValidateErrors) Error() string {
	if len(ve) == 1 {
		return ve[0].Error()
	}
	return fmt.Sprintf("plugin: %d validation errors (first: %s)", len(ve), ve[0].Error())
}

// ValidatePlugin validates a PluginManifest's fields.
// opts may be nil for structural-only validation.
// Returns nil on success, or ValidateErrors containing all failures.
func ValidatePlugin(p PluginManifest, opts *ValidateOptions) error {
	var errs ValidateErrors

	if p.Name == "" {
		errs = append(errs, &ValidationError{PluginName: p.Name, Message: "name is required"})
	} else {
		if len(p.Name) > 64 {
			errs = append(errs, &ValidationError{PluginName: p.Name, Message: "name exceeds 64 characters"})
		}
		if !pluginNameRE.MatchString(p.Name) {
			errs = append(errs, &ValidationError{PluginName: p.Name, Message: "name must match [a-z0-9]+(-[a-z0-9]+)*"})
		}
	}

	if p.Domain == "" {
		errs = append(errs, &ValidationError{PluginName: p.Name, Message: "domain is required"})
	}
	if p.Description == "" {
		errs = append(errs, &ValidationError{PluginName: p.Name, Message: "description is required"})
	}
	if len(p.Pipeline.Stages) == 0 {
		errs = append(errs, &ValidationError{PluginName: p.Name, Message: "pipeline must have at least one stage"})
	}

	// Validate stage IDs are unique.
	stageIDs := make(map[string]bool)
	for _, s := range p.Pipeline.Stages {
		if s.ID == "" {
			errs = append(errs, &ValidationError{PluginName: p.Name, Message: "stage id is required"})
			continue
		}
		if stageIDs[s.ID] {
			errs = append(errs, &ValidationError{PluginName: p.Name, Message: fmt.Sprintf("duplicate stage id %q", s.ID)})
		}
		stageIDs[s.ID] = true

		if s.Tool == "" && s.Atom == "" {
			errs = append(errs, &ValidationError{
				PluginName: p.Name,
				Message:    fmt.Sprintf("stage %q must have either tool or atom set", s.ID),
			})
		}

		// Validate atom reference if opts has a lookup function.
		if s.Atom != "" && opts != nil && opts.AtomLookupFn != nil && !opts.AtomLookupFn(s.Atom) {
			errs = append(errs, &ValidationError{
				PluginName: p.Name,
				Message:    fmt.Sprintf("stage %q references unknown atom %q", s.ID, s.Atom),
			})
		}
	}

	// Validate guardrail level if opts has a validation function.
	if p.GuardrailLevel != "" && opts != nil && opts.IsValidGuardrailLevel != nil && !opts.IsValidGuardrailLevel(p.GuardrailLevel) {
		errs = append(errs, &ValidationError{
			PluginName: p.Name,
			Message:    fmt.Sprintf("invalid guardrail_level %q", p.GuardrailLevel),
		})
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}
