// Package internal 提供内部共享的向量工具函数，供 retrieval、knowledge、memory 包使用。
// 此包仅限本模块内使用（Go internal 约束），对外不提供 API 稳定性保证。
package internal

import (
	"encoding/binary"
	"math"
)

// FloatsToBytes 将 float32 切片编码为 little-endian BLOB。
// 与 knowledge/sqlite 和 memory/sqlite_store 的 BLOB 存储格式一致。
func FloatsToBytes(vec []float32) []byte {
	if len(vec) == 0 {
		return nil
	}
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// BytesToFloats 将 little-endian BLOB 解码为 float32 切片。
// 是 FloatsToBytes 的逆操作。输入长度不是 4 的倍数或为空时返回 nil。
func BytesToFloats(b []byte) []float32 {
	if len(b)%4 != 0 || len(b) == 0 {
		return nil
	}
	vec := make([]float32, len(b)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return vec
}

// CosineSimilarity 已迁移到 retrieval.CosineSimilarity (retrieval/embedding.go)。
// 本包不再重复定义，保持一致的事实来源。

// L2Norm 计算 float32 向量的 L2 范数。
func L2Norm(vec []float32) float64 {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	return math.Sqrt(sum)
}

// TopKByScore 从 items 中选择分数最高的 topK 个元素。
// items 不会被修改，返回结果按分数降序排列。
func TopKByScore[T any](items []T, score func(T) float64, topK int) []T {
	if topK <= 0 || len(items) == 0 {
		return nil
	}
	if topK > len(items) {
		topK = len(items)
	}

	// 使用简单的选择排序（items 通常较小，且清晰可读）
	// 如果未来有性能需求，可替换为 container/heap 实现
	indices := make([]int, len(items))
	for i := range indices {
		indices[i] = i
	}

	// 按分数降序排列
	for i := 0; i < topK; i++ {
		maxIdx := i
		for j := i + 1; j < len(indices); j++ {
			if score(items[indices[j]]) > score(items[indices[maxIdx]]) {
				maxIdx = j
			}
		}
		indices[i], indices[maxIdx] = indices[maxIdx], indices[i]
	}

	result := make([]T, topK)
	for i, idx := range indices[:topK] {
		result[i] = items[idx]
	}
	return result
}

// RRFScore 计算倒数排名融合（Reciprocal Rank Fusion）的单项分数。
// k 是平滑常数，通常设为 60。
func RRFScore(rank int, k float64) float64 {
	if rank < 0 {
		return 0
	}
	return 1.0 / (k + float64(rank))
}
