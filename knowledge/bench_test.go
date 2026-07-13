package knowledge_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/knowledge/sqlite"
	"github.com/xujian519/mady/retrieval"
)

// benchEmbedder returns a pre-computed vector for any input, isolating
// retrieval performance from embedding service latency.
type benchEmbedder struct {
	vec []float32
}

func (e *benchEmbedder) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return [][]float32{e.vec}, nil
}

func (e *benchEmbedder) Dimensions() int { return len(e.vec) }

var (
	benchSQLiteStore *sqlite.SQLiteStore
	benchExt         *knowledge.KnowledgeExtension
	benchCtx         = context.Background()
	benchExtReady    bool
)

func sqliteDBPath() (string, error) {
	dir := os.Getenv("KNOWLEDGE_DB_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".mady", "knowledge")
	}
	p := filepath.Join(dir, "knowledge.db")
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}

func setupBenchExt(b *testing.B) {
	if benchExtReady {
		if benchExt == nil {
			b.Skip("knowledge.db not available")
		}
		return
	}
	benchExtReady = true

	dbPath, err := sqliteDBPath()
	if err != nil {
		b.Skipf("knowledge.db not found: %v", err)
		return
	}

	store, err := sqlite.NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("open store: %v", err)
	}
	benchSQLiteStore = store

	if err := store.PreloadVectors(); err != nil {
		b.Fatalf("preload vectors: %v", err)
	}

	queryVec := store.SampleVector()
	if queryVec == nil {
		b.Fatal("no sample vector available")
	}

	embedder := &benchEmbedder{vec: queryVec}

	ext := knowledge.NewExtension(nil, nil, "patent", knowledge.DefaultKnowledgeExtConfig())
	ext.WithBackend(store, embedder)
	benchExt = ext

	b.Logf("benchmark ext ready: dim=%d", store.EmbeddingDim())
}

// BenchmarkBackendSearch measures end-to-end backendSearch: FTS + Embed +
// VectorSearch (in-memory) + RRF fusion. This is the hot path executed on
// every model call when the SQLite backend is active.
//
// Run: go test -bench=BenchmarkBackendSearch -benchmem ./knowledge/
func BenchmarkBackendSearch(b *testing.B) {
	setupBenchExt(b)

	queries := []string{
		"专利侵权判定",
		"创造性判断标准",
		"权利要求保护范围",
		"等同原则适用",
		"专利申请审查流程",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q := queries[i%len(queries)]
		results := benchExt.Search(benchCtx, q, 5)
		if i == 0 {
			b.Logf("first query: %q → %d results", q, len(results))
		}
	}
}

// BenchmarkRRFFusion measures RRF fusion of pre-computed FTS + vector
// search results, isolating the fusion step from search I/O.
func BenchmarkRRFFusion(b *testing.B) {
	setupBenchExt(b)

	ftsResults, err := benchSQLiteStore.FTSSearch("专利侵权", 20)
	if err != nil {
		b.Fatal(err)
	}
	queryVec := benchSQLiteStore.SampleVector()
	vecResults, err := benchSQLiteStore.VectorSearch(queryVec, 20)
	if err != nil {
		b.Fatal(err)
	}
	lists := [][]retrieval.ScoredChunk{ftsResults, vecResults}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fuser := retrieval.NewRRFFuser()
		_ = fuser.Fuse(lists, 10)
	}
}
