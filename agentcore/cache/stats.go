package cache

import "sync/atomic"

// Stats 记录缓存命中/未命中统计。
type Stats struct {
	Hits        atomic.Int64 `json:"hits"`
	Misses      atomic.Int64 `json:"misses"`
	TokensSaved atomic.Int64 `json:"tokens_saved"`
	TotalCalls  atomic.Int64 `json:"total_calls"`
}

// RecordHit 记录一次缓存命中。
func (s *Stats) RecordHit(tokensSaved int64) {
	s.Hits.Add(1)
	s.TotalCalls.Add(1)
	s.TokensSaved.Add(tokensSaved)
}

// RecordMiss 记录一次缓存未命中。
func (s *Stats) RecordMiss() {
	s.Misses.Add(1)
	s.TotalCalls.Add(1)
}

// HitRate 返回缓存命中率（0-1）。
func (s *Stats) HitRate() float64 {
	total := s.TotalCalls.Load()
	if total == 0 {
		return 0
	}
	return float64(s.Hits.Load()) / float64(total)
}

// Snapshot 返回当前统计的快照。
func (s *Stats) Snapshot() StatsSnapshot {
	return StatsSnapshot{
		Hits:        s.Hits.Load(),
		Misses:      s.Misses.Load(),
		TokensSaved: s.TokensSaved.Load(),
		TotalCalls:  s.TotalCalls.Load(),
	}
}

// StatsSnapshot 是 Stats 的不可变快照。
type StatsSnapshot struct {
	Hits        int64 `json:"hits"`
	Misses      int64 `json:"misses"`
	TokensSaved int64 `json:"tokens_saved"`
	TotalCalls  int64 `json:"total_calls"`
}
