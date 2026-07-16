// Package csync provides generic thread-safe concurrent primitives.
//
// Value wraps a single value with a read-write mutex. Slice wraps a slice
// with a read-write mutex, supporting concurrent append and iteration.
// Map wraps a map with a read-write mutex, supporting concurrent get/set/delete.
//
// Use these instead of raw sync.RWMutex + value pairs to reduce boilerplate
// and prevent accidental unprotected access.
package csync

import (
	"encoding/json"
	"maps"
	"reflect"
	"sync"
)

// Value is a generic thread-safe wrapper for a single value, guarded by
// a sync.RWMutex. It panics when constructed with pointer, slice, or map
// types — use Slice or Map for those.
type Value[T any] struct {
	v  T
	mu sync.RWMutex
}

// NewValue creates a Value initialized to the given value.
// Panics if T is a pointer, slice, or map type.
func NewValue[T any](t T) *Value[T] {
	v := reflect.ValueOf(t)
	switch v.Kind() {
	case reflect.Pointer:
		panic("csync.Value does not support pointer types")
	case reflect.Slice:
		panic("csync.Value does not support slice types; use csync.Slice")
	case reflect.Map:
		panic("csync.Value does not support map types; use csync.Map")
	}
	return &Value[T]{v: t}
}

// Get returns a copy of the current value.
func (v *Value[T]) Get() T {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.v
}

// Set replaces the current value.
func (v *Value[T]) Set(t T) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.v = t
}

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

// Get returns the element at the given index. Returns the zero value and
// false if the index is out of bounds.
func (s *Slice[T]) Get(index int) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var zero T
	if index < 0 || index >= len(s.inner) {
		return zero, false
	}
	return s.inner[index], true
}

// Len returns the number of elements in the slice.
func (s *Slice[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.inner)
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

// Map is a thread-safe map with concurrent read/write access.
type Map[K comparable, V any] struct {
	inner map[K]V
	mu    sync.RWMutex
}

// NewMap creates an empty thread-safe map.
func NewMap[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{inner: make(map[K]V)}
}

// NewMapFrom creates a thread-safe map initialized from an existing map.
// The caller must not retain and modify the original map.
func NewMapFrom[K comparable, V any](m map[K]V) *Map[K, V] {
	return &Map[K, V]{inner: m}
}

// Set sets the value for a key. Initializes the underlying map if nil
// (supports zero-value Map).
func (m *Map[K, V]) Set(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inner == nil {
		m.inner = make(map[K]V)
	}
	m.inner[key] = value
}

// Get returns the value for a key. Returns the zero value and false
// if the key does not exist. Safe on nil map (zero-value Map).
func (m *Map[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.inner == nil {
		var zero V
		return zero, false
	}
	v, ok := m.inner[key]
	return v, ok
}

// Del removes a key from the map. Idempotent. Safe on nil map.
func (m *Map[K, V]) Del(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inner == nil {
		return
	}
	delete(m.inner, key)
}

// Len returns the number of items in the map. Safe on nil map.
func (m *Map[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.inner == nil {
		return 0
	}
	return len(m.inner)
}

// GetOrSet returns the existing value for the key, or calls fn to create
// and store a new value if the key is not present. Safe on nil map.
func (m *Map[K, V]) GetOrSet(key K, fn func() V) V {
	if v, ok := m.Get(key); ok {
		return v
	}
	value := fn()
	m.Set(key, value)
	return value
}

// Take gets a value and then deletes it from the map. Returns zero value
// and false if the key doesn't exist or the map is nil.
func (m *Map[K, V]) Take(key K) (V, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inner == nil {
		var zero V
		return zero, false
	}
	v, ok := m.inner[key]
	delete(m.inner, key)
	return v, ok
}

// Reset replaces the underlying map with the given one.
// The caller must not retain and modify the original map.
func (m *Map[K, V]) Reset(input map[K]V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inner = input
}

// Range calls f sequentially for each key and value in the map. If f
// returns false, iteration stops. The map is locked for reading during
// iteration. Safe on nil map (no iteration).
func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.inner == nil {
		return
	}
	for k, v := range m.inner {
		if !f(k, v) {
			return
		}
	}
}

// ForEach calls f for each key and value in the map. Convenience wrapper
// around Range that always iterates all entries.
func (m *Map[K, V]) ForEach(f func(key K, value V)) {
	m.Range(func(k K, v V) bool { f(k, v); return true })
}

// Copy returns a shallow copy of the underlying map. Returns nil for
// an uninitialized (nil) map.
func (m *Map[K, V]) Copy() map[K]V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.inner == nil {
		return nil
	}
	return maps.Clone(m.inner)
}

// Ensure json.Marshaler / json.Unmarshaler satisfaction.
var (
	_ json.Unmarshaler = (*Map[string, any])(nil)
	_ json.Marshaler   = (*Map[string, any])(nil)
)

// MarshalJSON implements json.Marshaler.
func (m *Map[K, V]) MarshalJSON() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return json.Marshal(m.inner)
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *Map[K, V]) UnmarshalJSON(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inner = make(map[K]V)
	return json.Unmarshal(data, &m.inner)
}
