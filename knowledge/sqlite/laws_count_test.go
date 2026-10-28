package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// createTestKnowledgeDB 建一个满足 NewSQLiteStore 最小结构的知识库文件：
// embeddings 表至少一行（用于探测维度），其余表由 Stats 查询失败时按 0 处理。
func createTestKnowledgeDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	statements := []string{
		`CREATE TABLE embeddings (id INTEGER PRIMARY KEY, vector BLOB)`,
		`INSERT INTO embeddings (vector) VALUES (zeroblob(4096))`,
		`CREATE TABLE documents (id TEXT PRIMARY KEY)`,
		`CREATE TABLE chunks (id INTEGER PRIMARY KEY, document_id TEXT)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
}

func TestLawsCountAndAvailability(t *testing.T) {
	dir := t.TempDir()
	knowledgePath := filepath.Join(dir, "knowledge.db")
	createTestKnowledgeDB(t, knowledgePath)
	store, err := NewSQLiteStore(knowledgePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	if store.LawsAvailable() {
		t.Error("未打开法规库时 LawsAvailable 应为 false")
	}
	if count := store.LawsCount(); count != 0 {
		t.Errorf("未打开时 LawsCount = %d", count)
	}

	lawsPath := filepath.Join(dir, "laws-full-local.db")
	lawsDB, err := sql.Open("sqlite", lawsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE law (id TEXT PRIMARY KEY, name TEXT, content TEXT)`,
		`INSERT INTO law (id, name, content) VALUES ('A1', '测试法', '第一条')`,
		`INSERT INTO law (id, name, content) VALUES ('A2', '测试法', '第二条')`,
		`CREATE VIRTUAL TABLE law_fts USING fts5(content, content='law', content_rowid='rowid')`,
	} {
		if _, err := lawsDB.Exec(statement); err != nil {
			t.Fatalf("laws setup: %v", err)
		}
	}
	_ = lawsDB.Close()

	if err := store.OpenLawsDB(lawsPath); err != nil {
		t.Fatal(err)
	}
	if !store.LawsAvailable() {
		t.Error("打开后 LawsAvailable 应为 true")
	}
	if count := store.LawsCount(); count != 2 {
		t.Errorf("LawsCount = %d, want 2", count)
	}
	if !store.HasLawFTS() {
		t.Error("应检测到 law_fts 表")
	}
}
