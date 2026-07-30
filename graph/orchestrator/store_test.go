package orchestrator

import (
	"context"
	"sync"

	"github.com/xujian519/mady/graph"
)

// memoryCheckpointStore is an in-memory implementation of graph.CheckpointStore
// for testing.
type memoryCheckpointStore struct {
	mu          sync.RWMutex
	checkpoints map[string]graph.Checkpoint // keyed by ID
}

func newMemoryCheckpointStore() *memoryCheckpointStore {
	return &memoryCheckpointStore{
		checkpoints: make(map[string]graph.Checkpoint),
	}
}

func (s *memoryCheckpointStore) Save(_ context.Context, cp graph.Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkpoints[cp.ID] = cp
	return nil
}

func (s *memoryCheckpointStore) Load(_ context.Context, id string) (*graph.Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp, ok := s.checkpoints[id]
	if !ok {
		return nil, nil
	}
	return &cp, nil
}

func (s *memoryCheckpointStore) List(_ context.Context, graphID string) ([]graph.Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []graph.Checkpoint
	for _, cp := range s.checkpoints {
		if cp.GraphID == graphID {
			result = append(result, cp)
		}
	}
	return result, nil
}

func (s *memoryCheckpointStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.checkpoints, id)
	return nil
}

func (s *memoryCheckpointStore) LoadLatest(_ context.Context, graphID string) (*graph.Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest *graph.Checkpoint
	for _, cp := range s.checkpoints {
		if cp.GraphID != graphID {
			continue
		}
		if latest == nil || cp.StepIndex > latest.StepIndex {
			c := cp
			latest = &c
		}
	}
	return latest, nil
}
