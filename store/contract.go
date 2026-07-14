// Package store provides common storage interfaces and implementations
// for the Mady platform. It includes:
//
//   - SnapshotStore: file-based agent state persistence (agentcore.Store)
//   - CaseStore: unified interface for case-scoped storage (checkpoint/memory/approval)
//   - Closer: resource cleanup interface for stores holding connections
package store

import "context"

// CaseStore is a common interface for stores that persist case-level data.
// It enables uniform case-scoped queries, schema migration tracking, and
// version-based evolution across checkpoint, memory, and approval stores.
//
// Any SQLite-backed store that holds case-scoped data should implement this
// interface so that callers can handle all three stores uniformly for
// operations like case restore, case delete, and version checking.
type CaseStore interface {
	// CaseID returns the case this store is scoped to, or "" if global/unscoped.
	CaseID() string

	// RunID returns the current run/session identifier, or "" if not scoped.
	RunID() string

	// Migrate runs schema migrations to bring the store up to date.
	// Returns the schema version after migration.
	Migrate(ctx context.Context) (int, error)

	// Version returns the current schema version of the store.
	// Implementations return 0 if no version tracking is in place.
	Version() int
}

// Closer is implemented by stores that hold resources requiring cleanup
// (e.g., database connections, open file handles).
type Closer interface {
	// Close releases any held resources. After Close returns, the store
	// should not be used for further operations.
	Close() error
}
