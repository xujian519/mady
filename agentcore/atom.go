// Package agentcore provides the Atom abstraction — composable, reusable
// pipeline atoms for patent/legal workflows. Each atom represents a single
// well-defined operation that can be composed into pipelines.
//
// Inspired by Open Design's plugin system but adapted for the Pregel-based
// Mady architecture. Atoms decouple workflow stage definitions from their
// implementations, enabling plugin authors to reference standard operations
// by name rather than specifying tool-level details.
package agentcore

import (
	"fmt"
	"sort"
	"sync"
)

// Atom is a composable, reusable pipeline unit. Each atom represents a
// single well-defined operation in a patent/legal workflow (search, extract,
// compare, reason, gate). Atoms are registered by name and referenced from
// PluginStage.Atom, decoupling workflow definitions from implementations.
//
// Design note: Atom intentionally does not include an Execute method. It is
// a schema-and-metadata contract used by PluginStage validation, tool
// dispatch, and CLI documentation. Execution is handled by the existing
// tool call framework and Pregel nodes, which map atom names to concrete
// implementations at runtime. This separation keeps atoms as lightweight
// declarative references rather than coupling them to a specific execution
// model.
type Atom interface {
	// Name returns the registered identifier (e.g., "search", "extract").
	Name() string

	// Description returns a human-readable summary of what this atom does.
	Description() string

	// Category returns the functional group (search, extract, compare, reason, gate).
	Category() string

	// InputSchema lists the input keys this atom expects.
	InputSchema() []string

	// OutputSchema lists the output keys this atom produces.
	OutputSchema() []string
}

// AtomCategories enumerates the standard atom categories.
const (
	AtomCategorySearch  = "search"
	AtomCategoryExtract = "extract"
	AtomCategoryCompare = "compare"
	AtomCategoryReason  = "reason"
	AtomCategoryGate    = "gate"
)

// =============================================================================
// Concrete Atoms
// =============================================================================

// searchAtom performs patent/legal prior-art searching.
type searchAtom struct{}

func (a searchAtom) Name() string { return "search" }
func (a searchAtom) Description() string {
	return "Patent/legal prior-art search using keywords, IPC, date ranges, and semantic similarity"
}
func (a searchAtom) Category() string { return AtomCategorySearch }
func (a searchAtom) InputSchema() []string {
	return []string{"query", "keywords", "domain", "max_results"}
}
func (a searchAtom) OutputSchema() []string { return []string{"prior_art", "search_summary"} }

// extractAtom performs structured information extraction from text.
type extractAtom struct{}

func (a extractAtom) Name() string { return "extract" }
func (a extractAtom) Description() string {
	return "Structured extraction of technical features, problems, effects from patent text"
}
func (a extractAtom) Category() string      { return AtomCategoryExtract }
func (a extractAtom) InputSchema() []string { return []string{"text", "extraction_type", "domain"} }
func (a extractAtom) OutputSchema() []string {
	return []string{"features", "problems", "effects", "extraction_result"}
}

// compareAtom performs feature-by-feature comparison (claim chart).
type compareAtom struct{}

func (a compareAtom) Name() string { return "compare" }
func (a compareAtom) Description() string {
	return "Feature-by-feature comparison between a claim and prior art, producing a claim chart"
}
func (a compareAtom) Category() string { return AtomCategoryCompare }
func (a compareAtom) InputSchema() []string {
	return []string{"claim", "prior_art", "comparison_scope"}
}
func (a compareAtom) OutputSchema() []string {
	return []string{"claim_chart", "diff_features", "similarity_score"}
}

// reasoningAtom performs three-step patentability reasoning.
type reasoningAtom struct{}

func (a reasoningAtom) Name() string { return "reasoning" }
func (a reasoningAtom) Description() string {
	return "Three-step patentability reasoning: closest prior art → difference → non-obviousness"
}
func (a reasoningAtom) Category() string      { return AtomCategoryReason }
func (a reasoningAtom) InputSchema() []string { return []string{"claim", "prior_art_list", "domain"} }
func (a reasoningAtom) OutputSchema() []string {
	return []string{"closest_prior_art", "distinguishing_features", "non_obviousness_rationale", "conclusion"}
}

// approvalGateAtom implements a human-in-the-loop approval gate.
type approvalGateAtom struct{}

func (a approvalGateAtom) Name() string { return "approval-gate" }
func (a approvalGateAtom) Description() string {
	return "Human-in-the-loop approval gate that pauses workflow for manual review before proceeding"
}
func (a approvalGateAtom) Category() string { return AtomCategoryGate }
func (a approvalGateAtom) InputSchema() []string {
	return []string{"decision", "review_context", "guardrail_level"}
}
func (a approvalGateAtom) OutputSchema() []string {
	return []string{"approved", "rejection_reason", "modified_content"}
}

// =============================================================================
// Atom Registry
// =============================================================================

var (
	atomRegistry   = make(map[string]Atom)
	atomRegistryMu sync.RWMutex
)

// init registers all built-in atoms.
func init() {
	RegisterAtom(searchAtom{})
	RegisterAtom(extractAtom{})
	RegisterAtom(compareAtom{})
	RegisterAtom(reasoningAtom{})
	RegisterAtom(approvalGateAtom{})
}

// RegisterAtom adds an atom to the global registry.
// Duplicate names silently overwrite the previous registration.
func RegisterAtom(a Atom) {
	atomRegistryMu.Lock()
	defer atomRegistryMu.Unlock()
	atomRegistry[a.Name()] = a
}

// LookupAtom returns the registered atom by name, or nil if not found.
func LookupAtom(name string) Atom {
	atomRegistryMu.RLock()
	defer atomRegistryMu.RUnlock()
	return atomRegistry[name]
}

// ListAtoms returns all registered atoms sorted by name.
func ListAtoms() []Atom {
	atomRegistryMu.RLock()
	defer atomRegistryMu.RUnlock()
	atoms := make([]Atom, 0, len(atomRegistry))
	for _, a := range atomRegistry {
		atoms = append(atoms, a)
	}
	sort.Slice(atoms, func(i, j int) bool {
		return atoms[i].Name() < atoms[j].Name()
	})
	return atoms
}

// ListAtomsByCategory returns registered atoms in the given category.
func ListAtomsByCategory(category string) []Atom {
	atomRegistryMu.RLock()
	defer atomRegistryMu.RUnlock()
	var atoms []Atom
	for _, a := range atomRegistry {
		if a.Category() == category {
			atoms = append(atoms, a)
		}
	}
	sort.Slice(atoms, func(i, j int) bool {
		return atoms[i].Name() < atoms[j].Name()
	})
	return atoms
}

// AtomIndex returns a human-readable summary of all registered atoms,
// grouped by category. Useful for diagnostic output and CLI help.
func AtomIndex() string {
	atoms := ListAtoms()
	if len(atoms) == 0 {
		return "No atoms registered."
	}
	cats := make(map[string][]Atom)
	for _, a := range atoms {
		cats[a.Category()] = append(cats[a.Category()], a)
	}
	categoryOrder := []string{AtomCategorySearch, AtomCategoryExtract, AtomCategoryCompare, AtomCategoryReason, AtomCategoryGate}
	var result string
	for _, cat := range categoryOrder {
		items, ok := cats[cat]
		if !ok {
			continue
		}
		result += fmt.Sprintf("[%s]\n", cat)
		for _, a := range items {
			result += fmt.Sprintf("  %-18s — %s\n", a.Name(), a.Description())
		}
		result += "\n"
	}
	return result
}
