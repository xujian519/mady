package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/agentcore"
)

// SnapshotStore implements agentcore.Store by writing JSON files to a local directory.
type SnapshotStore struct {
	dir string
}

func NewSnapshotStore(dir string) (*SnapshotStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create store directory: %w", err)
	}
	return &SnapshotStore{dir: dir}, nil
}

func (fs *SnapshotStore) Save(_ context.Context, key string, snap agentcore.StateSnapshot) error {
	if err := validateKey(key); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := os.WriteFile(fs.path(key), data, 0o644); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	return nil
}

func (fs *SnapshotStore) Load(_ context.Context, key string) (agentcore.StateSnapshot, error) {
	if err := validateKey(key); err != nil {
		return agentcore.StateSnapshot{}, err
	}
	data, err := os.ReadFile(fs.path(key))
	if err != nil {
		return agentcore.StateSnapshot{}, fmt.Errorf("read snapshot: %w", err)
	}
	var snap agentcore.StateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return agentcore.StateSnapshot{}, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return snap, nil
}

func (fs *SnapshotStore) Delete(_ context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if err := os.Remove(fs.path(key)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete snapshot: %w", err)
	}
	return nil
}

func (fs *SnapshotStore) List(_ context.Context) ([]string, error) {
	entries, err := os.ReadDir(fs.dir)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	var keys []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			name := e.Name()
			keys = append(keys, name[:len(name)-5])
		}
	}
	return keys, nil
}

func (fs *SnapshotStore) Has(_ context.Context, key string) (bool, error) {
	if err := validateKey(key); err != nil {
		return false, err
	}
	_, err := os.Stat(fs.path(key))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("check snapshot: %w", err)
}

func (fs *SnapshotStore) path(key string) string {
	return filepath.Join(fs.dir, key+".json")
}

// validateKey rejects keys that could escape the store directory (path
// separators, ".", "..", or empty strings). Keys must resolve to a single
// path segment so that fs.path never writes/reads outside fs.dir.
func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("store key must not be empty")
	}
	if key == "." || key == ".." || key != filepath.Base(key) {
		return fmt.Errorf("invalid store key %q: must be a single path segment", key)
	}
	return nil
}
