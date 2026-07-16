// Package graph provides an in-memory knowledge graph engine for the Mady
// agent framework. It stores entities (statutes, cases, IPC classes, technical
// features) and their relationships as an adjacency list, with JSON
// persistence and zero external dependencies.
//
// The graph is designed to support multi-hop patent/legal reasoning: a
// ReasoningWalker (domains/reasoning) traverses the graph to collect evidence
// chains, while the RuleEngine and Checker consult it for cross-references.
//
// Architecture:
//
//	Document → Builder → GraphStore (adjacency list) → Query/Cache
//	                     ↑ JSON persist/restore
//	                     ↑ ReasoningStoreAdapter → graph.KnowledgeGraphStore (L1)
package graph

// NodeType constants identify the kind of entity a graph node represents.
const (
	NodeConcept       = "Concept"
	NodeLawArticle    = "LawArticle"
	NodeGuidelineRule = "GuidelineRule"
	NodeCase          = "Case"
	NodeJudgment      = "Judgment"
	NodeWikiCard      = "WikiCard"
	NodePersonalNote  = "PersonalNote"
	NodeBookReference = "BookReference"
	NodeRule          = "Rule"
	NodeDomainGuide   = "DomainGuide"
	NodeIPC           = "IPC"
)

// RelationType constants describe directed edges between nodes.
const (
	RelRelatedTo     = "RELATED_TO"
	RelCites         = "CITES"
	RelApplies       = "APPLIES"
	RelSimilarTo     = "SIMILAR_TO"
	RelInterpretedBy = "INTERPRETED_BY"
	RelDefines       = "DEFINES"
	RelHasPrecedent  = "HAS_PRECEDENT"
	RelCitedBy       = "CITED_BY"
	RelImplementedBy = "IMPLEMENTED_BY"
	RelContains      = "CONTAINS"
)

// GraphNode is a single entity in the knowledge graph.
type GraphNode struct {
	ID               string            `json:"id"`
	NodeType         string            `json:"node_type"`
	Name             string            `json:"name"`
	Title            string            `json:"title,omitempty"`
	Content          string            `json:"content,omitempty"`
	Domain           string            `json:"domain,omitempty"`
	Source           string            `json:"source,omitempty"`
	FullRef          string            `json:"full_ref,omitempty"`
	Chapter          string            `json:"chapter,omitempty"`
	ArticleNumber    string            `json:"article_number,omitempty"`
	LawRefs          []string          `json:"law_refs,omitempty"`
	Priority         int               `json:"priority,omitempty"`
	AuthorityWeight  float64           `json:"authority_weight"`
	LevelInHierarchy int               `json:"level_in_hierarchy,omitempty"`
	IPC              string            `json:"ipc,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// GraphEdge is a directed, weighted relationship from SourceID to TargetID.
type GraphEdge struct {
	SourceID string  `json:"source_id"`
	TargetID string  `json:"target_id"`
	Relation string  `json:"relation"`
	Weight   float64 `json:"weight"`
	Evidence string  `json:"evidence,omitempty"`
}

// GraphNodeDetail bundles a node with its outgoing and incoming edges.
type GraphNodeDetail struct {
	Node     *GraphNode  `json:"node"`
	Outgoing []GraphEdge `json:"outgoing"`
	Incoming []GraphEdge `json:"incoming"`
}

// ParsedDoc is the intermediate representation extracted from a
// knowledge.Document before it is inserted into the graph. It carries
// structured metadata that the builder uses to create nodes and edges.
type ParsedDoc struct {
	ID       string         `json:"id"`
	Source   string         `json:"source"`
	DocType  string         `json:"doc_type"`
	Domain   string         `json:"domain"`
	Title    string         `json:"title"`
	Content  string         `json:"content"`
	Metadata ParsedMetadata `json:"metadata"`
}

// ParsedMetadata holds domain-specific fields extracted from a document's
// metadata map. All fields are optional; the builder uses what is present.
type ParsedMetadata struct {
	Title         string   `json:"title,omitempty"`
	Level         string   `json:"level,omitempty"` // e.g. 法律/行政法规/司法解释
	Module        string   `json:"module,omitempty"`
	Priority      string   `json:"priority,omitempty"`   // e.g. "P1".."P5"
	LawRefs       []string `json:"law_refs,omitempty"`   // cited law article IDs
	CrossRefs     []string `json:"cross_refs,omitempty"` // wiki cross-references
	IPCCodes      []string `json:"ipc_codes,omitempty"`
	CaseNumber    string   `json:"case_number,omitempty"`
	Court         string   `json:"court,omitempty"`
	DecisionNum   string   `json:"decision_number,omitempty"`
	ArticleNumber string   `json:"article_number,omitempty"`
	PublishDate   string   `json:"publish_date,omitempty"`
}

// GraphBuildResult reports how many nodes and edges a build produced.
type GraphBuildResult struct {
	NodeCount int `json:"node_count"`
	EdgeCount int `json:"edge_count"`
}

// docTypeToNode maps document type strings (from metadata "type") to
// canonical node types.
var docTypeToNode = map[string]string{
	"concept":        NodeConcept,
	"law_article":    NodeLawArticle,
	"guideline_rule": NodeGuidelineRule,
	"case":           NodeCase,
	"judgment":       NodeJudgment,
	"card":           NodeWikiCard,
	"personal_note":  NodePersonalNote,
	"book_reference": NodeBookReference,
	"agent_config":   NodeConcept,
	"rule":           NodeRule,
	"domain_guide":   NodeDomainGuide,
	"ipc":            NodeIPC,
}

// authorityWeights maps a legal authority level to a 0–1 weight.
var authorityWeights = map[string]float64{
	"法律":    1.0,
	"行政法规":  0.9,
	"司法解释":  0.85,
	"部门规章":  0.8,
	"审查指南":  0.8,
	"指导性案例": 0.75,
	"一般案例":  0.7,
	"学术观点":  0.5,
	"个人笔记":  0.4,
}

// levelMap maps a legal authority level to a hierarchy depth (lower = higher
// authority).
var levelMap = map[string]int{
	"法律":    0,
	"行政法规":  1,
	"司法解释":  2,
	"部门规章":  3,
	"审查指南":  3,
	"指导性案例": 4,
	"一般案例":  5,
	"学术观点":  6,
	"个人笔记":  7,
}

// mapDocType resolves a document type string to a canonical node type,
// falling back to Concept when unknown.
func mapDocType(docType string) string {
	if nt, ok := docTypeToNode[docType]; ok {
		return nt
	}
	return NodeConcept
}

// authorityFromLevel returns the authority weight for a legal level string.
// Unknown levels default to 0.6.
func authorityFromLevel(level string) float64 {
	if w, ok := authorityWeights[level]; ok {
		return w
	}
	return 0.6
}

// hierarchyFromLevel returns the hierarchy depth for a legal level string.
// Unknown levels default to 3.
func hierarchyFromLevel(level string) int {
	if h, ok := levelMap[level]; ok {
		return h
	}
	return 3
}

// mapPriority parses a priority string like "P1".."P5" into an int.
// Empty or malformed strings default to 3.
func mapPriority(priority string) int {
	if priority == "" {
		return 3
	}
	if len(priority) >= 2 && priority[0] == 'P' {
		d := int(priority[1] - '0')
		if d >= 0 && d <= 9 {
			return d
		}
	}
	return 3
}
