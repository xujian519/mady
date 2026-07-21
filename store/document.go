package store

import "context"

// DocumentStore 是文档存储的统一接口。
type DocumentStore interface {
	// InsertDocument 插入一个文档到指定集合。
	InsertDocument(ctx context.Context, collection string, doc Document) error

	// SearchDocuments 在指定集合中搜索与 query 匹配的 topK 个文档。
	// query 可以是自然语言查询，由实现决定搜索方式（FTS/向量/混合）。
	SearchDocuments(ctx context.Context, collection string, query string, topK int) ([]ScoredDocument, error)

	// DeleteDocument 从指定集合中删除一个文档。
	DeleteDocument(ctx context.Context, collection string, id string) error
}

// Document 表示一个可存储的文档。
type Document struct {
	ID       string
	Title    string
	Content  string
	Source   string
	Sections []DocumentSection
	Metadata map[string]string
}

// DocumentSection 是文档的一个分区（段落/节）。
type DocumentSection struct {
	Index   int
	Heading string
	Content string
}

// ScoredDocument 是文档搜索结果，附带相关性分数。
type ScoredDocument struct {
	Document
	Score float64
}
