package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/util"
)

// SnapshotStore implements agentcore.Store by writing JSON files to a local directory.
type SnapshotStore struct {
	dir string
}

// NewSnapshotStore creates a SnapshotStore rooted at the given directory.
// The directory is created (with 0o750 permissions) if it does not exist.
func NewSnapshotStore(dir string) (*SnapshotStore, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create store directory: %w", err)
	}
	return &SnapshotStore{dir: dir}, nil
}

// Save persists a state snapshot to a JSON file under the configured directory.
// Writes are atomic: data is first written to a .tmp file, then renamed.
func (fs *SnapshotStore) Save(_ context.Context, key string, snap agentcore.StateSnapshot) error {
	if err := util.ValidateKey(key); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	tmp := fs.path(key) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	if err := os.Rename(tmp, fs.path(key)); err != nil {
		return fmt.Errorf("rename snapshot: %w", err)
	}
	return nil
}

// Load reads and deserializes a state snapshot by key.
func (fs *SnapshotStore) Load(_ context.Context, key string) (agentcore.StateSnapshot, error) {
	if err := util.ValidateKey(key); err != nil {
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

// Delete removes a state snapshot file by key. No error is returned if the
// file does not exist.
func (fs *SnapshotStore) Delete(_ context.Context, key string) error {
	if err := util.ValidateKey(key); err != nil {
		return err
	}
	if err := os.Remove(fs.path(key)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete snapshot: %w", err)
	}
	return nil
}

// List returns all snapshot keys (without the .json extension) stored in the
// configured directory.
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

// Has checks whether a snapshot file exists for the given key.
func (fs *SnapshotStore) Has(_ context.Context, key string) (bool, error) {
	if err := util.ValidateKey(key); err != nil {
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
