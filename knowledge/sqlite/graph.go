package sqlite

import (
	"log/slog"

	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver

	"github.com/xujian519/mady/knowledge/graph"
)

// OpenPatentKGdb opens patent_kg.db for supplementary graph queries.
func (s *SQLiteStore) OpenPatentKGdb(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", path)
	kgDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open patent_kg.db: %w", err)
	}
	kgDB.SetMaxOpenConns(2)
	// 关闭旧连接（防止重复调用 OpenPatentKGdb 泄漏句柄）。
	if s.kgDB != nil {
		if cerr := s.kgDB.Close(); cerr != nil {
			slog.Warn("knowledge/sqlite: close previous patent_kg.db", "err", cerr)
		}
	}
	s.kgDB = kgDB
	return nil
}

// LoadGraph loads all nodes and edges from kg_nodes/kg_edges into a new
// GraphStore. The SQLite schema mirrors Mady's GraphNode/GraphEdge types
// exactly, so mapping is a direct field-to-column translation.
func (s *SQLiteStore) LoadGraph() (*graph.GraphStore, error) {
	gs := graph.NewGraphStore()

	// Load nodes.
	nodeRows, err := s.db.QueryContext(s.baseCtx, `
		SELECT id, node_type, name, title, content, domain, source,
		       full_ref, chapter, article_number, law_refs,
		       priority, authority_weight, level_in_hierarchy
		FROM kg_nodes`)
	if err != nil {
		return nil, fmt.Errorf("load graph nodes: %w", err)
	}
	defer func() { _ = nodeRows.Close() }()

	nodeCount := 0
	for nodeRows.Next() {
		var n graph.GraphNode
		var title, content, source, fullRef, chapter, articleNumber, lawRefs sql.NullString
		var priority, levelInHierarchy sql.NullInt64
		var authorityWeight sql.NullFloat64

		if err := nodeRows.Scan(
			&n.ID, &n.NodeType, &n.Name, &title, &content, &n.Domain,
			&source, &fullRef, &chapter, &articleNumber, &lawRefs,
			&priority, &authorityWeight, &levelInHierarchy,
		); err != nil {
			return nil, fmt.Errorf("scan graph node: %w", err)
		}

		n.Title = title.String
		n.Content = content.String
		n.Source = source.String
		n.FullRef = fullRef.String
		n.Chapter = chapter.String
		n.ArticleNumber = articleNumber.String
		if lawRefs.String != "" {
			n.LawRefs = strings.Split(lawRefs.String, ";")
		}
		n.Priority = int(priority.Int64)
		n.AuthorityWeight = authorityWeight.Float64
		n.LevelInHierarchy = int(levelInHierarchy.Int64)

		gs.AddNode(&n)
		nodeCount++
	}
	if err := nodeRows.Err(); err != nil {
		return nil, err
	}

	// Load edges.
	edgeRows, err := s.db.QueryContext(s.baseCtx, `
		SELECT source_id, target_id, relation, weight, evidence
		FROM kg_edges`)
	if err != nil {
		return nil, fmt.Errorf("load graph edges: %w", err)
	}
	defer func() { _ = edgeRows.Close() }()

	edgeCount := 0
	for edgeRows.Next() {
		var e graph.GraphEdge
		var weight sql.NullFloat64
		var evidence sql.NullString
		if err := edgeRows.Scan(&e.SourceID, &e.TargetID, &e.Relation, &weight, &evidence); err != nil {
			return nil, fmt.Errorf("scan graph edge: %w", err)
		}
		e.Weight = weight.Float64
		e.Evidence = evidence.String
		if gs.HasNode(e.SourceID) && gs.HasNode(e.TargetID) {
			gs.AddEdge(e)
			edgeCount++
		}
	}

	return gs, edgeRows.Err()
}
