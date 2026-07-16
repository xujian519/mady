package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/knowledge/sqlite"
	"github.com/xujian519/mady/retrieval/domain"
)

// realStore opens the local knowledge.db for integration testing. Skips
// when the file is absent (e.g. CI without the xiaonuo corpus), so these
// tests never fail on missing external data.
func realStore(t *testing.T) *sqlite.SQLiteStore {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("UserHomeDir: %v", err)
	}
	p := filepath.Join(home, ".mady", "knowledge", "knowledge.db")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("knowledge.db not found at %s", p)
	}
	store, err := sqlite.NewSQLiteStore(p)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestNewPatentDomainRetriever_NilOnNilStore(t *testing.T) {
	if got := NewPatentDomainRetriever(nil); got != nil {
		t.Fatalf("NewPatentDomainRetriever(nil) = %v, want nil", got)
	}
}

func TestPatentDomainRetriever_SourceName(t *testing.T) {
	r := &PatentDomainRetriever{}
	if got := r.SourceName(); got != SourceNamePatent {
		t.Errorf("SourceName() = %q, want %q", got, SourceNamePatent)
	}
}

func TestPatentDomainRetriever_NilReceiverSearch(t *testing.T) {
	var r *PatentDomainRetriever
	res, err := r.Search(context.Background(), domain.DomainQuery{Text: "x"})
	if err != nil {
		t.Fatalf("nil receiver Search: %v", err)
	}
	if res == nil || len(res.Documents) != 0 {
		t.Fatalf("nil receiver got %+v, want empty result", res)
	}
}

func TestPatentDomainRetriever_Search_Integration(t *testing.T) {
	store := realStore(t)
	r := NewPatentDomainRetriever(store)

	res, err := r.Search(context.Background(), domain.DomainQuery{
		Text:       "创造性",
		Keywords:   []string{"三步法", "技术启示"},
		MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if res.Source != SourceNamePatent {
		t.Errorf("Source = %q, want %q", res.Source, SourceNamePatent)
	}
	if len(res.Documents) == 0 {
		t.Skip("knowledge.db returned 0 docs for 新颖性判断; corpus may not cover this term")
	}

	// Verify field mapping + score normalization. Documents come from a map
	// merge so they are unordered; find the max-score doc to assert the
	// normalization ceiling, and check all scores are in (0,1].
	var maxScore float64
	var maxDoc domain.DomainDocument
	for _, d := range res.Documents {
		if d.ID == "" {
			t.Error("Document.ID empty")
		}
		if d.Content == "" {
			t.Error("Document.Content should hold chunk text")
		}
		if d.Score <= 0 || d.Score > 1.0 {
			t.Errorf("Score = %v, want in (0,1]", d.Score)
		}
		if d.Score > maxScore {
			maxScore = d.Score
			maxDoc = d
		}
	}
	// The highest-scoring doc normalizes to 1.0 (rawMax/rawMax).
	if maxScore < 1.0-1e-9 {
		t.Errorf("max normalized score = %v, want 1.0", maxScore)
	}
	t.Logf("Search returned %d docs; max: id=%s title=%q score=%.3f",
		len(res.Documents), maxDoc.ID, maxDoc.Title, maxDoc.Score)
}

func TestPatentDomainRetriever_GetDocument_Integration(t *testing.T) {
	store := realStore(t)
	r := NewPatentDomainRetriever(store)

	// First find a real docID via Search, then expand it via GetDocument.
	res, err := r.Search(context.Background(), domain.DomainQuery{
		Text:       "创造性",
		MaxResults: 3,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Documents) == 0 {
		t.Skip("no docs to expand; corpus may not cover 创造性")
	}
	docID := res.Documents[0].ID

	doc, err := r.GetDocument(context.Background(), docID)
	if err != nil {
		t.Fatalf("GetDocument(%s): %v", docID, err)
	}
	if doc == nil {
		t.Fatalf("GetDocument(%s) = nil, want expanded doc", docID)
	}
	if doc.ID != docID {
		t.Errorf("expanded ID = %q, want %q", doc.ID, docID)
	}
	if doc.Content == "" {
		t.Error("expanded Content should concatenate chunk text")
	}
	if len(doc.Content) < len(res.Documents[0].Content) {
		t.Error("expanded Content should be >= single-chunk Content")
	}
}

func TestPatentDomainRetriever_GetDocument_EmptyID(t *testing.T) {
	r := &PatentDomainRetriever{}
	doc, err := r.GetDocument(context.Background(), "")
	if err != nil {
		t.Fatalf("empty ID: %v", err)
	}
	if doc != nil {
		t.Fatalf("empty ID got %+v, want nil", doc)
	}
}

func TestPatentDomainRetriever_GetDocument_UnknownID(t *testing.T) {
	store := realStore(t)
	r := NewPatentDomainRetriever(store)

	doc, err := r.GetDocument(context.Background(), "nonexistent-doc-id-xyz")
	if err != nil {
		t.Fatalf("unknown ID should not error, got %v", err)
	}
	if doc != nil {
		t.Fatalf("unknown ID got %+v, want nil", doc)
	}
}
