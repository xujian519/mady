package store

import "context"

// VectorStore 是向量存储的统一接口。
// 由 knowledge/sqlite 和 memory/sqlite_store 分别实现适配器层。
type VectorStore interface {
	// Insert 插入一个向量到指定集合。
	// collection: 逻辑集合名称（如 "patents"、"laws"、"memories"）
	// id: 向量唯一标识
	// vector: 嵌入向量（float32 切片）
	// metadata: 可选的元数据键值对
	Insert(ctx context.Context, collection string, id string, vector []float32, metadata map[string]string) error

	// Search 在指定集合中搜索与 query 向量最相似的 topK 个结果。
	Search(ctx context.Context, collection string, query []float32, topK int) ([]ScoredResult, error)

	// Delete 从指定集合中删除一个向量。
	Delete(ctx context.Context, collection string, id string) error
}

// ScoredResult 是向量搜索的单个结果。
type ScoredResult struct {
	ID       string
	Score    float64
	Metadata map[string]string
}
