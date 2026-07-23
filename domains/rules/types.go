package rules

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Severity rates how critical a rule is.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityMajor    Severity = "major"
	SeverityMinor    Severity = "minor"
)

// Action defines what should happen when a rule check fails.
type Action string

const (
	ActionBlock  Action = "block"
	ActionReview Action = "review"
	ActionWarn   Action = "warn"
)

// Rule is a single patent/legal compliance rule loaded from YAML.
type Rule struct {
	RuleID      string         `yaml:"ruleId"`
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	LegalBasis  string         `yaml:"legalBasis"`
	Domain      string         `yaml:"domain"`
	Severity    Severity       `yaml:"severity"`
	Action      Action         `yaml:"action"`
	Check       Check          `yaml:"check"`
	Extra       map[string]any `yaml:"-"`
}

// knownRuleFields lists the top-level YAML keys that are mapped to typed Rule struct fields.
var knownRuleFields = []string{
	"ruleId", "name", "description", "legalBasis",
	"domain", "severity", "action", "check",
}

// UnmarshalYAML implements custom two-pass decoding: known fields populate
// the typed struct, remaining fields (e.g. evidenceAssessment, evidenceType)
// are captured in Extra.
func (r *Rule) UnmarshalYAML(value *yaml.Node) error {
	type plain Rule
	if err := value.Decode((*plain)(r)); err != nil {
		return err
	}
	var raw map[string]any
	if err := value.Decode(&raw); err != nil {
		return err
	}
	for _, k := range knownRuleFields {
		delete(raw, k)
	}
	if len(raw) > 0 {
		r.Extra = raw
	}
	return nil
}

// Check holds the rule's checking logic. The YAML schema is intentionally
// heterogeneous — different rule types use different sub-fields — so known
// fields are typed while everything else is preserved in Extra for the
// consumer (LLM or reasoning engine) to interpret.
type Check struct {
	Type         string            `yaml:"type"`
	Method       string            `yaml:"method,omitempty"`
	Principles   []string          `yaml:"principles,omitempty"`
	Rules        []string          `yaml:"rules,omitempty"`
	Conditions   []string          `yaml:"conditions,omitempty"`
	Requirements []string          `yaml:"requirements,omitempty"`
	Scope        []string          `yaml:"scope,omitempty"`
	Notes        []string          `yaml:"notes,omitempty"`
	Assessment   map[string]string `yaml:"assessment,omitempty"`
	Extra        map[string]any    `yaml:"-"`
}

// UnmarshalYAML implements custom two-pass decoding: known fields populate
// the typed struct, remaining fields are captured in Extra.
func (c *Check) UnmarshalYAML(value *yaml.Node) error {
	type plain Check
	if err := value.Decode((*plain)(c)); err != nil {
		return err
	}
	var raw map[string]any
	if err := value.Decode(&raw); err != nil {
		return err
	}
	for _, k := range []string{
		"type", "method", "principles", "rules", "conditions",
		"requirements", "scope", "notes", "assessment",
	} {
		delete(raw, k)
	}
	if len(raw) > 0 {
		c.Extra = raw
	}
	return nil
}

// ArticleStep is one step in a multi-step article judgment framework.
type ArticleStep struct {
	ID           string            `yaml:"id"`
	Order        int               `yaml:"order"`
	Name         string            `yaml:"name"`
	RuleRef      string            `yaml:"ruleRef"`
	InputHint    string            `yaml:"inputHint"`
	OutputSchema map[string]string `yaml:"outputSchema"`
}

// ArticleFramework is a Layer-1 法条级原子单元 — the step-by-step judgment
// framework for a specific patent law article.
type ArticleFramework struct {
	ArticleID        string            `yaml:"articleId"`
	Name             string            `yaml:"name"`
	LawRef           string            `yaml:"lawRef"`
	GuidelineRef     string            `yaml:"guidelineRef,omitempty"`
	Steps            []ArticleStep     `yaml:"steps"`
	ConclusionSchema map[string]string `yaml:"conclusionSchema"`
	ApplicableTo     []string          `yaml:"applicableTo"`
}

// DiscoveryStage is a phase in the case discovery process.
type DiscoveryStage struct {
	Name        string   `yaml:"name"`
	Goal        string   `yaml:"goal"`
	Suggestions []string `yaml:"suggestions"`
}

// AvailableArticle links an article framework into an orchestration.
type AvailableArticle struct {
	ArticleID   string `yaml:"articleId"`
	Priority    int    `yaml:"priority"`
	Description string `yaml:"description"`
}

// ExecutionTemplate describes the output artifact structure.
type ExecutionTemplate struct {
	ArtifactType string   `yaml:"artifactType"`
	Sections     []string `yaml:"sections"`
}

// Orchestration is a Layer-2 事务级编排 — the complete workflow for a
// patent/legal business transaction (e.g. invalidation, infringement).
type Orchestration struct {
	ID                string             `yaml:"id"`
	Name              string             `yaml:"name"`
	CaseType          string             `yaml:"caseType"`
	Description       string             `yaml:"description"`
	DiscoveryStages   []DiscoveryStage   `yaml:"discoveryStages"`
	AvailableArticles []AvailableArticle `yaml:"availableArticles"`
	ExecutionTemplate ExecutionTemplate  `yaml:"executionTemplate"`
}

// ReflectionDomain holds error/uncertainty indicator phrases for one domain.
type ReflectionDomain struct {
	Description string   `yaml:"description"`
	Phrases     []string `yaml:"phrases"`
	Patterns    []string `yaml:"patterns,omitempty"`
}

// RuleFile is the top-level container for rule YAML files.
type ruleFile struct {
	Rules []Rule `yaml:"rules"`
}

// reflectionFile is the top-level container for reflection-indicators.yaml.
type reflectionFile map[string]ReflectionDomain

// String returns a human-readable summary of the rule.
func (r Rule) String() string {
	return fmt.Sprintf("[%s] %s: %s (%s)", r.RuleID, r.Name, r.Severity, r.Action)
}
