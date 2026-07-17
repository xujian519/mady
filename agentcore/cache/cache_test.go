package cache

import (
	"testing"
	"time"
)

func TestDefaultPolicy(t *testing.T) {
	p := DefaultPolicy(StrategyAnthropicPrefix)
	if p.Strategy != StrategyAnthropicPrefix {
		t.Errorf("strategy: got %s, want %s", p.Strategy, StrategyAnthropicPrefix)
	}
	if p.TTL != 5*time.Minute {
		t.Errorf("TTL: got %v, want 5m", p.TTL)
	}
	if p.Priority != 8 {
		t.Errorf("priority: got %d, want 8", p.Priority)
	}
}

func TestGenericPolicy(t *testing.T) {
	p := DefaultPolicy(StrategyGeneric)
	if p.Priority != 0 {
		t.Errorf("generic priority: got %d, want 0", p.Priority)
	}
	if p.TTL != 0 {
		t.Errorf("generic TTL: got %v, want 0", p.TTL)
	}
}

func TestStatsRecord(t *testing.T) {
	s := &Stats{}

	s.RecordHit(1024)
	s.RecordHit(2048)
	s.RecordMiss()

	if s.HitRate() != 2.0/3.0 {
		t.Errorf("hit rate: got %.2f, want 0.67", s.HitRate())
	}

	snap := s.Snapshot()
	if snap.TotalCalls != 3 {
		t.Errorf("total calls: got %d, want 3", snap.TotalCalls)
	}
	if snap.TokensSaved != 3072 {
		t.Errorf("tokens saved: got %d, want 3072", snap.TokensSaved)
	}
}
