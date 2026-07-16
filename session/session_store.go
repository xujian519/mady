package session

import (
	"bufio"
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/util"
)

// ---------------------------------------------------------------------------
// Persistence (lazy flush)
// ---------------------------------------------------------------------------

func (m *Manager) persistEntry(entry Entry) error {
	if !m.persist || m.filePath == "" {
		return nil
	}

	// Lazy full flush on first write — writes header and all buffered entries.
	// Subsequent appends write only the new entry in append mode.
	if !m.flushed {
		return m.flushAllLocked()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	f, err := os.OpenFile(m.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock session file: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write entry: %w", err)
	}
	return nil
}

// flushAll writes the complete session (header + all entries) to disk.
// It acquires m.mu to protect against concurrent Append operations.
// Callers that already hold m.mu should use flushAllLocked instead.
func (m *Manager) flushAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.flushAllLocked()
}

// flushAllLocked writes the complete session to disk without acquiring m.mu.
// The caller must hold m.mu (read or write lock).
func (m *Manager) flushAllLocked() error {
	f, err := os.OpenFile(m.filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create session file: %w", err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock session file: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	headerData, err := json.Marshal(m.header)
	if err != nil {
		return fmt.Errorf("marshal header: %w", err)
	}
	if _, err := f.Write(append(headerData, '\n')); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, e := range m.entries {
		data, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal entry: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("write entry: %w", err)
		}
	}
	m.flushed = true
	return nil
}

// ---------------------------------------------------------------------------
// Version migration
// ---------------------------------------------------------------------------

func migrateEntries(entries []Entry, fromVersion int64) bool {
	changed := false
	if fromVersion < 2 {
		migrateV1ToV2(entries)
		changed = true
	}
	if fromVersion < 3 {
		migrateV2ToV3(entries)
		changed = true
	}
	if fromVersion < 4 {
		migrateV3ToV4(entries)
		changed = true
	}
	return changed
}

func migrateV1ToV2(entries []Entry) {
	var lastID string
	for i := range entries {
		if entries[i].ID == "" {
			entries[i].ID = generateID()
		}
		if entries[i].ParentID == "" && lastID != "" {
			entries[i].ParentID = lastID
		}
		lastID = entries[i].ID
		entries[i].Version = 2
	}
}

func migrateV2ToV3(entries []Entry) {
	for i := range entries {
		if entries[i].Type == EntryCompaction {
			var raw map[string]any
			if json.Unmarshal(entries[i].Data, &raw) == nil {
				if _, ok := raw["firstKeptEntryIndex"]; ok {
					delete(raw, "firstKeptEntryIndex")
					updated, _ := json.Marshal(raw)
					entries[i].Data = updated
				}
			}
		}
		entries[i].Version = 3
	}
}

func migrateV3ToV4(entries []Entry) {
	for i := range entries {
		if entries[i].Type == EntryMessage {
			var msg map[string]any
			if json.Unmarshal(entries[i].Data, &msg) == nil {
				if role, ok := msg["role"].(string); ok && role == "hookMessage" {
					msg["role"] = "custom"
					updated, _ := json.Marshal(msg)
					entries[i].Data = updated
				}
			}
		}
		entries[i].Version = 4
	}
}

// ---------------------------------------------------------------------------
// FileStore — one JSONL file per session
// ---------------------------------------------------------------------------

// lockEntry pairs a session ID with its RWMutex for the LRU list.
type lockEntry struct {
	id string
	mu *sync.RWMutex
}

type FileStore struct {
	dir      string
	locks    map[string]*list.Element // id → LRU list element
	lockList *list.List               // LRU: Front=MRU, Back=LRU
	locksMu  sync.Mutex               // TODO(csync): csync.Map now has Range/ForEach. Migration feasible — replace locks (map) with csync.Map while keeping locksMu for lockList atomicity.

	// maxLocks caps the per-session lock cache. When the cache exceeds this
	// limit the oldest entries are evicted (LRU). This prevents unbounded
	// memory growth from long-running processes that create many sessions
	// over time. The default (0) means unlimited.
	maxLocks int
}

// FileStoreOption configures a FileStore.
type FileStoreOption func(*FileStore)

// WithMaxLocks sets the maximum number of per-session locks cached by the
// FileStore. When the cache exceeds this size it is pruned. Set to 0 for
// unlimited (the default).
func WithMaxLocks(n int) FileStoreOption {
	return func(fs *FileStore) { fs.maxLocks = n }
}

func NewFileStore(dir string, opts ...FileStoreOption) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create session directory: %w", err)
	}
	fs := &FileStore{
		dir:      dir,
		locks:    make(map[string]*list.Element),
		lockList: list.New(),
	}
	for _, opt := range opts {
		opt(fs)
	}
	return fs, nil
}

func (s *FileStore) sessionLock(id string) *sync.RWMutex {
	s.locksMu.Lock()
	defer s.locksMu.Unlock()

	if elem, ok := s.locks[id]; ok {
		s.lockList.MoveToFront(elem) // O(1) LRU touch
		return elem.Value.(*lockEntry).mu
	}

	if s.maxLocks > 0 && len(s.locks) >= s.maxLocks {
		evictCount := len(s.locks) / 10
		if evictCount < 1 {
			evictCount = 1
		}
		for i := 0; i < evictCount; i++ {
			oldest := s.lockList.Back()
			if oldest == nil {
				break
			}
			entry := s.lockList.Remove(oldest).(*lockEntry)
			delete(s.locks, entry.id)
		}
	}

	entry := &lockEntry{id: id, mu: &sync.RWMutex{}}
	s.locks[id] = s.lockList.PushFront(entry)
	return entry.mu
}

func (s *FileStore) path(sessionID string) string {
	return filepath.Join(s.dir, sessionID+".jsonl")
}

func (s *FileStore) Create(_ context.Context, opts CreateOptions) (*Manager, error) {
	if opts.ID == "" {
		opts.ID = generateID()
	} else if err := util.ValidateKey(opts.ID); err != nil {
		return nil, err
	}

	header := Header{
		Type:          EntryHeader,
		Version:       CurrentVersion,
		ID:            opts.ID,
		Timestamp:     time.Now().Format(time.RFC3339),
		Cwd:           opts.Cwd,
		ParentSession: opts.ParentSession,
	}

	persist := !opts.InMemory
	filePath := ""
	if persist {
		filePath = s.path(opts.ID)
	}

	mgr := newManager(header, filePath, persist)

	if persist {
		headerData, _ := json.Marshal(header)
		if err := os.WriteFile(filePath, append(headerData, '\n'), 0o600); err != nil {
			return nil, fmt.Errorf("write session header: %w", err)
		}
		mgr.flushed = true
	}

	return mgr, nil
}

func (s *FileStore) Open(_ context.Context, sessionID string) (*Manager, error) {
	if err := util.ValidateKey(sessionID); err != nil {
		return nil, err
	}
	lock := s.sessionLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	f, err := os.Open(s.path(sessionID))
	if err != nil {
		return nil, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	var header Header
	var entries []Entry
	headerParsed := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if !headerParsed {
			var raw map[string]any
			if err := json.Unmarshal([]byte(line), &raw); err == nil {
				if t, ok := raw["type"].(string); ok && t == string(EntryHeader) {
					if err := json.Unmarshal([]byte(line), &header); err != nil {
						return nil, fmt.Errorf("parse session header: %w", err)
					}
					headerParsed = true
					continue
				}
			}
			header = Header{
				Type:    EntryHeader,
				Version: 1,
				ID:      sessionID,
			}
			headerParsed = true
		}

		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan session file: %w", err)
	}

	if !headerParsed {
		return nil, fmt.Errorf("session %q: empty or invalid file", sessionID)
	}

	fromVersion := header.Version
	if fromVersion == 0 {
		fromVersion = 1
	}

	needsRewrite := false
	if fromVersion < CurrentVersion {
		needsRewrite = migrateEntries(entries, fromVersion)
		header.Version = CurrentVersion
	}

	mgr := newManager(header, s.path(sessionID), true)
	mgr.entries = entries
	mgr.flushed = true

	for i := range mgr.entries {
		e := &mgr.entries[i]
		mgr.index[e.ID] = e
	}

	for _, e := range entries {
		if e.Type == EntryLabel {
			var ld LabelData
			if json.Unmarshal(e.Data, &ld) == nil {
				if ld.Label == "" {
					delete(mgr.labelsByID, ld.TargetID)
				} else {
					mgr.labelsByID[ld.TargetID] = ld.Label
				}
			}
		}
		if !mgr.hasAssistant && e.Type == EntryMessage {
			var msg agentcore.Message
			if json.Unmarshal(e.Data, &msg) == nil && msg.Role == agentcore.RoleAssistant {
				mgr.hasAssistant = true
			}
		}
	}

	if len(entries) > 0 {
		mgr.leafID = entries[len(entries)-1].ID
	}

	if needsRewrite {
		if err := mgr.flushAll(); err != nil {
			return nil, err
		}
	}

	return mgr, nil
}

func (s *FileStore) List(_ context.Context) ([]Info, error) {
	dirEntries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("list session directory: %w", err)
	}

	var sessions []Info
	for _, de := range dirEntries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".jsonl") {
			continue
		}
		name := strings.TrimSuffix(de.Name(), ".jsonl")
		info := s.readInfo(name)
		sessions = append(sessions, info)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

func (s *FileStore) Delete(_ context.Context, sessionID string) error {
	if err := util.ValidateKey(sessionID); err != nil {
		return err
	}
	lock := s.sessionLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	if err := os.Remove(s.path(sessionID)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete session: %w", err)
	}

	s.locksMu.Lock()
	delete(s.locks, sessionID)
	s.locksMu.Unlock()

	return nil
}

// lockCleanup removes the per-session lock cache entry without touching the
// underlying file. Useful when a session file has already been replaced
// (e.g. during rewriteSession) and only the stale lock needs to be purged.
func (s *FileStore) lockCleanup(sessionID string) {
	s.locksMu.Lock()
	if elem, ok := s.locks[sessionID]; ok {
		s.lockList.Remove(elem)
		delete(s.locks, sessionID)
	}
	s.locksMu.Unlock()
}

func (s *FileStore) Has(_ context.Context, sessionID string) (bool, error) {
	if err := util.ValidateKey(sessionID); err != nil {
		return false, err
	}
	_, err := os.Stat(s.path(sessionID))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("check session: %w", err)
}

func (s *FileStore) readInfo(sessionID string) Info {
	lock := s.sessionLock(sessionID)
	lock.RLock()
	defer lock.RUnlock()

	info := Info{ID: sessionID}

	f, err := os.Open(s.path(sessionID))
	if err != nil {
		return info
	}
	defer f.Close()

	fi, err := f.Stat()
	if err == nil {
		info.UpdatedAt = fi.ModTime()
		info.CreatedAt = fi.ModTime()
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	lineNum := int64(0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineNum++

		if lineNum == 1 {
			var header Header
			if json.Unmarshal([]byte(line), &header) == nil && header.Type == EntryHeader {
				info.ParentSession = header.ParentSession
				info.Cwd = header.Cwd
				info.Version = header.Version
				if t, err := time.Parse(time.RFC3339, header.Timestamp); err == nil {
					info.CreatedAt = t
				}
				continue
			}
		}

		var entry Entry
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}

		if entry.Type == EntryMessage {
			info.MessageCount++
		}
		if entry.Type == EntrySessionInfo {
			var meta map[string]string
			if json.Unmarshal(entry.Data, &meta) == nil {
				if v, ok := meta["name"]; ok {
					info.Name = v
				}
			}
		}
		if entry.Type == EntryLabel {
			var ld LabelData
			if json.Unmarshal(entry.Data, &ld) == nil && ld.Label != "" {
				info.Label = ld.Label
			}
		}
	}

	return info
}

// MessagesFromEntries rebuilds a Message slice from entries (flat, no tree awareness).
func MessagesFromEntries(entries []Entry) []agentcore.Message {
	var msgs []agentcore.Message
	for _, e := range entries {
		if e.Type != EntryMessage {
			continue
		}
		var msg agentcore.Message
		if json.Unmarshal(e.Data, &msg) == nil {
			msgs = append(msgs, msg)
		}
	}
	return msgs
}

// ---------------------------------------------------------------------------
// ID generator
// ---------------------------------------------------------------------------

func (m *Manager) generateID() string {
	counter := m.idCounter.Add(1)
	return fmt.Sprintf("%d_%d", time.Now().UnixNano(), counter)
}

func generateID() string {
	return fmt.Sprintf("%d_%d", time.Now().UnixNano(), 1)
}
