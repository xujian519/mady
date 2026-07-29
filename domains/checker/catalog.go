// Package checker provides a Checker subsystem for patent quality review.
//
// The Checker subsystem manages a catalog of specialized review agents (Checkers)
// that inspect patent-related artifacts and return structured verdicts. It works
// alongside patent_eval auto-evaluation to form a complete quality closed loop:
//
//	auto-eval → checker review → revision → re-eval → sign-off
package checker

// CheckerTier classifies the review capability level.
type CheckerTier string

const (
	TierReviewer       CheckerTier = "reviewer"        // 文档形式规范/格式审查
	TierQualityChecker CheckerTier = "quality_checker" // 三维度加权质量评分
	TierNoveltyChecker CheckerTier = "novelty_checker" // 新颖性分析复核
	TierOAChecker      CheckerTier = "oa_checker"      // OA 答复逻辑复核
)

// CheckerEntry defines one checker's identity and capability.
type CheckerEntry struct {
	RoleID      string      `json:"role_id"`
	Tier        CheckerTier `json:"tier"`
	Name        string      `json:"name"`
	Description string      `json:"description"`

	// InvokesAfter lists which worker/step IDs should complete before this checker runs.
	InvokesAfter []string `json:"invokes_after,omitempty"`

	// HITLCheckpoint marks whether this checker's verdict requires human approval.
	HITLCheckpoint bool `json:"hitl_checkpoint,omitempty"`

	// RequiredInputs describes what artifacts this checker needs to read.
	RequiredInputs []string `json:"required_inputs,omitempty"`

	// OutputContracts describes what the checker produces.
	OutputContracts []string `json:"output_contracts,omitempty"`

	// AllowedTools restricts what tools this checker may call, if any.
	AllowedTools []string `json:"allowed_tools,omitempty"`
}

// Catalog is the registry of all available Checker definitions.
type Catalog struct {
	entries []CheckerEntry
	byID    map[string]*CheckerEntry
}

// NewCatalog creates an empty Catalog.
func NewCatalog() *Catalog {
	return &Catalog{
		byID: make(map[string]*CheckerEntry),
	}
}

// Register adds a checker to the catalog. Replaces any existing entry with the same RoleID.
func (c *Catalog) Register(entry CheckerEntry) {
	for i, e := range c.entries {
		if e.RoleID == entry.RoleID {
			c.entries[i] = entry
			c.byID[entry.RoleID] = &c.entries[i]
			return
		}
	}
	c.entries = append(c.entries, entry)
	c.byID[entry.RoleID] = &c.entries[len(c.entries)-1]
}

// Get returns a checker by RoleID, or nil if not found.
func (c *Catalog) Get(roleID string) *CheckerEntry {
	return c.byID[roleID]
}

// List returns all registered checkers.
func (c *Catalog) List() []CheckerEntry {
	out := make([]CheckerEntry, len(c.entries))
	copy(out, c.entries)
	return out
}

// Suggest returns checkers that are relevant for a given artifact path.
// This uses simple path-prefix matching to be easily extensible.
func (c *Catalog) Suggest(artifactPath string) []CheckerEntry {
	var matches []CheckerEntry
	for _, e := range c.entries {
		for _, input := range e.RequiredInputs {
			if matchArtifact(artifactPath, input) {
				matches = append(matches, e)
				break
			}
		}
	}
	return matches
}

// matchArtifact checks if an artifact path matches a required input pattern.
// Patterns can use "*" as a wildcard for simple glob matching.
func matchArtifact(path, pattern string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	// Simple prefix/suffix matching
	plen := len(pattern)
	if plen > 0 && pattern[0] == '*' {
		return len(path) >= plen-1 && path[len(path)-(plen-1):] == pattern[1:]
	}
	if plen > 0 && pattern[plen-1] == '*' {
		return len(path) >= plen-1 && path[:plen-1] == pattern[:plen-1]
	}
	return path == pattern
}

// DefaultCatalog returns the built-in Checker catalog with standard entries.
func DefaultCatalog() *Catalog {
	c := NewCatalog()
	c.Register(CheckerEntry{
		RoleID:          "reviewer",
		Tier:            TierReviewer,
		Name:            "文件审查专家",
		Description:     "检查专利文档的形式规范、权利要求格式、说明书完整性、附图一致性、引用正确性。",
		InvokesAfter:    []string{"patent-claim-drafter", "patent-spec-drafter"},
		HITLCheckpoint:  false,
		RequiredInputs:  []string{"*.md", "*.txt"},
		OutputContracts: []string{"review-report.md"},
		AllowedTools:    []string{"read", "grep"},
	})
	c.Register(CheckerEntry{
		RoleID:          "quality_checker",
		Tier:            TierQualityChecker,
		Name:            "质量评估专家",
		Description:     "对专利分析报告进行三维度加权评分：清晰性（35%）、支持性（30%）、保护范围（35%）。仅当 patent_eval 综合分 ≥ 0.7 时调用。",
		InvokesAfter:    []string{"patent-technical-analyzer", "patent-novelty-analyzer"},
		HITLCheckpoint:  true,
		RequiredInputs:  []string{"*technical-analysis*", "*novelty-analy*", "*inventiveness*"},
		OutputContracts: []string{"quality-report.md", "quality-verdict.json"},
		AllowedTools:    []string{"read", "patent_eval"},
	})
	c.Register(CheckerEntry{
		RoleID:          "novelty_checker",
		Tier:            TierNoveltyChecker,
		Name:            "新颖性复核专家",
		Description:     "复核新颖性分析的逐特征比对逻辑，确认对比文件公开内容引用是否准确，特征比对是否完整。",
		InvokesAfter:    []string{"patent-novelty-analyzer"},
		HITLCheckpoint:  true,
		RequiredInputs:  []string{"*novelty*", "*novelty-analy*"},
		OutputContracts: []string{"novelty-review.md"},
		AllowedTools:    []string{"read", "grep"},
	})
	c.Register(CheckerEntry{
		RoleID:          "oa_checker",
		Tier:            TierOAChecker,
		Name:            "OA 答复复核专家",
		Description:     "复核 OA 答复的逻辑完整性、修改合规性（A33）、争辩力度、对比文件区分准确性。参照 oa-response-checklist.md 逐项核对。",
		InvokesAfter:    []string{"patent-oa-response-drafter"},
		HITLCheckpoint:  true,
		RequiredInputs:  []string{"*oa-response*", "*OA*"},
		OutputContracts: []string{"oa-review.md"},
		AllowedTools:    []string{"read", "grep"},
	})
	return c
}
