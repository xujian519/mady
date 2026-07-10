package knowledge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/retrieval"
)

func TestStore_LoadText(t *testing.T) {
	store := NewStore()

	err := store.LoadText("patent", "test-1", "测试专利",
		"一种基于人工智能的专利检索方法，包括以下步骤：首先收集用户查询，然后通过关键词和语义混合检索。")
	if err != nil {
		t.Fatal(err)
	}

	doc, ok := store.GetDocument("test-1")
	if !ok {
		t.Fatal("document not found")
	}
	if doc.Title != "测试专利" {
		t.Errorf("title = %q, want %q", doc.Title, "测试专利")
	}

	chunks := store.ChunksForDomain("patent")
	if len(chunks) == 0 {
		t.Fatal("expected chunks for patent domain")
	}
}

func TestStore_LoadDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claims.txt")
	content := "## 权利要求 1\n一种基于中观哲学的多领域智能调度方法。\n\n## 权利要求 2\n根据权利要求1所述的方法，其特征在于还包括人机协作步骤。"
	os.WriteFile(path, []byte(content), 0644)

	store := NewStore()
	err := store.LoadDocument("patent", "claims-1", path)
	if err != nil {
		t.Fatal(err)
	}

	chunks := store.ChunksForDomain("patent")
	if len(chunks) == 0 {
		t.Fatal("expected chunks from file")
	}
}

func TestStore_Stats(t *testing.T) {
	store := NewStore()
	store.LoadText("patent", "p1", "专利1", "专利检索方法包括关键词检索和语义检索。")
	store.LoadText("legal", "l1", "法律1", "根据民法典相关规定，合同纠纷应优先协商解决。")

	stats := store.Stats()
	if stats.TotalDocs != 2 {
		t.Errorf("TotalDocs = %d, want 2", stats.TotalDocs)
	}
	if stats.TotalChunks == 0 {
		t.Error("expected non-zero chunks")
	}
}

func TestStore_DomainSeparation(t *testing.T) {
	store := NewStore()
	store.LoadText("patent", "p1", "专利", "IPC分类号检索")
	store.LoadText("legal", "l1", "法律", "民法典合同编")

	patentChunks := store.ChunksForDomain("patent")
	legalChunks := store.ChunksForDomain("legal")
	allChunks := store.AllChunks()

	if len(patentChunks) == 0 || len(legalChunks) == 0 {
		t.Fatal("expected chunks in both domains")
	}
	if len(allChunks) != len(patentChunks)+len(legalChunks) {
		t.Errorf("allChunks = %d, want %d", len(allChunks), len(patentChunks)+len(legalChunks))
	}
}

func TestStore_RetrievalHook(t *testing.T) {
	store := NewStore()
	store.LoadText("patent", "p1", "专利", "专利检索方法包括关键词检索和语义检索。")

	hook := store.RetrievalHook("patent", retrieval.DefaultRetrievalConfig())
	if hook == nil {
		t.Fatal("expected non-nil hook")
	}
}
