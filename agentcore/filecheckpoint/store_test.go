package filecheckpoint

import (
	"os"
	"testing"
	"time"
)

// memFS is an in-memory FileSystem for testing.
type memFS struct {
	files map[string][]byte
}

func newMemFS() *memFS {
	return &memFS{files: map[string][]byte{}}
}

func (m *memFS) ReadFile(path string) ([]byte, error) {
	if data, ok := m.files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}
func (m *memFS) Stat(path string) (os.FileInfo, error) {
	if _, ok := m.files[path]; ok {
		return mockInfo{}, nil
	}
	return nil, os.ErrNotExist
}
func (m *memFS) WriteFile(path string, content []byte) error {
	cp := make([]byte, len(content))
	copy(cp, content)
	m.files[path] = cp
	return nil
}
func (m *memFS) Remove(path string) error {
	delete(m.files, path)
	return nil
}

type mockInfo struct{}

func (mockInfo) Name() string       { return "" }
func (mockInfo) Size() int64        { return 0 }
func (mockInfo) Mode() os.FileMode  { return 0644 }
func (mockInfo) ModTime() time.Time { return time.Time{} }
func (mockInfo) IsDir() bool        { return false }
func (mockInfo) Sys() any           { return nil }

func TestStore_SnapshotAndRestore(t *testing.T) {
	fs := newMemFS()
	fs.files["/a.txt"] = []byte("original")

	s := New(fs, "")
	s.BeginTurn(1, "edit a.txt", 0)

	// Snapshot before modifying
	if err := s.SnapshotFile("/a.txt"); err != nil {
		t.Fatal(err)
	}

	// Simulate modification
	fs.files["/a.txt"] = []byte("modified")

	s.EndTurn()

	// List
	metas := s.List()
	if len(metas) != 1 {
		t.Fatalf("List()=%d items want 1", len(metas))
	}
	if metas[0].Turn != 1 {
		t.Errorf("Turn=%d want 1", metas[0].Turn)
	}

	// Restore
	if err := s.Restore(1); err != nil {
		t.Fatal(err)
	}
	if string(fs.files["/a.txt"]) != "original" {
		t.Errorf("after restore=%q want %q", fs.files["/a.txt"], "original")
	}
}

func TestStore_DedupSameTurn(t *testing.T) {
	fs := newMemFS()
	fs.files["/a.go"] = []byte("v1")

	s := New(fs, "")
	s.BeginTurn(1, "test", 0)

	// Snapshot twice
	_ = s.SnapshotFile("/a.go")
	fs.files["/a.go"] = []byte("v2")
	_ = s.SnapshotFile("/a.go") // should be no-op (dedup)
	fs.files["/a.go"] = []byte("v3")

	s.EndTurn()

	// Restore should give v1 (first snapshot)
	_ = s.Restore(1)
	if string(fs.files["/a.go"]) != "v1" {
		t.Errorf("restore=%q want %q", fs.files["/a.go"], "v1")
	}
}

func TestStore_NewFileDeleted(t *testing.T) {
	fs := newMemFS()

	s := New(fs, "")
	s.BeginTurn(1, "create new", 0)

	// Snapshot a file that doesn't exist yet
	_ = s.SnapshotFile("/new.txt")

	// Simulate file creation
	fs.files["/new.txt"] = []byte("created")

	s.EndTurn()

	// Restore should delete the file
	_ = s.Restore(1)
	if _, ok := fs.files["/new.txt"]; ok {
		t.Error("expected /new.txt to be deleted after restore")
	}
}

func TestStore_MultiTurnRestore(t *testing.T) {
	fs := newMemFS()
	fs.files["/f.go"] = []byte("v0")

	s := New(fs, "")

	// Turn 1: snapshot "v0", modify to "v1"
	s.BeginTurn(1, "turn1", 0)
	_ = s.SnapshotFile("/f.go")
	fs.files["/f.go"] = []byte("v1")
	s.EndTurn()

	// Turn 2: snapshot "v1", modify to "v2"
	s.BeginTurn(2, "turn2", 0)
	_ = s.SnapshotFile("/f.go")
	fs.files["/f.go"] = []byte("v2")
	s.EndTurn()

	// Restore(2) undoes turn 2 → reverts to pre-turn-2 state = "v1"
	_ = s.Restore(2)
	if string(fs.files["/f.go"]) != "v1" {
		t.Errorf("restore turn 2: got %q want %q", fs.files["/f.go"], "v1")
	}

	// Restore(1) undoes turn 1 → reverts to pre-turn-1 state = "v0"
	_ = s.Restore(1)
	if string(fs.files["/f.go"]) != "v0" {
		t.Errorf("restore turn 1: got %q want %q", fs.files["/f.go"], "v0")
	}
}

func TestStore_RestoreNonexistent(t *testing.T) {
	fs := newMemFS()
	s := New(fs, "")
	s.BeginTurn(1, "x", 0)
	s.EndTurn()

	err := s.Restore(99)
	if err == nil {
		t.Error("expected error for nonexistent turn")
	}
}

func TestStore_RestoreAndTrim(t *testing.T) {
	fs := newMemFS()
	fs.files["/f.go"] = []byte("v0")

	s := New(fs, "")

	s.BeginTurn(1, "t1", 0)
	_ = s.SnapshotFile("/f.go")
	fs.files["/f.go"] = []byte("v1")
	s.EndTurn()

	s.BeginTurn(2, "t2", 0)
	_ = s.SnapshotFile("/f.go")
	fs.files["/f.go"] = []byte("v2")
	s.EndTurn()

	_ = s.RestoreAndTrim(1)
	if len(s.List()) != 1 {
		t.Errorf("List()=%d want 1 after trim", len(s.List()))
	}
}

func TestIsWithinRoot(t *testing.T) {
	tests := []struct {
		path, root string
		want       bool
	}{
		{"/ws/a.go", "/ws", true},
		{"/ws/sub/b.go", "/ws", true},
		{"/etc/passwd", "/ws", false},
		{"/ws", "/ws", true},
		{"", "", true},
	}
	for _, tt := range tests {
		got := isWithinRoot(tt.path, tt.root)
		if got != tt.want {
			t.Errorf("isWithinRoot(%q,%q)=%v want %v", tt.path, tt.root, got, tt.want)
		}
	}
}
