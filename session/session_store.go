package session

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"
)

const sessionFileExt = ".jsonl"

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
	f, err := os.OpenFile(m.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open session file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock session file: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

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
	f, err := os.OpenFile(m.filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create session file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock session file: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

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
// ID generator
// ---------------------------------------------------------------------------

func (m *Manager) generateID() string {
	counter := m.idCounter.Add(1)
	return fmt.Sprintf("%d_%d", time.Now().UnixNano(), counter)
}

func generateID() string {
	return fmt.Sprintf("%d_%d", time.Now().UnixNano(), 1)
}
