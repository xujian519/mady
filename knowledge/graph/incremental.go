package graph

// IncrementalGraphUpdate applies a set of additions and removals to an
// existing graph store without rebuilding from scratch. This is the
// convenience entry point used after incremental document ingestion.
//
// Removed documents have their nodes (and all connected edges) deleted from
// the store. Added documents are parsed into nodes and edges exactly as in a
// full Build. The law-reference and wiki-name indices are rebuilt only for
// the added batch, keeping the incremental cost proportional to the change
// set rather than the full corpus.
func IncrementalGraphUpdate(store *GraphStore, added []ParsedDoc, removedIDs []string) GraphBuildResult {
	b := NewGraphBuilder(store)
	return b.IncrementalUpdate(added, removedIDs)
}
