package sqlite

import (
	"os"
	"path/filepath"
	"testing"
)

// 专利法律领域查询样本——用于 FTS 和端到端 benchmark
var benchQueries = []string{
	"专利侵权判定",
	"创造性判断标准",
	"权利要求保护范围",
	"等同原则适用",
	"专利申请审查流程",
}

var (
	benchStore    *SQLiteStore // 预加载了向量索引
	benchStoreSQL *SQLiteStore // 未预加载，用于 SQL 对比
	benchQueryVec []float32    // 从 DB 取的查询向量
	benchReady    bool
)

func benchDBPath() (string, error) {
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

func setupBench(b *testing.B) {
	if benchReady {
		if benchStore == nil {
			b.Skip("knowledge.db not available")
		}
		return
	}
	benchReady = true

	dbPath, err := benchDBPath()
	if err != nil {
		b.Skipf("knowledge.db not found: %v", err)
		return
	}

	s1, err := NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("open store: %v", err)
	}
	benchStore = s1

	s2, err := NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("open storeSQL: %v", err)
	}
	benchStoreSQL = s2

	if err := benchStore.PreloadVectors(); err != nil {
		b.Fatalf("preload vectors: %v", err)
	}

	benchQueryVec = benchStore.SampleVector()
	if benchQueryVec == nil {
		b.Fatal("no sample vector available")
	}

	b.Logf("benchmark env: %d vectors, dim=%d, queries=%d",
		benchStore.vecIndex.Count(), benchStore.vecIndex.Dim(), len(benchQueries))
}

// BenchmarkPreloadVectorIndex measures loading all 144K vectors from SQLite
// into memory. This is a one-time startup cost (~562 MB for BGE-M3 1024-dim).
// Run: go test -bench=BenchmarkPreloadVectorIndex -benchmem ./knowledge/sqlite/
func BenchmarkPreloadVectorIndex(b *testing.B) {
	dbPath, err := benchDBPath()
	if err != nil {
		b.Skipf("knowledge.db not found: %v", err)
	}
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.PreloadVectorIndex()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFTSSearch measures BM25 + trigram FTS5 search performance.
func BenchmarkFTSSearch(b *testing.B) {
	setupBench(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q := benchQueries[i%len(benchQueries)]
		_, err := benchStore.FTSSearch(q, 10)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVectorIndexSearch measures pure in-memory parallel brute-force
// vector search (no SQL IO for chunk content retrieval). This isolates
// the compute cost from the I/O cost.
func BenchmarkVectorIndexSearch(b *testing.B) {
	setupBench(b)
	idx := benchStore.vecIndex

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.Search(benchQueryVec, 10)
	}
}

// BenchmarkVectorSearchInMemory measures end-to-end vector search with
// in-memory index (includes chunk content retrieval via SQL getChunk).
func BenchmarkVectorSearchInMemory(b *testing.B) {
	setupBench(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := benchStore.VectorSearch(benchQueryVec, 10)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVectorSearchSQL measures SQL batch vector search (fallback path,
// no in-memory index). Very slow (~3-4s per query for 144K vectors).
// Run with: -benchtime=1x to limit to a single iteration.
func BenchmarkVectorSearchSQL(b *testing.B) {
	setupBench(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := benchStoreSQL.VectorSearch(benchQueryVec, 10)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetChunk measures single chunk retrieval by integer ID.
func BenchmarkGetChunk(b *testing.B) {
	setupBench(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = benchStore.getChunk(1 + (i % 1000))
	}
}
