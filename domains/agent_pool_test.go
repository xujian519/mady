package domains

import (
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
)

func TestAgentPool_CreateAndGet(t *testing.T) {
	base := agentcore.Config{}
	pool := NewTestAgentPool(base)
	defer pool.Close()

	rec := ProjectRecord{
		ProjectID: "test-001",
		Domain:    DomainPatent,
		Alias:     "测试案件",
		RootPath:  t.TempDir(),
		Status:    "active",
	}

	agent1, err := pool.GetOrCreate(rec)
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	if agent1 == nil {
		t.Fatal("GetOrCreate returned nil agent")
	}

	// Second call should return the same agent (cached)
	agent2, err := pool.GetOrCreate(rec)
	if err != nil {
		t.Fatalf("GetOrCreate (cached) failed: %v", err)
	}
	if agent2 != agent1 {
		t.Error("GetOrCreate should return cached agent on second call")
	}

	// Stats should show 1 agent
	stats := pool.Stats()
	if stats.TotalAgents != 1 {
		t.Errorf("expected 1 agent, got %d", stats.TotalAgents)
	}
	if stats.TotalHitCount < 1 {
		t.Error("expected hit count > 0")
	}
}

func TestAgentPool_MultipleProjects(t *testing.T) {
	base := agentcore.Config{}
	pool := NewTestAgentPool(base)
	defer pool.Close()

	recs := []ProjectRecord{
		{ProjectID: "proj-a", Domain: DomainPatent, RootPath: t.TempDir(), Status: "active"},
		{ProjectID: "proj-b", Domain: DomainLegal, RootPath: t.TempDir(), Status: "active"},
		{ProjectID: "proj-c", Domain: DomainPatent, RootPath: t.TempDir(), Status: "active"},
	}

	agents := make(map[string]*agentcore.Agent)
	for _, rec := range recs {
		agent, err := pool.GetOrCreate(rec)
		if err != nil {
			t.Fatalf("GetOrCreate %s failed: %v", rec.ProjectID, err)
		}
		agents[rec.ProjectID] = agent
	}

	stats := pool.Stats()
	if stats.TotalAgents != 3 {
		t.Errorf("expected 3 agents, got %d", stats.TotalAgents)
	}

	// Each project should have its own agent
	if agents["proj-a"] == agents["proj-b"] {
		t.Error("different projects should have different agents")
	}
}

func TestAgentPool_Evict(t *testing.T) {
	base := agentcore.Config{}
	pool := NewTestAgentPool(base)
	defer pool.Close()

	rec := ProjectRecord{
		ProjectID: "evict-me",
		Domain:    DomainChat,
		RootPath:  t.TempDir(),
		Status:    "active",
	}

	agent, err := pool.GetOrCreate(rec)
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	pool.Evict("evict-me")

	stats := pool.Stats()
	if stats.TotalAgents != 0 {
		t.Errorf("expected 0 agents after evict, got %d", stats.TotalAgents)
	}

	_ = agent // agent was closed by Evict
}

func TestAgentPool_IdleExpiry(t *testing.T) {
	base := agentcore.Config{}
	pool := NewTestAgentPool(base) // uses default 30min TTL
	defer pool.Close()

	rec := ProjectRecord{
		ProjectID: "idle-test",
		Domain:    DomainAssistant,
		RootPath:  t.TempDir(),
		Status:    "active",
	}

	_, err := pool.GetOrCreate(rec)
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	// Setting LastUsed to a very old time manually to trigger reap
	pool.mu.Lock()
	if entry, ok := pool.agents["idle-test"]; ok {
		entry.LastUsed = time.Now().Add(-2 * pool.idleTTL)
	}
	pool.mu.Unlock()

	// Trigger reap
	pool.reap()

	stats := pool.Stats()
	if stats.TotalAgents != 0 {
		t.Errorf("expected 0 agents after idle expiry, got %d", stats.TotalAgents)
	}
}

func TestAgentPool_MaxProjects(t *testing.T) {
	if maxCachedProjects < 5 {
		t.Skip("maxCachedProjects too small for this test")
	}

	base := agentcore.Config{}
	pool := NewTestAgentPool(base)
	defer pool.Close()

	// Create more projects than maxCachedProjects
	for i := 0; i < maxCachedProjects+5; i++ {
		rec := ProjectRecord{
			ProjectID: string(rune('A' + i%26)),
			Domain:    DomainChat,
			RootPath:  t.TempDir(),
			Status:    "active",
		}
		// Note: using the same IDs will overwrite, so not all will be cached
		// This test just verifies the pool handles the load gracefully
		_, err := pool.GetOrCreate(rec)
		if err != nil {
			t.Fatalf("GetOrCreate failed: %v", err)
		}
	}

	// Should not panic or error
	stats := pool.Stats()
	if stats.TotalAgents < 1 {
		t.Error("expected at least some agents in the pool")
	}
}
