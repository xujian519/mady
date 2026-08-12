// Package patent provides a Pregel-based patent invalidation analysis workflow.
//
// The invalidation workflow analyzes a target patent against prior-art grounds
// under Chinese patent law (A22.2 novelty / A22.3 inventiveness / A26.3 disclosure
// sufficiency / A26.4 claim clarity & support / A33 amendment scope).
//
// Graph structure (without retriever):
//
//	parse_patent → identify_grounds → analyze_grounds → conclude → __end__
//
// Graph structure (with retriever):
//
//	parse_patent → identify_grounds → gather_evidence → analyze_grounds → conclude → __end__
//
// Each node is deterministic (no LLM calls) — the graph produces a structured
// invalidation analysis skeleton that a patent attorney reviews and finalizes.
// The deterministic rule engine validates the analysis for legal completeness.
package patent

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/retrieval/domain"
)

// State keys used by the invalidation workflow.
const (
	InvStateInput       = "inv_input"        // original input (claims + requester grounds)
	InvStateClaims      = "inv_claims"       // parsed patent claims text
	InvStateClaimTree   = "inv_claim_tree"   // []InvClaimNode: parsed claim structure
	InvStateGrounds     = "inv_grounds"      // []InvGround: identified invalidation grounds
	InvStateEvidence    = "inv_evidence"     // []string: retrieved evidence references
	InvStateAnalysis    = "inv_analysis"     // per-ground analysis text
	InvStateRuleCheck   = "inv_rule_check"   // rule engine check report
	InvStateRuleVerdict = "inv_rule_verdict" // aggregate verdict
	InvStateConclusion  = "inv_conclusion"   // final conclusion
	InvStateOutput      = "inv_output"       // final output text

	// New evidence integration state keys.
	InvStateEvidenceJudgments = "inv_evidence_judgments" // []evidence.EvidenceJudgment
	InvStateValidEvidence     = "inv_valid_evidence"     // []agentcore_evidence.EvidenceSpan
	InvStateConflicts         = "inv_conflicts"          // []agentcore_evidence.Conflict
)

// InvalidationGroundType identifies the legal basis for invalidation.
type InvalidationGroundType string

const (
	// GroundNovelty corresponds to Article 22.2 (lack of novelty).
	GroundNovelty InvalidationGroundType = "A22.2_novelty"
	// GroundInventiveness corresponds to Article 22.3 (lack of inventive step).
	GroundInventiveness InvalidationGroundType = "A22.3_inventiveness"
	// GroundDisclosure corresponds to Article 26.3 (insufficient disclosure).
	GroundDisclosure InvalidationGroundType = "A26.3_disclosure"
	// GroundClaimClarity corresponds to Article 26.4 (unclear claims).
	GroundClaimClarity InvalidationGroundType = "A26.4_clarity"
	// GroundAmendment corresponds to Article 33 (amendment beyond original disclosure).
	GroundAmendment InvalidationGroundType = "A33_amendment"
)

// InvClaimNode represents a single parsed claim from the target patent.
type InvClaimNode struct {
	Number        int    // claim number (1, 2, 3...)
	IsIndependent bool   // true if independent claim
	Type          string // "method", "apparatus", "system", "compound", etc.
	Text          string // raw claim text
}

// InvGround represents one identified invalidation ground.
type InvGround struct {
	Type        InvalidationGroundType
	Article     string // legal article reference
	Description string // human-readable description
	ClaimRefs   []int  // affected claim numbers
}

// =============================================================================
// Pregel Nodes
// =============================================================================

// =============================================================================
// Graph Builder
// =============================================================================

// InvGraphOption optionally configures the invalidation graph's dependencies.
type InvGraphOption func(*invGraphConfig)

type invGraphConfig struct {
	retriever domain.DomainRetriever
}

// WithInvRetriever injects a domain retriever, enabling real evidence retrieval.
// When not injected, evidence gathering returns degraded results.
func WithInvRetriever(r domain.DomainRetriever) InvGraphOption {
	return func(c *invGraphConfig) { c.retriever = r }
}

// BuildInvalidationGraph constructs a Pregel graph for patent invalidation analysis
// (no retriever injected, evidence gathering returns degraded results).
//
// Graph structure:
//
//	parse_patent → identify_grounds → analyze_grounds → conclude → __end__
func BuildInvalidationGraph() (*graph.CompiledPregelGraph, error) {
	return BuildInvalidationGraphWithOpts()
}

// BuildInvalidationGraphWithOpts constructs the invalidation analysis Pregel graph
// with optional dependency injection.
//
// Without retriever:
//
//	parse_patent → identify_grounds → analyze_grounds → conclude → __end__
//
// With retriever (evidence gathering node inserted):
//
//	parse_patent → identify_grounds → gather_evidence → analyze_grounds → conclude → __end__
func BuildInvalidationGraphWithOpts(opts ...InvGraphOption) (*graph.CompiledPregelGraph, error) {
	cfg := &invGraphConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	g := graph.NewPregelGraph()

	if err := g.AddNode("parse_patent", parsePatentNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("identify_grounds", identifyGroundsNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("analyze_grounds", analyzeGroundsNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("conclude", invConcludeNode); err != nil {
		return nil, err
	}

	// Conditionally insert evidence gathering node and evidence integration nodes.
	hasRetriever := cfg.retriever != nil
	if hasRetriever {
		if err := g.AddNode("gather_evidence", newGatherEvidenceNodeWithRetriever(cfg.retriever)); err != nil {
			return nil, err
		}
		if err := g.AddNode("judge_evidence", judgeEvidenceNode); err != nil {
			return nil, err
		}
		if err := g.AddNode("filter_evidence", filterEvidenceNode); err != nil {
			return nil, err
		}
		if err := g.AddNode("detect_conflict", detectConflictNode); err != nil {
			return nil, err
		}
	}

	// Build edges.
	edges := [][2]string{
		{"parse_patent", "identify_grounds"},
	}
	if hasRetriever {
		edges = append(edges, [][2]string{
			{"identify_grounds", "gather_evidence"},
			{"gather_evidence", "judge_evidence"},
			{"judge_evidence", "filter_evidence"},
			{"filter_evidence", "detect_conflict"},
			{"detect_conflict", "analyze_grounds"},
		}...)
	} else {
		edges = append(edges, [2]string{"identify_grounds", "analyze_grounds"})
	}
	edges = append(edges, [][2]string{
		{"analyze_grounds", "conclude"},
		{"conclude", graph.PregelEnd},
	}...)

	for _, edge := range edges {
		if err := g.AddEdge(edge[0], edge[1]); err != nil {
			return nil, err
		}
	}

	return g.Compile("parse_patent", 15)
}

// yamlDiscoveryStage is a helper struct for parsing the discoveryStages section
// from an orchestration YAML file within BuildInvalidationGraphFromYAML.
type yamlDiscoveryStage struct {
	Name        string   `yaml:"name"`
	Goal        string   `yaml:"goal"`
	Suggestions []string `yaml:"suggestions"`
}

// yamlOrchestration is a helper struct for parsing orchestration YAML files
// within BuildInvalidationGraphFromYAML.
type yamlOrchestration struct {
	DiscoveryStages []yamlDiscoveryStage `yaml:"discoveryStages"`
}

// BuildInvalidationGraphFromYAML constructs the invalidation analysis Pregel graph
// with suggestions injected from a YAML orchestration file.
func BuildInvalidationGraphFromYAML(yamlPath string) (*graph.CompiledPregelGraph, error) {
	data, err := os.ReadFile(yamlPath) //nolint:gosec // yamlPath is from caller (typically filepath.Join to rules/orchestrations dir)
	if err != nil {
		return nil, fmt.Errorf("read orchestration YAML %s: %w", yamlPath, err)
	}
	var orch yamlOrchestration
	if err := yaml.Unmarshal(data, &orch); err != nil {
		return nil, fmt.Errorf("unmarshal orchestration: %w", err)
	}
	var suggestions strings.Builder
	suggestions.WriteString("\n### 编排建议\n\n")
	suggestions.WriteString("以下建议来自无效宣告事务编排模板：\n\n")
	for _, stage := range orch.DiscoveryStages {
		fmt.Fprintf(&suggestions, "**阶段：%s**（目标：%s）\n", stage.Name, stage.Goal)
		for _, s := range stage.Suggestions {
			fmt.Fprintf(&suggestions, "- %s\n", s)
		}
		suggestions.WriteString("\n")
	}
	suggestionText := suggestions.String()
	originalAnalyze := analyzeGroundsNode
	enhancedAnalyze := func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		out, err := originalAnalyze(ctx, state)
		if err != nil {
			return nil, err
		}
		existing := out.GetString(InvStateAnalysis)
		if existing != "" {
			out[InvStateAnalysis] = suggestionText + "\n" + existing
		}
		return out, nil
	}
	g := graph.NewPregelGraph()
	_ = g.AddNode("parse_patent", parsePatentNode)
	_ = g.AddNode("identify_grounds", identifyGroundsNode)
	_ = g.AddNode("analyze_grounds", enhancedAnalyze)
	_ = g.AddNode("conclude", invConcludeNode)
	for _, edge := range [][2]string{
		{"parse_patent", "identify_grounds"},
		{"identify_grounds", "analyze_grounds"},
		{"analyze_grounds", "conclude"},
		{"conclude", graph.PregelEnd},
	} {
		_ = g.AddEdge(edge[0], edge[1])
	}
	return g.Compile("parse_patent", 15)
}
