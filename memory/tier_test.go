package memory

import (
	"testing"
	"time"
)

func TestMigrateEntries_HotToWarm(t *testing.T) {
	now := time.Now()
	old := now.Add(-8 * 24 * time.Hour) // 8 days ago

	entries := []MemoryEntry{
		{ID: "1", Tier: TierHot, LastAccess: old, AccessCount: 2},
		{ID: "2", Tier: TierHot, LastAccess: now, AccessCount: 10},
	}

	updated, pruned := MigrateEntries(entries, 7, 30, 90, 3)

	if len(updated) != 1 {
		t.Fatalf("expected 1 updated entry, got %d", len(updated))
	}
	if updated[0].ID != "1" {
		t.Errorf("expected entry 1 to be migrated, got %s", updated[0].ID)
	}
	if updated[0].Tier != TierWarm {
		t.Errorf("expected TierWarm, got %s", updated[0].Tier)
	}
	if len(pruned) != 0 {
		t.Errorf("expected 0 pruned, got %d", len(pruned))
	}
}

func TestMigrateEntries_WarmToCold(t *testing.T) {
	now := time.Now()
	old := now.Add(-35 * 24 * time.Hour) // 35 days ago

	entries := []MemoryEntry{
		{ID: "1", Tier: TierWarm, LastAccess: old},
		{ID: "2", Tier: TierWarm, LastAccess: now},
	}

	updated, pruned := MigrateEntries(entries, 7, 30, 90, 3)

	if len(updated) != 1 {
		t.Fatalf("expected 1 updated, got %d", len(updated))
	}
	if updated[0].Tier != TierCold {
		t.Errorf("expected TierCold, got %s", updated[0].Tier)
	}
	if len(pruned) != 0 {
		t.Errorf("expected 0 pruned, got %d", len(pruned))
	}
}

func TestMigrateEntries_ColdPrune(t *testing.T) {
	now := time.Now()
	old := now.Add(-100 * 24 * time.Hour) // 100 days ago

	entries := []MemoryEntry{
		{ID: "1", Tier: TierCold, LastAccess: old},
		{ID: "2", Tier: TierCold, LastAccess: now},
	}

	updated, pruned := MigrateEntries(entries, 7, 30, 90, 3)

	if len(updated) != 0 {
		t.Errorf("expected 0 updated, got %d", len(updated))
	}
	if len(pruned) != 1 {
		t.Fatalf("expected 1 pruned, got %d", len(pruned))
	}
	if pruned[0] != "1" {
		t.Errorf("expected entry 1 to be pruned, got %s", pruned[0])
	}
}

func TestMigrateEntries_EternalNeverMigrates(t *testing.T) {
	old := time.Now().Add(-200 * 24 * time.Hour) // very old

	entries := []MemoryEntry{
		{ID: "eternal", Tier: TierEternal, LastAccess: old},
		{ID: "hot-old", Tier: TierHot, LastAccess: old, AccessCount: 0},
	}

	updated, pruned := MigrateEntries(entries, 7, 30, 90, 3)

	// Eternal should not be in updated or pruned
	for _, u := range updated {
		if u.ID == "eternal" {
			t.Error("eternal entry should not be migrated")
		}
	}
	for _, p := range pruned {
		if p == "eternal" {
			t.Error("eternal entry should not be pruned")
		}
	}
	// hot-old should be migrated
	if len(updated) != 1 || updated[0].ID != "hot-old" {
		t.Errorf("expected hot-old to be migrated, got updated=%v", updated)
	}
}

func TestPromoteOnAccess(t *testing.T) {
	entry := MemoryEntry{
		ID:          "1",
		Tier:        TierWarm,
		AccessCount: 3,
		LastAccess:  time.Now().Add(-10 * 24 * time.Hour),
	}

	promoted := PromoteOnAccess(entry)

	if promoted.Tier != TierHot {
		t.Errorf("expected TierHot after promotion, got %s", promoted.Tier)
	}
	if promoted.AccessCount != 4 {
		t.Errorf("expected AccessCount 4, got %d", promoted.AccessCount)
	}
	if promoted.LastAccess.Before(time.Now().Add(-time.Second)) {
		t.Error("expected LastAccess to be updated to now")
	}
}

func TestPromoteOnAccess_AlreadyHot(t *testing.T) {
	entry := MemoryEntry{
		ID:          "1",
		Tier:        TierHot,
		AccessCount: 5,
	}

	promoted := PromoteOnAccess(entry)

	if promoted.Tier != TierHot {
		t.Errorf("expected TierHot to remain, got %s", promoted.Tier)
	}
	if promoted.AccessCount != 6 {
		t.Errorf("expected AccessCount 6, got %d", promoted.AccessCount)
	}
}

func TestMarkEternal_UnmarkEternal(t *testing.T) {
	entry := MemoryEntry{ID: "1", Tier: TierHot}

	eternal := MarkEternal(entry)
	if eternal.Tier != TierEternal {
		t.Errorf("expected TierEternal, got %s", eternal.Tier)
	}

	unmarked := UnmarkEternal(eternal)
	if unmarked.Tier != TierHot {
		t.Errorf("expected TierHot after unmark, got %s", unmarked.Tier)
	}
}

func TestUnmarkEternal_NonEternal(t *testing.T) {
	entry := MemoryEntry{ID: "1", Tier: TierCold}

	unmarked := UnmarkEternal(entry)
	if unmarked.Tier != TierCold {
		t.Errorf("expected TierCold to remain, got %s", unmarked.Tier)
	}
}

func TestMigrateEntries_EmptyTier(t *testing.T) {
	// "" (unset) tier should be treated as HOT for backward compatibility
	old := time.Now().Add(-8 * 24 * time.Hour)

	entries := []MemoryEntry{
		{ID: "1", Tier: "", LastAccess: old, AccessCount: 1},
	}

	updated, _ := MigrateEntries(entries, 7, 30, 90, 3)

	if len(updated) != 1 {
		t.Fatalf("expected 1 updated for unset tier, got %d", len(updated))
	}
	if updated[0].Tier != TierWarm {
		t.Errorf("expected TierWarm for unset tier migration, got %s", updated[0].Tier)
	}
}
