package worker

import "fmt"

// Catalog is a registry of Worker definitions.
type Catalog struct {
	entries []Definition
	byName  map[string]*Definition
}

// NewCatalog creates an empty Catalog.
func NewCatalog() *Catalog {
	return &Catalog{
		byName: make(map[string]*Definition),
	}
}

// Register adds a Worker definition. Replaces any existing entry with the same name.
func (c *Catalog) Register(d Definition) error {
	if d.Name == "" {
		return fmt.Errorf("worker: definition name is required")
	}
	c.byName[d.Name] = &d
	for i, e := range c.entries {
		if e.Name == d.Name {
			c.entries[i] = d
			return nil
		}
	}
	c.entries = append(c.entries, d)
	return nil
}

// Get returns a Worker definition by name, or nil.
func (c *Catalog) Get(name string) *Definition {
	return c.byName[name]
}

// List returns all registered Worker definitions.
func (c *Catalog) List() []Definition {
	out := make([]Definition, len(c.entries))
	copy(out, c.entries)
	return out
}

// ListByTier returns Workers matching the given tier.
func (c *Catalog) ListByTier(tier WorkerTier) []Definition {
	var out []Definition
	for _, d := range c.entries {
		if d.Tier == tier {
			out = append(out, d)
		}
	}
	return out
}

// Verify checks that all pre-registered Workers in the catalog exist
// (have a valid Definition with non-empty Name). Returns a list of issues.
func (c *Catalog) Verify() []string {
	var issues []string
	for _, d := range c.entries {
		if d.Name == "" {
			issues = append(issues, "Worker with empty name found")
			continue
		}
		if d.Description == "" {
			issues = append(issues, fmt.Sprintf("Worker %q has no description", d.Name))
		}
		if len(d.Outputs) == 0 {
			issues = append(issues, fmt.Sprintf("Worker %q has no output contracts", d.Name))
		}
	}
	return issues
}

// DefaultWorkers returns the built-in patent domain Workers.
func DefaultWorkers() []Definition {
	return []Definition{
		// ===== Work Tier =====
		{
			Name:         "patent-technical-analyzer",
			Tier:         TierWork,
			Description:  "分析技术交底书，提取技术三要素（问题/特征/效果），生成结构化分析报告。对应 disclosure/ 管线。",
			Inputs:       []Input{{Path: "data/cases/{caseId}/disclosure/*.{md,txt,pdf}", Optional: false}},
			Outputs:      []Output{{Path: "data/cases/{caseId}/outputs/technical-analysis.md", Format: "markdown", ContractLevel: ContractHard}},
			AllowedTools: []string{"read", "grep", "bash"},
			TriggersHITL: false,
		},
		{
			Name:         "patent-claim-drafter",
			Tier:         TierWork,
			Description:  "基于技术方案撰写权利要求书。支持机械/电学/化学/软件四领域，生成独立+从属权利要求。对应 domains/claimdrafting/。",
			Inputs:       []Input{{Path: "data/cases/{caseId}/outputs/technical-analysis.md", Optional: false}},
			Outputs:      []Output{{Path: "data/cases/{caseId}/outputs/claims.md", Format: "markdown", ContractLevel: ContractHard}},
			AllowedTools: []string{"read", "patent_eval"},
			TriggersHITL: true,
		},
		{
			Name:         "patent-spec-drafter",
			Tier:         TierWork,
			Description:  "撰写专利说明书（五部分完整结构）。支持四大领域自适应。对应 domains/specdrafting/。",
			Inputs:       []Input{{Path: "data/cases/{caseId}/outputs/technical-analysis.md"}},
			Outputs:      []Output{{Path: "data/cases/{caseId}/outputs/specification.md", Format: "markdown", ContractLevel: ContractHard}},
			AllowedTools: []string{"read"},
			TriggersHITL: true,
		},
		{
			Name:         "patent-oa-response-drafter",
			Tier:         TierWork,
			Description:  "起草审查意见（OA）答复，含争辩论点和修改方案。对应 domains/workflows/patent/oa_response.go。",
			Inputs:       []Input{{Path: "data/cases/{caseId}/office-actions/*.{pdf,txt}"}},
			Outputs:      []Output{{Path: "data/cases/{caseId}/outputs/oa-response.md", Format: "markdown", ContractLevel: ContractHard}},
			AllowedTools: []string{"read", "grep"},
			TriggersHITL: true,
		},
		{
			Name:         "patent-slop-cleaner",
			Tier:         TierWork,
			Description:  "清洗专利文档中的 AI 套话（填充词/模糊语/过激断言等），提高表达质量。对应 domains/rules/slop_engine.go。",
			Inputs:       []Input{{Path: "data/cases/{caseId}/outputs/*.md"}},
			Outputs:      []Output{{Path: "data/cases/{caseId}/outputs/*-cleaned.md", Format: "markdown", ContractLevel: ContractSoft}},
			AllowedTools: []string{"read", "patent_eval"},
		},
		// ===== Reasoning Tier =====
		{
			Name:         "patent-novelty-analyzer",
			Tier:         TierReasoning,
			Description:  "逐特征比对权利要求与对比文件，判断新颖性（A22.2）。对应 domains/novelty/。",
			Inputs:       []Input{{Path: "data/cases/{caseId}/outputs/claims.md"}},
			Outputs:      []Output{{Path: "data/cases/{caseId}/outputs/novelty-analysis.md", Format: "markdown", ContractLevel: ContractHard}},
			AllowedTools: []string{"read", "patent_search"},
		},
		{
			Name:         "patent-inventiveness-analyzer",
			Tier:         TierReasoning,
			Description:  "三步法判断创造性（A22.3）。对应 domains/inventiveness/。",
			Inputs:       []Input{{Path: "data/cases/{caseId}/outputs/claims.md"}},
			Outputs:      []Output{{Path: "data/cases/{caseId}/outputs/inventiveness-analysis.md", Format: "markdown", ContractLevel: ContractHard}},
			AllowedTools: []string{"read", "patent_search"},
		},
		// ===== Domain Tier =====
		{
			Name:         "patent-search-planner",
			Tier:         TierDomain,
			Description:  "根据检索需求制定多轮渐进式检索策略。类似 Search Commander 功能。",
			Inputs:       []Input{{Path: "data/cases/{caseId}/search-request.md"}},
			Outputs:      []Output{{Path: "data/cases/{caseId}/outputs/search-plan.md", Format: "markdown", ContractLevel: ContractStructured}},
			AllowedTools: []string{"read"},
		},
		{
			Name:         "patent-search-executor",
			Tier:         TierDomain,
			Description:  "执行专利检索（Google Patents/CNIPA），收集对比文件和相关信息。",
			Inputs:       []Input{{Path: "data/cases/{caseId}/outputs/search-plan.md"}},
			Outputs:      []Output{{Path: "data/cases/{caseId}/outputs/search-results.md", Format: "markdown", ContractLevel: ContractHard}},
			AllowedTools: []string{"web_search", "web_fetch", "bash"},
		},
		// ===== Checker Tier =====
		{
			Name:         "reviewer",
			Tier:         TierChecker,
			Description:  "文档形式规范审查：权利要求格式、说明书完整性、附图一致性、引用正确性。",
			Inputs:       []Input{{Path: "data/cases/{caseId}/outputs/*.md"}},
			Outputs:      []Output{{Path: "data/cases/{caseId}/outputs/review-report.md", Format: "markdown", ContractLevel: ContractSoft}},
			AllowedTools: []string{"read", "grep"},
		},
		{
			Name:         "quality_checker",
			Tier:         TierChecker,
			Description:  "三维度质量评分：清晰性(35%)、支持性(30%)、保护范围(35%)。需要 patent_eval 预检 ≥0.7。",
			Inputs:       []Input{{Path: "data/cases/{caseId}/outputs/*.md"}},
			Outputs:      []Output{{Path: "data/cases/{caseId}/outputs/quality-report.md"}},
			AllowedTools: []string{"read", "patent_eval"},
			TriggersHITL: true,
		},
	}
}
