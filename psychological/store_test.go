package psychological

import (
	"testing"
)

func TestStoreSaveLoad(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	state := SDTState{Autonomy: 0.6, Competence: 0.7, Relatedness: 0.5, Motivation: 0.6}
	err = store.SaveSDTState("test-session", state, 5)
	if err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	data, err := store.LoadSDTState("test-session")
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if data == nil {
		t.Fatal("expected data, got nil")
	}
	if data.SDTState.Autonomy != 0.6 {
		t.Errorf("expected autonomy 0.6, got %f", data.SDTState.Autonomy)
	}
	if data.RoundCount != 5 {
		t.Errorf("expected round 5, got %d", data.RoundCount)
	}
}

func TestStoreLoadNonExistent(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)
	data, err := store.LoadSDTState("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != nil {
		t.Errorf("expected nil data for nonexistent session")
	}
}

func TestStoreOverwrite(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	state1 := SDTState{Autonomy: 0.3, Competence: 0.3, Relatedness: 0.5, Motivation: 0.36}
	store.SaveSDTState("overwrite-test", state1, 1)

	state2 := SDTState{Autonomy: 0.7, Competence: 0.8, Relatedness: 0.6, Motivation: 0.7}
	store.SaveSDTState("overwrite-test", state2, 10)

	data, _ := store.LoadSDTState("overwrite-test")
	if data.SDTState.Autonomy != 0.7 {
		t.Errorf("expected overwritten autonomy 0.7, got %f", data.SDTState.Autonomy)
	}
	if data.RoundCount != 10 {
		t.Errorf("expected overwritten round 10, got %d", data.RoundCount)
	}
}
