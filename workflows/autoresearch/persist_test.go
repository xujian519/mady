package autoresearch

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestInMemoryResearchStore_SaveAndLoad(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryResearchStore()

	c := NewResearchContract("ar-store-1", "测试存储", "patent")
	c.Start()
	c.AdvanceRound()
	c.AddEvidence(Evidence{
		Round:    1,
		Summary:  "第一轮检索",
		Findings: []string{"发现A"},
	})

	err := s.SaveContract(ctx, c)
	if err != nil {
		t.Fatalf("SaveContract: %v", err)
	}

	loaded, err := s.LoadContract(ctx, "ar-store-1")
	if err != nil {
		t.Fatalf("LoadContract: %v", err)
	}

	if loaded.ID != c.ID {
		t.Errorf("ID: got %q, want %q", loaded.ID, c.ID)
	}
	if loaded.Goal != c.Goal {
		t.Errorf("Goal: got %q, want %q", loaded.Goal, c.Goal)
	}
	if loaded.Domain != c.Domain {
		t.Errorf("Domain: got %q, want %q", loaded.Domain, c.Domain)
	}
	if loaded.Status != c.Status {
		t.Errorf("Status: got %s, want %s", loaded.Status, c.Status)
	}
	if loaded.CurrentRound != c.CurrentRound {
		t.Errorf("CurrentRound: got %d, want %d", loaded.CurrentRound, c.CurrentRound)
	}
	if len(loaded.Evidence) != 1 {
		t.Errorf("evidence count: got %d, want 1", len(loaded.Evidence))
	}
}

func TestInMemoryResearchStore_NotFound(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryResearchStore()

	_, err := s.LoadContract(ctx, "nonexistent")
	if err != ErrContractNotFound {
		t.Errorf("expected ErrContractNotFound, got %v", err)
	}
}

func TestInMemoryResearchStore_ListActive(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryResearchStore()

	// Three contracts: one running, one paused, one completed.
	c1 := NewResearchContract("ar-active-1", "运行中", "patent")
	c1.Start()
	c2 := NewResearchContract("ar-active-2", "已暂停", "patent")
	c2.Start()
	c2.Pause()
	c3 := NewResearchContract("ar-active-3", "已完成", "patent")
	c3.Start()
	c3.Complete()

	for _, c := range []*ResearchContract{c1, c2, c3} {
		if err := s.SaveContract(ctx, c); err != nil {
			t.Fatalf("SaveContract: %v", err)
		}
	}

	active, err := s.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}

	if len(active) != 2 {
		t.Errorf("active count: got %d, want 2", len(active))
	}

	// Verify deep copy: modifying loaded contract should not affect store.
	if len(active) > 0 {
		active[0].Goal = "篡改测试"
		original, _ := s.LoadContract(ctx, active[0].ID)
		if original.Goal == "篡改测试" {
			t.Error("store should return deep copy, not original reference")
		}
	}
}

func TestInMemoryResearchStore_ListActive_Empty(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryResearchStore()

	active, err := s.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if active == nil {
		t.Error("ListActive should return empty slice, not nil")
	}
	if len(active) != 0 {
		t.Errorf("expected empty list, got %d items", len(active))
	}
}

func TestInMemoryResearchStore_SaveHeartbeat(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryResearchStore()

	c := NewResearchContract("ar-hb-store-1", "测试心跳存储", "legal")
	h := c.CreateHeartbeat(5*time.Second, 30*time.Second)
	h.Beat()

	if err := s.SaveHeartbeat(ctx, h); err != nil {
		t.Fatalf("SaveHeartbeat: %v", err)
	}

	// Verify the contract round-trips with heartbeat.
	if err := s.SaveContract(ctx, c); err != nil {
		t.Fatalf("SaveContract: %v", err)
	}

	loaded, err := s.LoadContract(ctx, "ar-hb-store-1")
	if err != nil {
		t.Fatalf("LoadContract: %v", err)
	}
	if loaded.Goal != c.Goal {
		t.Errorf("Goal: got %q, want %q", loaded.Goal, c.Goal)
	}
}

func TestInMemoryResearchStore_DeleteContract(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryResearchStore()

	c := NewResearchContract("ar-del-1", "测试删除", "patent")
	if err := s.SaveContract(ctx, c); err != nil {
		t.Fatalf("SaveContract: %v", err)
	}

	// Delete existing.
	if err := s.DeleteContract(ctx, "ar-del-1"); err != nil {
		t.Fatalf("DeleteContract: %v", err)
	}
	if s.Count() != 0 {
		t.Errorf("count after delete: got %d, want 0", s.Count())
	}

	// Delete non-existing (idempotent).
	if err := s.DeleteContract(ctx, "nonexistent"); err != nil {
		t.Errorf("idempotent delete should not error: %v", err)
	}
}

func TestInMemoryResearchStore_DeepCopyEvidence(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryResearchStore()

	c := NewResearchContract("ar-deep-1", "深拷贝测试", "patent")
	c.Start()
	c.AddEvidence(Evidence{
		Round:     1,
		Summary:   "原始",
		Findings:  []string{"发现1", "发现2"},
		ToolsUsed: []string{"patent_search"},
	})

	if err := s.SaveContract(ctx, c); err != nil {
		t.Fatalf("SaveContract: %v", err)
	}

	// Mutate original after save.
	c.Evidence[0].Findings[0] = "已篡改"

	loaded, _ := s.LoadContract(ctx, "ar-deep-1")
	if loaded.Evidence[0].Findings[0] != "发现1" {
		t.Errorf("deep copy failed: got %q, want %q",
			loaded.Evidence[0].Findings[0], "发现1")
	}
}

func TestInMemoryResearchStore_ConcurrentSafety(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryResearchStore()

	// Pre-save a contract.
	c := NewResearchContract("ar-con-1", "并发测试", "patent")
	c.Start()
	_ = s.SaveContract(ctx, c)

	var done sync.WaitGroup
	done.Add(3)

	// Writer 1: save.
	go func() {
		defer done.Done()
		for i := 0; i < 50; i++ {
			c2 := NewResearchContract("ar-con-1", "并发测试", "patent")
			c2.Start()
			_ = s.SaveContract(ctx, c2)
		}
	}()

	// Writer 2: save heartbeat.
	go func() {
		defer done.Done()
		h := NewHeartbeat("ar-con-1", 5*time.Second, 30*time.Second)
		for i := 0; i < 50; i++ {
			h.Beat()
			_ = s.SaveHeartbeat(ctx, h)
		}
	}()

	// Reader: load and list.
	go func() {
		defer done.Done()
		for i := 0; i < 50; i++ {
			_, _ = s.LoadContract(ctx, "ar-con-1")
			_, _ = s.ListActive(ctx)
			_ = s.Count()
		}
	}()

	done.Wait()
	// Primary assertion: -race passes without data race.
}
