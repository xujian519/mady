// Package csync provides a generic thread-safe Slice primitive for concurrent
// read/write access within the TUI module.
//
// This is a minimal subset of the top-level mady/pkg/csync package, copied
// here to avoid a circular dependency when the TUI module is published as
// an independent Go sub-module.
package csync

import "sync"

// Slice is a thread-safe slice with concurrent read/write access.
type Slice[T any] struct {
	inner []T
	mu    sync.RWMutex
}

// NewSlice creates an empty thread-safe slice.
func NewSlice[T any]() *Slice[T] {
	return &Slice[T]{inner: make([]T, 0)}
}

// NewSliceFrom creates a thread-safe slice initialized from an existing slice.
func NewSliceFrom[T any](s []T) *Slice[T] {
	inner := make([]T, len(s))
	copy(inner, s)
	return &Slice[T]{inner: inner}
}

// Append adds elements to the end of the slice.
func (s *Slice[T]) Append(items ...T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inner = append(s.inner, items...)
}

// SetSlice replaces the entire slice with a copy of the given slice.
func (s *Slice[T]) SetSlice(items []T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inner = make([]T, len(items))
	copy(s.inner, items)
}

// Copy returns a copy of the underlying slice.
func (s *Slice[T]) Copy() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]T, len(s.inner))
	copy(items, s.inner)
	return items
}
