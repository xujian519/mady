package filecheckpoint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// FileSystem operations are abstracted for testability.
type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	Stat(path string) (os.FileInfo, error)
	WriteFile(path string, content []byte) error
	Remove(path string) error
}

// OSFileSystem is the default FileSystem backed by the local disk.
type OSFileSystem struct{}

func (OSFileSystem) ReadFile(path string) ([]byte, error)  { return os.ReadFile(path) }
func (OSFileSystem) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }
func (OSFileSystem) WriteFile(path string, content []byte) error {
	return os.WriteFile(path, content, 0600)
}
func (OSFileSystem) Remove(path string) error { return os.Remove(path) }

// Store holds a session's checkpoints in memory. All methods are safe for
// concurrent use — the agent snapshots from tool hooks.
type Store struct {
	fs   FileSystem
	root string // workspace root for path-escape guards

	mu   sync.Mutex
	done []*TurnCheckpoint
	cur  *TurnCheckpoint
	seen map[string]bool // paths already snapshotted this turn (dedup)
}

// New returns a store for the given workspace root. An empty root disables
// path-escape checks (use only in tests).
func New(fs FileSystem, root string) *Store {
	if fs == nil {
		fs = OSFileSystem{}
	}
	return &Store{
		fs:   fs,
		root: root,
		seen: map[string]bool{},
	}
}

// BeginTurn opens a checkpoint for a new user turn, finalizing the previous one.
func (s *Store) BeginTurn(turn int64, prompt string, msgIndex int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur != nil {
		s.done = append(s.done, s.cur)
	}
	s.cur = &TurnCheckpoint{
		Turn:     turn,
		Time:     time.Now(),
		Prompt:   prompt,
		MsgIndex: msgIndex,
	}
	s.seen = map[string]bool{}
}

// SnapshotFile records the pre-edit state of path if it hasn't been snapshotted
// this turn. This must be called BEFORE the file is modified.
func (s *Store) SnapshotFile(path string) error {
	if !s.isPathSafe(path) {
		return fmt.Errorf("snapshot: path %q escapes workspace root", path)
	}
	s.mu.Lock()
	if s.cur == nil {
		s.mu.Unlock()
		return fmt.Errorf("no active turn checkpoint")
	}
	if s.seen[path] {
		s.mu.Unlock()
		return nil // already snapshotted
	}
	s.seen[path] = true
	s.mu.Unlock()

	content, err := s.fs.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet — record as nil so restore can delete it.
			s.mu.Lock()
			s.cur.Files = append(s.cur.Files, FileSnap{Path: path, Content: nil})
			s.mu.Unlock()
			return nil
		}
		return fmt.Errorf("snapshot %s: %w", path, err)
	}
	c := string(content)
	s.mu.Lock()
	s.cur.Files = append(s.cur.Files, FileSnap{Path: path, Content: &c})
	s.mu.Unlock()
	return nil
}

// EndTurn finalizes the current turn checkpoint.
func (s *Store) EndTurn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur != nil {
		s.done = append(s.done, s.cur)
		s.cur = nil
	}
}

// List returns metadata for all finalized checkpoints, oldest first.
func (s *Store) List() []Meta {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Meta, 0, len(s.done))
	for _, c := range s.done {
		m := Meta{Turn: c.Turn, Time: c.Time, Prompt: c.Prompt}
		for _, f := range c.Files {
			m.Paths = append(m.Paths, f.Path)
		}
		out = append(out, m)
	}
	return out
}

// Restore reverts the workspace files to their state at the given turn.
// Files created after the checkpoint are deleted; modified files are reverted.
// The lock is held throughout file IO to prevent TOCTOU races with concurrent
// BeginTurn (matching RestoreAndTrim's approach).
func (s *Store) Restore(turn int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var target *TurnCheckpoint
	for _, c := range s.done {
		if c.Turn == turn {
			target = c
			break
		}
	}

	if target == nil {
		return fmt.Errorf("no checkpoint for turn %d", turn)
	}

	for _, f := range target.Files {
		if err := s.restoreFile(f); err != nil {
			return fmt.Errorf("restore %s: %w", f.Path, err)
		}
	}
	return nil
}

func (s *Store) restoreFile(f FileSnap) error {
	if !s.isPathSafe(f.Path) {
		return fmt.Errorf("restore: path %q escapes workspace root", f.Path)
	}
	if f.Content == nil {
		// File didn't exist at checkpoint time — delete it if present.
		if _, err := s.fs.Stat(f.Path); err == nil {
			return s.fs.Remove(f.Path)
		}
		return nil
	}
	return s.fs.WriteFile(f.Path, []byte(*f.Content))
}

// RestoreAndTrim restores to the given turn and removes all checkpoints after
// it. The restore and the trim run atomically with respect to concurrent
// BeginTurn by holding the store lock for the whole operation: previously
// Restore released the lock while doing file IO, so a concurrent BeginTurn
// could append a checkpoint that the subsequent trim would then drop.
func (s *Store) RestoreAndTrim(turn int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var target *TurnCheckpoint
	keep := make([]*TurnCheckpoint, 0, len(s.done))
	for _, c := range s.done {
		if c.Turn == turn {
			target = c
		}
		if c.Turn <= turn {
			keep = append(keep, c)
		}
	}
	if target == nil {
		return fmt.Errorf("no checkpoint for turn %d", turn)
	}

	// Restore files (restoreFile does not take s.mu, so holding the lock
	// here is safe; it briefly blocks other operations but guarantees
	// atomicity of restore+trim).
	for _, f := range target.Files {
		if err := s.restoreFile(f); err != nil {
			return fmt.Errorf("restore %s: %w", f.Path, err)
		}
	}
	s.done = keep
	return nil
}

// SortedPaths returns the distinct file paths touched in the current turn.
func (s *Store) CurrentTurnPaths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil {
		return nil
	}
	out := make([]string, 0, len(s.cur.Files))
	for _, f := range s.cur.Files {
		out = append(out, f.Path)
	}
	sort.Strings(out)
	return out
}

// isPathSafe reports whether path is allowed to be snapshotted or restored.
// When root is empty (test mode), all paths are allowed; otherwise the path
// must resolve to a location inside the workspace root, preventing a tool
// from escaping the sandbox via ".." or absolute paths outside the root.
func (s *Store) isPathSafe(path string) bool {
	if s.root == "" {
		return true
	}
	return isWithinRoot(path, s.root)
}

// isWithinRoot checks whether path is within the workspace root.
func isWithinRoot(path, root string) bool {
	if root == "" {
		return true
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil {
		return false
	}
	if rel == ".." || len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator) {
		return false
	}
	return true
}
