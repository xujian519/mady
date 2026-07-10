package graph

import (
	"sort"
	"strings"

	"github.com/xujian519/mady/knowledge"
)

// GraphBuilder constructs a knowledge graph from parsed documents, creating
// nodes and relationship edges (CITES, APPLIES, SIMILAR_TO, RELATED_TO) based
// on document metadata.
//
// The builder mirrors the proven design from the reference implementation:
//   - LawArticle nodes are auto-created (deduplicated) for every cited statute.
//   - Cases/Judgments get reverse APPLIES edges from their cited laws.
//   - Documents sharing a common law citation get SIMILAR_TO edges.
//   - Wiki cross-references become RELATED_TO edges.
type GraphBuilder struct {
	store           *GraphStore
	ensuredLawNodes map[string]bool     // dedup set for auto-created LawArticle nodes
	lawRefIndex     map[string][]string // lawRef → []docID (for SIMILAR_TO detection)
	wikiNameIndex   map[string]string   // wiki filename → docID

	nodeCount int
	edgeCount int
}

// NewGraphBuilder creates a builder that writes into the given store.
func NewGraphBuilder(store *GraphStore) *GraphBuilder {
	return &GraphBuilder{
		store:           store,
		ensuredLawNodes: make(map[string]bool),
		lawRefIndex:     make(map[string][]string),
		wikiNameIndex:   make(map[string]string),
	}
}

// Build constructs the full graph from the given documents, replacing any
// existing content via the builder's store. It runs in two phases: first all
// nodes are created (so cross-references resolve), then all edges. This
// ordering ensures SIMILAR_TO edges between sibling documents are not dropped
// because a target node has not been added yet.
func (b *GraphBuilder) Build(docs []ParsedDoc) GraphBuildResult {
	b.buildIndices(docs)
	for _, doc := range docs {
		b.createNode(doc)
	}
	for _, doc := range docs {
		b.createEdges(doc)
	}
	return GraphBuildResult{NodeCount: b.nodeCount, EdgeCount: b.edgeCount}
}

// IncrementalUpdate adds new documents and removes stale ones without
// rebuilding the entire graph. Removed documents have their nodes and edges
// deleted; added documents are processed as in Build (two-phase).
func (b *GraphBuilder) IncrementalUpdate(added []ParsedDoc, removedIDs []string) GraphBuildResult {
	for _, id := range removedIDs {
		b.store.RemoveNode(id)
	}
	b.buildIndices(added)
	for _, doc := range added {
		b.createNode(doc)
	}
	for _, doc := range added {
		b.createEdges(doc)
	}
	return GraphBuildResult{NodeCount: b.nodeCount, EdgeCount: b.edgeCount}
}

// buildIndices pre-computes the law-reference and wiki-name lookup tables used
// during edge creation.
func (b *GraphBuilder) buildIndices(docs []ParsedDoc) {
	for _, doc := range docs {
		for _, ref := range doc.Metadata.LawRefs {
			b.lawRefIndex[ref] = append(b.lawRefIndex[ref], doc.ID)
		}
		if doc.Source == "wiki" {
			fileName := doc.ID
			if idx := strings.LastIndex(doc.ID, ":"); idx >= 0 {
				fileName = doc.ID[idx+1:]
			}
			if _, exists := b.wikiNameIndex[fileName]; !exists {
				b.wikiNameIndex[fileName] = doc.ID
			}
		}
	}
}

// createNode adds the document's node to the store (build phase 1). Edges are
// created separately in createEdges so that cross-document references (e.g.
// SIMILAR_TO) resolve against already-present target nodes.
func (b *GraphBuilder) createNode(doc ParsedDoc) {
	nodeType := mapDocType(doc.DocType)
	authorityWeight := authorityFromLevel(doc.Metadata.Level)
	levelInHierarchy := hierarchyFromLevel(doc.Metadata.Level)

	// Truncate content for the graph node (full content stays in the Store).
	contentPreview := doc.Content
	if len([]rune(contentPreview)) > 200 {
		contentPreview = string([]rune(contentPreview)[:200])
	}

	// Law refs stored as JSON-ish list (capped at 10).
	lawRefsCapped := capSlice(doc.Metadata.LawRefs, 10)

	node := &GraphNode{
		ID:               doc.ID,
		NodeType:         nodeType,
		Name:             doc.Title,
		Title:            doc.Metadata.Title,
		Content:          contentPreview,
		Domain:           doc.Domain,
		Source:           doc.Source,
		ArticleNumber:    doc.Metadata.ArticleNumber,
		LawRefs:          lawRefsCapped,
		Priority:         mapPriority(doc.Metadata.Priority),
		AuthorityWeight:  authorityWeight,
		LevelInHierarchy: levelInHierarchy,
		IPC:              firstOr(doc.Metadata.IPCCodes, ""),
		Metadata: map[string]string{
			"case_number":     doc.Metadata.CaseNumber,
			"court":           doc.Metadata.Court,
			"decision_number": doc.Metadata.DecisionNum,
			"publish_date":    doc.Metadata.PublishDate,
			"module":          doc.Metadata.Module,
		},
	}
	if node.Title == "" {
		node.Title = doc.Title
	}
	b.store.AddNode(node)
	b.nodeCount++
}

// createEdges builds all relationship edges for a document (build phase 2).
func (b *GraphBuilder) createEdges(doc ParsedDoc) {
	nodeType := mapDocType(doc.DocType)

	// Cross-references → RELATED_TO edges.
	for _, ref := range capSlice(doc.Metadata.CrossRefs, 20) {
		targetID := b.resolveWikiRef(ref, doc.Source)
		b.store.AddEdge(GraphEdge{
			SourceID: doc.ID,
			TargetID: targetID,
			Relation: RelRelatedTo,
			Weight:   0.8,
			Evidence: "交叉引用: [[" + ref + "]]",
		})
		b.edgeCount++
	}

	// Law citations → CITES edges + auto-create LawArticle nodes.
	for _, lawRef := range capSlice(doc.Metadata.LawRefs, 15) {
		lawNodeID := lawNodeID(lawRef)
		if lawNodeID == "" {
			continue
		}
		b.ensureLawNode(lawRef, lawNodeID)
		b.store.AddEdge(GraphEdge{
			SourceID: doc.ID,
			TargetID: lawNodeID,
			Relation: RelCites,
			Weight:   0.9,
			Evidence: "引用: " + lawRef,
		})
		b.edgeCount++
	}

	// Cases and Judgments: reverse APPLIES edges + SIMILAR_TO detection.
	if nodeType == NodeCase || nodeType == NodeJudgment {
		for _, lawRef := range capSlice(doc.Metadata.LawRefs, 5) {
			lawNodeID := lawNodeID(lawRef)
			if lawNodeID == "" {
				continue
			}
			b.ensureLawNode(lawRef, lawNodeID)
			b.store.AddEdge(GraphEdge{
				SourceID: lawNodeID,
				TargetID: doc.ID,
				Relation: RelApplies,
				Weight:   0.85,
				Evidence: "适用: " + lawRef,
			})
			b.edgeCount++

			// Find a similar document sharing this law citation (first match
			// with a higher ID to avoid duplicate bidirectional edges).
			if similarDocs, ok := b.lawRefIndex[lawRef]; ok {
				for _, similarID := range similarDocs {
					if similarID != doc.ID && similarID > doc.ID {
						b.store.AddEdge(GraphEdge{
							SourceID: doc.ID,
							TargetID: similarID,
							Relation: RelSimilarTo,
							Weight:   0.6,
							Evidence: "共同引用: " + lawRef,
						})
						b.edgeCount++
						break
					}
				}
			}
		}
	}
}

// ensureLawNode creates a LawArticle node for a law reference if not already
// present (deduplication across documents).
func (b *GraphBuilder) ensureLawNode(lawRef, lawNodeID string) {
	if b.ensuredLawNodes[lawNodeID] {
		return
	}
	b.ensuredLawNodes[lawNodeID] = true
	b.store.AddNode(&GraphNode{
		ID:              lawNodeID,
		NodeType:        NodeLawArticle,
		Name:            lawRef,
		Title:           lawRef,
		Domain:          "patent",
		Source:          "extracted",
		Priority:        1,
		AuthorityWeight: 1.0,
	})
	b.nodeCount++
}

// resolveWikiRef maps a wiki cross-reference to a node ID, preferring an
// already-indexed wiki document.
func (b *GraphBuilder) resolveWikiRef(ref, docSource string) string {
	if id, ok := b.wikiNameIndex[ref]; ok {
		return id
	}
	if docSource == "wiki" {
		return "wiki:" + strings.ReplaceAll(ref, "/", ":")
	}
	return ref
}

// lawNodeID converts a law reference string (e.g. "专利法第22条第3款") into a
// stable, deduplicated node ID. Returns "" if the reference is empty.
func lawNodeID(lawRef string) string {
	lawRef = strings.TrimSpace(lawRef)
	if lawRef == "" {
		return ""
	}
	return "law:" + strings.NewReplacer(" ", "", "\t", "").Replace(lawRef)
}

// --- Document parsing ---

// ParseKnowledgeDocument extracts a ParsedDoc from a knowledge.Document by
// reading its metadata map. This bridges the document store and the graph
// builder.
func ParseKnowledgeDocument(doc *knowledge.Document) ParsedDoc {
	m := doc.Metadata
	parsed := ParsedDoc{
		ID:      doc.ID,
		Source:  doc.Source,
		DocType: metadataStr(m, "type"),
		Domain:  doc.Domain,
		Title:   doc.Title,
		Content: doc.Content,
		Metadata: ParsedMetadata{
			Title:         metadataStr(m, "title"),
			Level:         metadataStr(m, "level"),
			Module:        metadataStr(m, "module"),
			Priority:      metadataStr(m, "priority"),
			LawRefs:       splitList(m["law_refs"]),
			CrossRefs:     splitList(m["cross_refs"]),
			IPCCodes:      splitList(m["ipc_codes"]),
			CaseNumber:    metadataStr(m, "case_number"),
			Court:         metadataStr(m, "court"),
			DecisionNum:   metadataStr(m, "decision_number"),
			ArticleNumber: metadataStr(m, "article_number"),
			PublishDate:   metadataStr(m, "publish_date"),
		},
	}
	// Fall back to the "ipc" / "law" fields when the structured variants
	// are absent.
	if len(parsed.Metadata.IPCCodes) == 0 {
		if ipc := metadataStr(m, "ipc"); ipc != "" {
			parsed.Metadata.IPCCodes = []string{ipc}
		}
	}
	if len(parsed.Metadata.LawRefs) == 0 {
		if law := metadataStr(m, "law"); law != "" {
			parsed.Metadata.LawRefs = []string{law}
		}
	}
	if parsed.DocType == "" {
		parsed.DocType = inferDocType(doc.Domain, parsed.Metadata.Level)
	}
	return parsed
}

// inferDocType guesses a document type from its domain and authority level
// when no explicit "type" metadata is present.
func inferDocType(domain, level string) string {
	switch level {
	case "法律":
		return "law_article"
	case "审查指南":
		return "guideline_rule"
	case "指导性案例", "一般案例":
		return "case"
	}
	if strings.Contains(strings.ToLower(domain), "case") {
		return "case"
	}
	return "concept"
}

// --- helpers ---

func metadataStr(m map[string]string, key string) string {
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[key])
}

// splitList splits a comma/semicolon-separated metadata value into a slice.
func splitList(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == '、'
	})
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func capSlice(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func firstOr(s []string, def string) string {
	if len(s) == 0 {
		return def
	}
	return s[0]
}

// SortedLawRefs returns the law references index keys in deterministic order
// (useful for testing and debugging).
func (b *GraphBuilder) SortedLawRefs() []string {
	keys := make([]string, 0, len(b.lawRefIndex))
	for k := range b.lawRefIndex {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
