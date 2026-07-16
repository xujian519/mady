package session

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/agentcore"
)

const CurrentVersion int64 = 4

// ---------------------------------------------------------------------------
// Entry types — mirrors the session-manager.ts entry taxonomy
// ---------------------------------------------------------------------------

type EntryType string

const (
	EntryHeader        EntryType = "session"
	EntryMessage       EntryType = "message"
	EntrySessionInfo   EntryType = "session_info"
	EntryModelChange   EntryType = "model_change"
	EntryCompaction    EntryType = "compaction"
	EntryBranchSummary EntryType = "branch_summary"
	EntryLabel         EntryType = "label"
	EntryCustom        EntryType = "custom"
	EntryCustomMessage EntryType = "custom_message"
)

// ---------------------------------------------------------------------------
// Core types
// ---------------------------------------------------------------------------

// Header is always the first entry in a JSONL session file.
type Header struct {
	Type          EntryType `json:"type"`
	Version       int64     `json:"version"`
	ID            string    `json:"id"`
	Timestamp     string    `json:"timestamp"`
	Cwd           string    `json:"cwd,omitempty"`
	ParentSession string    `json:"parent_session,omitempty"`
}

// Entry is one line in the JSONL file. Forms a tree via ID/ParentID.
type Entry struct {
	ID        string          `json:"id"`
	ParentID  string          `json:"parent_id,omitempty"`
	Type      EntryType       `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp time.Time       `json:"timestamp"`
	Version   int64           `json:"version,omitempty"`
}

// LabelData is the payload of an EntryLabel.
type LabelData struct {
	TargetID string `json:"target_id"`
	Label    string `json:"label,omitempty"`
}

// CompactionData is the payload of an EntryCompaction.
type CompactionData struct {
	Summary          string `json:"summary"`
	FirstKeptEntryID string `json:"first_kept_entry_id"`
	KeptCount        int64  `json:"kept_count"`
}

// BranchSummaryData is the payload of an EntryBranchSummary.
type BranchSummaryData struct {
	Summary  string `json:"summary"`
	BranchID string `json:"branch_id"`
}

// Info is the public metadata of a session for listing.
type Info struct {
	ID            string    `json:"id"`
	Name          string    `json:"name,omitempty"`
	Label         string    `json:"label,omitempty"`
	Summary       string    `json:"summary,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	ParentSession string    `json:"parent_session,omitempty"`
	Cwd           string    `json:"cwd,omitempty"`
	MessageCount  int64     `json:"message_count"`
	Version       int64     `json:"version"`
}

// ---------------------------------------------------------------------------
// Store interface
// ---------------------------------------------------------------------------

type Store interface {
	Create(ctx context.Context, opts CreateOptions) (*Manager, error)
	Open(ctx context.Context, sessionID string) (*Manager, error)
	List(ctx context.Context) ([]Info, error)
	Delete(ctx context.Context, sessionID string) error
}

// Compile-time check: FileStore satisfies Store.
var _ Store = (*FileStore)(nil)

type CreateOptions struct {
	ID            string
	Cwd           string
	ParentSession string
	InMemory      bool
}

// ---------------------------------------------------------------------------
// Manager — the heart: manages one session's lifecycle (append-only tree)
// ---------------------------------------------------------------------------

type Manager struct {
	header  Header
	entries []Entry
	leafID  string

	filePath string
	persist  bool
	flushed  bool

	labelsByID map[string]string
	index      map[string]*Entry

	hasAssistant bool
	pathCache    atomic.Pointer[pathCache]

	mu        sync.RWMutex
	idCounter atomic.Int64
}

func newManager(header Header, filePath string, persist bool) *Manager {
	return &Manager{
		header:     header,
		filePath:   filePath,
		persist:    persist,
		labelsByID: make(map[string]string),
		index:      make(map[string]*Entry),
	}
}

// Header returns the session header.
func (m *Manager) Header() Header {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.header
}

// LeafID returns the current leaf (cursor) in the tree.
func (m *Manager) LeafID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.leafID
}

// Entries returns a copy of all entries.
func (m *Manager) Entries() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make([]Entry, len(m.entries))
	copy(cp, m.entries)
	return cp
}

// Append adds a new entry as a child of the current leaf.
func (m *Manager) Append(_ context.Context, entry Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry.ID == "" {
		entry.ID = m.generateID()
	}
	if entry.ParentID == "" {
		entry.ParentID = m.leafID
	}
	if entry.Version == 0 {
		entry.Version = CurrentVersion
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	m.entries = append(m.entries, entry)
	m.index[entry.ID] = &m.entries[len(m.entries)-1]
	// Save pre-mutation state for rollback on persistence failure.
	prevLeafID := m.leafID
	prevHasAssistant := m.hasAssistant
	var prevLabelTargetID, prevLabelValue string
	var prevLabelHad bool
	m.leafID = entry.ID
	m.invalidatePathCache()

	if !m.hasAssistant && entry.Type == EntryMessage {
		var msg agentcore.Message
		if json.Unmarshal(entry.Data, &msg) == nil && msg.Role == agentcore.RoleAssistant {
			m.hasAssistant = true
		}
	}

	if entry.Type == EntryLabel {
		var ld LabelData
		if err := json.Unmarshal(entry.Data, &ld); err == nil {
			prevLabelHad = true
			prevLabelTargetID = ld.TargetID
			prevLabelValue = m.labelsByID[ld.TargetID]
			if ld.Label == "" {
				delete(m.labelsByID, ld.TargetID)
			} else {
				m.labelsByID[ld.TargetID] = ld.Label
			}
		}
	}

	if err := m.persistEntry(entry); err != nil {
		// Rollback: persistence failed, restore memory state.
		m.entries = m.entries[:len(m.entries)-1]
		delete(m.index, entry.ID)
		m.leafID = prevLeafID
		m.hasAssistant = prevHasAssistant
		if prevLabelHad {
			if prevLabelValue == "" {
				delete(m.labelsByID, prevLabelTargetID)
			} else {
				m.labelsByID[prevLabelTargetID] = prevLabelValue
			}
		}
		m.invalidatePathCache()
		return err
	}
	return nil
}

// AppendMessage is a convenience for adding a message entry.
func (m *Manager) AppendMessage(ctx context.Context, msg agentcore.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	return m.Append(ctx, Entry{Type: EntryMessage, Data: data})
}

// AppendCompaction records a compaction event.
func (m *Manager) AppendCompaction(ctx context.Context, cd CompactionData) error {
	data, err := json.Marshal(cd)
	if err != nil {
		return fmt.Errorf("marshal compaction: %w", err)
	}
	return m.Append(ctx, Entry{Type: EntryCompaction, Data: data})
}

// AppendBranchSummary records a branch summary.
func (m *Manager) AppendBranchSummary(ctx context.Context, bs BranchSummaryData) error {
	data, err := json.Marshal(bs)
	if err != nil {
		return fmt.Errorf("marshal branch summary: %w", err)
	}
	return m.Append(ctx, Entry{Type: EntryBranchSummary, Data: data})
}

// SetLabel adds or removes a label on an entry.
func (m *Manager) SetLabel(ctx context.Context, targetID, label string) error {
	data, err := json.Marshal(LabelData{TargetID: targetID, Label: label})
	if err != nil {
		return fmt.Errorf("marshal label: %w", err)
	}
	return m.Append(ctx, Entry{Type: EntryLabel, Data: data})
}

// SetSessionName writes a session_info entry with a display name.
func (m *Manager) SetSessionName(ctx context.Context, name string) error {
	data, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return fmt.Errorf("marshal session name: %w", err)
	}
	return m.Append(ctx, Entry{Type: EntrySessionInfo, Data: data})
}

// Branch moves the leaf pointer to an earlier entry without deleting history.
func (m *Manager) Branch(branchFromID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.index[branchFromID]; !ok {
		return fmt.Errorf("session: entry %q not found", branchFromID)
	}
	m.leafID = branchFromID
	return nil
}

// GetLabel returns the label for an entry, if any.
func (m *Manager) GetLabel(entryID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.labelsByID[entryID]
}

// MessagesOnPath extracts Messages along the root→leaf path.
func (m *Manager) MessagesOnPath() []agentcore.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	path := m.pathToLeaf()
	var msgs []agentcore.Message
	var lastCompaction *CompactionData

	for _, entry := range path {
		if entry.Type == EntryCompaction {
			var cd CompactionData
			if json.Unmarshal(entry.Data, &cd) == nil {
				lastCompaction = &cd
			}
		}
	}

	skipUntil := ""
	if lastCompaction != nil {
		skipUntil = lastCompaction.FirstKeptEntryID
		msgs = append(msgs, agentcore.Message{
			Role:    agentcore.RoleSystem,
			Content: lastCompaction.Summary,
			Type:    agentcore.MessageTypeCompactionSummary,
		})
	}

	skipping := skipUntil != ""
	for _, entry := range path {
		if skipping {
			if entry.ID == skipUntil {
				skipping = false
			} else {
				continue
			}
		}

		switch entry.Type {
		case EntryMessage:
			var msg agentcore.Message
			if json.Unmarshal(entry.Data, &msg) == nil {
				msgs = append(msgs, msg)
			}
		case EntryBranchSummary:
			var bs BranchSummaryData
			if json.Unmarshal(entry.Data, &bs) == nil {
				msgs = append(msgs, agentcore.Message{
					Role:    agentcore.RoleSystem,
					Content: bs.Summary,
					Type:    agentcore.MessageTypeBranchSummary,
				})
			}
		}
	}

	return msgs
}

// Info returns the public metadata for this session.
func (m *Manager) Info() Info {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info := Info{
		ID:            m.header.ID,
		ParentSession: m.header.ParentSession,
		Cwd:           m.header.Cwd,
		Version:       m.header.Version,
	}

	if len(m.entries) > 0 {
		info.CreatedAt = m.entries[0].Timestamp
		info.UpdatedAt = m.entries[len(m.entries)-1].Timestamp
	}

	for i := len(m.entries) - 1; i >= 0; i-- {
		e := m.entries[i]
		if e.Type == EntrySessionInfo {
			var meta map[string]string
			if json.Unmarshal(e.Data, &meta) == nil {
				if v, ok := meta["name"]; ok && info.Name == "" {
					info.Name = v
				}
			}
		}
	}

	for _, e := range m.entries {
		if e.Type == EntryMessage {
			info.MessageCount++
		}
	}

	return info
}

// CreateBranchedSession extracts root→leafID path into a new JSONL file.
// Returns the new session's file path.
func (m *Manager) CreateBranchedSession(ctx context.Context, store *FileStore) (string, error) {
	m.mu.RLock()
	path := m.pathToLeaf()
	m.mu.RUnlock()

	newID := m.generateID()
	newMgr, err := store.Create(ctx, CreateOptions{
		ID:            newID,
		Cwd:           m.header.Cwd,
		ParentSession: m.header.ID,
	})
	if err != nil {
		return "", fmt.Errorf("create branched session: %w", err)
	}

	for _, entry := range path {
		if entry.Type == EntryLabel {
			continue
		}
		entry.Version = CurrentVersion
		if err := newMgr.Append(ctx, entry); err != nil {
			return "", fmt.Errorf("copy entry to branch: %w", err)
		}
	}

	m.mu.RLock()
	for _, entry := range path {
		if label, ok := m.labelsByID[entry.ID]; ok {
			if err := newMgr.SetLabel(ctx, entry.ID, label); err != nil {
				m.mu.RUnlock()
				return "", fmt.Errorf("copy label to branch: %w", err)
			}
		}
	}
	m.mu.RUnlock()

	return newID, nil
}

// pathCache caches pre-built maps from entries to avoid rebuilding them
// on every pathToLeaf call (which can be O(N) per access for large sessions).
// Uses atomic.Pointer to allow safe concurrent reads without a write lock.
type pathCache struct {
	parentMap map[string]string
	entryMap  map[string]Entry
}

// invalidatePathCache marks the cached maps as stale. Called after Append.
func (m *Manager) invalidatePathCache() {
	m.pathCache.Store(nil)
}

// buildPathCache constructs the parent and entry maps from the current
// entries. Uses atomic load and compare-and-swap to avoid data races
// under concurrent readers.
func (m *Manager) buildPathCache() {
	if pc := m.pathCache.Load(); pc != nil {
		return
	}
	parentMap := make(map[string]string, len(m.entries))
	entryMap := make(map[string]Entry, len(m.entries))
	for _, e := range m.entries {
		parentMap[e.ID] = e.ParentID
		entryMap[e.ID] = e
	}
	m.pathCache.CompareAndSwap(nil, &pathCache{parentMap: parentMap, entryMap: entryMap})
}

// pathToLeaf returns the ordered list of entries from root to current leaf.
// Uses a cached map to avoid rebuilding from scratch on every call.
func (m *Manager) pathToLeaf() []Entry {
	if m.leafID == "" {
		return nil
	}

	m.buildPathCache()
	pc := m.pathCache.Load()
	parentMap := pc.parentMap
	entryMap := pc.entryMap

	var chain []Entry
	visited := make(map[string]bool)
	current := m.leafID
	for current != "" {
		if visited[current] {
			break
		}
		visited[current] = true
		if e, ok := entryMap[current]; ok {
			chain = append(chain, e)
		}
		current = parentMap[current]
	}

	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}
