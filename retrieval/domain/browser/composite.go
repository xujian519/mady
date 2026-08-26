package browser

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/xujian519/mady/retrieval/domain"
)

// CompositeRetriever 组合多个 domain.DomainRetriever（如本地 FTS5 语料 +
// 在线专利数据库），Search 时向所有子检索器并发查询并合并去重。
// 单个子检索器失败时降级跳过（记录警告），不阻塞整体结果。
type CompositeRetriever struct {
	retrievers []domain.DomainRetriever
}

// 编译期接口合规检查。
var _ domain.DomainRetriever = (*CompositeRetriever)(nil)

// NewCompositeRetriever 构造组合检索器。nil 成员被过滤（含 typed nil——
// 工厂函数返回 *BrowserRetriever 指针，nil 指针转为接口后非 nil，需反射判断）；
// 空列表返回 nil。
func NewCompositeRetriever(retrievers ...domain.DomainRetriever) *CompositeRetriever {
	kept := make([]domain.DomainRetriever, 0, len(retrievers))
	for _, r := range retrievers {
		if !isNilInterface(r) {
			kept = append(kept, r)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return &CompositeRetriever{retrievers: kept}
}

// isNilInterface 判断接口值是否为空：覆盖未赋值的 nil 接口与 typed nil
// （如 nil 指针、nil 切片等）。
func isNilInterface(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return rv.IsNil()
	}
	return false
}

// Search 并发查询所有子检索器（浏览器源各自独立进程，串行会叠加超时），
// 按分数降序合并去重（保留分数最高的副本）。单源失败降级跳过。
func (c *CompositeRetriever) Search(ctx context.Context, query domain.DomainQuery) (*domain.DomainResults, error) {
	results := make([]*domain.DomainResults, len(c.retrievers))
	var wg sync.WaitGroup
	for i, r := range c.retrievers {
		wg.Add(1)
		go func(i int, r domain.DomainRetriever) {
			defer wg.Done()
			res, err := r.Search(ctx, query)
			if err != nil {
				slog.Warn("composite retriever: 子检索器失败，跳过该源", "source", r.SourceName(), "error", err)
				return
			}
			results[i] = res
		}(i, r)
	}
	wg.Wait()

	merged := make(map[string]domain.DomainDocument)
	for _, res := range results {
		if res == nil {
			continue
		}
		for _, doc := range res.Documents {
			if doc.ID == "" {
				continue
			}
			if existing, ok := merged[doc.ID]; !ok || doc.Score > existing.Score {
				merged[doc.ID] = doc
			}
		}
	}

	docs := make([]domain.DomainDocument, 0, len(merged))
	for _, doc := range merged {
		docs = append(docs, doc)
	}
	// 分数降序；平局按 ID 升序，保证并发下输出可复现。
	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Score != docs[j].Score {
			return docs[i].Score > docs[j].Score
		}
		return docs[i].ID < docs[j].ID
	})

	topK := query.MaxResults
	if topK <= 0 {
		topK = 10
	}
	if len(docs) > topK {
		docs = docs[:topK]
	}
	return &domain.DomainResults{
		Query:      query,
		Documents:  docs,
		TotalCount: len(docs),
		Source:     c.SourceName(),
	}, nil
}

// GetDocument 依次尝试子检索器，返回第一个命中。
// 全源未命中返回 (nil, nil)（缺失文档是正常"未命中"）；所有源均报错时
// 返回聚合错误，供调用方区分"找不到文档"与"取文档失败"，避免吞掉真实
// 故障而误导上层给出"未收录"结论。
func (c *CompositeRetriever) GetDocument(ctx context.Context, docID string) (*domain.DomainDocument, error) {
	var errs []error
	for _, r := range c.retrievers {
		doc, err := r.GetDocument(ctx, docID)
		if err != nil {
			slog.Warn("composite retriever: 子检索器取文档失败，尝试下一源", "source", r.SourceName(), "error", err)
			errs = append(errs, fmt.Errorf("%s: %w", r.SourceName(), err))
			continue
		}
		if doc != nil {
			return doc, nil
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("全源取文档失败: %w", errors.Join(errs...))
	}
	return nil, nil
}

// SourceName 返回组合源的描述。
func (c *CompositeRetriever) SourceName() string {
	names := make([]string, 0, len(c.retrievers))
	for _, r := range c.retrievers {
		names = append(names, r.SourceName())
	}
	return "Composite(" + strings.Join(names, " + ") + ")"
}
