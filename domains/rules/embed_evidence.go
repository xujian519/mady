package rules

import (
	_ "embed"
)

//go:embed data/rules/evidence-rules.yaml
var evidenceRulesYAML []byte

// EvidenceRulesYAML returns the embedded evidence rules YAML content for use
// by the evidence judgment engine. Returns nil if the YAML is empty.
func EvidenceRulesYAML() []byte {
	return evidenceRulesYAML
}
