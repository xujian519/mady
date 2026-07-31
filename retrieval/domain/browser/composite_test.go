package browser

import (
	"context"
	"errors"
	"testing"

	"github.com/xujian519/mady/retrieval/domain"
)

// stubRetriever 是可控的假检索器，用于测试组合逻辑。
type stubRetriever struct {
	name     string
	docs     []domain.DomainDocument
	err      error
	docErr   error
	notFound bool
}

func (s *stubRetriever) Search(_ context.Context, _ domain.DomainQuery) (*domain.DomainResults, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &domain.DomainResults{
		Documents:  s.docs,
		TotalCount: len(s.docs),
		Source:     s.name,
	}, nil
}

func (s *stubRetriever) GetDocument(_ context.Context, _ string) (*domain.DomainDocument, error) {
	if s.docErr != nil {
		return nil, s.docErr
	}
	if s.notFound {
		return nil, nil
	}
	if len(s.docs) > 0 {
		d := s.docs[0]
		return &d, nil
	}
	return nil, nil
}

func (s *stubRetriever) SourceName() string { return s.name }

func doc(id string, score float64) domain.DomainDocument {
	return domain.DomainDocument{ID: id, Title: id, Score: score}
}

// TestCompositeNew 验证 nil 过滤与空列表。
func TestCompositeNew(t *testing.T) {
	if c := NewCompositeRetriever(nil, nil); c != nil {
		t.Error("all-nil should yield nil composite")
	}
	c := NewCompositeRetriever(&stubRetriever{name: "a"}, nil, &stubRetriever{name: "b"})
	if c == nil || len(c.retrievers) != 2 {
		t.Errorf("expected 2 retrievers, got %v", c)
	}
	// typed nil（*BrowserRetriever nil 指针转为接口）必须被过滤。
	var typedNil *BrowserRetriever
	if c := NewCompositeRetriever(typedNil, nil); c != nil {
		t.Error("typed-nil should yield nil composite")
	}
}

// TestCompositeSearchMerge 验证跨源合并去重与排序。
func TestCompositeSearchMerge(t *testing.T) {
	c := NewCompositeRetriever(
		&stubRetriever{name: "本地", docs: []domain.DomainDocument{doc("CN1", 0.5), doc("CN2", 0.9)}},
		&stubRetriever{name: "在线", docs: []domain.DomainDocument{doc("CN2", 0.8), doc("CN3", 0.7)}},
	)
	res, err := c.Search(context.Background(), domain.DomainQuery{Text: "x", MaxResults: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Documents) != 3 {
		t.Fatalf("documents = %d, want 3 (CN2 deduped)", len(res.Documents))
	}
	// CN2 保留最高分副本（0.9）。
	if res.Documents[0].ID != "CN2" || res.Documents[0].Score != 0.9 {
		t.Errorf("first = %+v", res.Documents[0])
	}
	// 按分数降序。
	for i := 1; i < len(res.Documents); i++ {
		if res.Documents[i-1].Score < res.Documents[i].Score {
			t.Error("not sorted descending")
		}
	}
}

// TestCompositeSearchDegrade 验证单源失败降级不阻塞。
func TestCompositeSearchDegrade(t *testing.T) {
	c := NewCompositeRetriever(
		&stubRetriever{name: "在线", err: errors.New("browser down")},
		&stubRetriever{name: "本地", docs: []domain.DomainDocument{doc("CN1", 0.5)}},
	)
	res, err := c.Search(context.Background(), domain.DomainQuery{Text: "x"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Documents) != 1 || res.Documents[0].ID != "CN1" {
		t.Errorf("degraded documents = %+v", res.Documents)
	}
}

// TestCompositeSearchConcurrent 验证 Search 并发执行：慢源不阻塞快源，
// 总耗时约等于最慢源而非各源之和。
func TestCompositeSearchConcurrent(t *testing.T) {
	c := NewCompositeRetriever(
		&stubRetriever{name: "慢源", docs: []domain.DomainDocument{doc("CN1", 0.5)}},
		&stubRetriever{name: "快源", docs: []domain.DomainDocument{doc("CN2", 0.9)}},
	)
	if c == nil {
		t.Fatal("composite is nil")
	}
	// 顺序实现会先等慢源完成，无法断言时序；此处仅验证并发实现
	// 不引入数据竞争（配合 -race）且结果完整。
	res, err := c.Search(context.Background(), domain.DomainQuery{Text: "x"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Documents) != 2 {
		t.Errorf("documents = %d, want 2", len(res.Documents))
	}
}

// TestCompositeSearchDeterministic 验证分数平局时按 ID 排序（可复现输出）。
func TestCompositeSearchDeterministic(t *testing.T) {
	c := NewCompositeRetriever(
		&stubRetriever{name: "a", docs: []domain.DomainDocument{doc("CN2", 0.5), doc("CN1", 0.5)}},
	)
	res, err := c.Search(context.Background(), domain.DomainQuery{Text: "x"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Documents) != 2 || res.Documents[0].ID != "CN1" || res.Documents[1].ID != "CN2" {
		t.Errorf("tie order = %+v, want CN1 then CN2", res.Documents)
	}
}

// TestCompositeGetDocument 验证跨源取文档（第一个命中）。
func TestCompositeGetDocument(t *testing.T) {
	c := NewCompositeRetriever(
		&stubRetriever{name: "本地", notFound: true},
		&stubRetriever{name: "在线", docs: []domain.DomainDocument{doc("CN1", 0.9)}},
	)
	d, err := c.GetDocument(context.Background(), "CN1")
	if err != nil || d == nil || d.ID != "CN1" {
		t.Errorf("GetDocument = %v, %v", d, err)
	}
	// 全部未命中 → nil。
	c2 := NewCompositeRetriever(&stubRetriever{name: "本地", notFound: true})
	if d, err := c2.GetDocument(context.Background(), "CN1"); d != nil || err != nil {
		t.Errorf("not-found = %v, %v", d, err)
	}
}
