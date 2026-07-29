package worker

import (
	"fmt"
	"sync"
)

// Registry manages Worker runtime registration. It supports both pre-registration
// (startup) and lazy registration (on first use).
type Registry struct {
	mu        sync.RWMutex
	entries   map[string]*Definition
	activated map[string]bool // tracks which Workers have been activated
}

// NewRegistry creates an empty Worker registry.
func NewRegistry() *Registry {
	return &Registry{
		entries:   make(map[string]*Definition),
		activated: make(map[string]bool),
	}
}

// Register adds a Worker definition. Returns error if name is empty or duplicate.
func (r *Registry) Register(d Definition) error {
	if d.Name == "" {
		return fmt.Errorf("worker: cannot register with empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[d.Name]; exists {
		return fmt.Errorf("worker: %q already registered", d.Name)
	}
	r.entries[d.Name] = &d
	r.activated[d.Name] = true // pre-registered = activated
	return nil
}

// RegisterOrUpdate adds or replaces a Worker definition.
func (r *Registry) RegisterOrUpdate(d Definition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[d.Name] = &d
	r.activated[d.Name] = true
}

// LazyActivate registers a Worker on first use. Returns true if newly activated.
func (r *Registry) LazyActivate(d Definition) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[d.Name]; exists {
		return false // already registered
	}
	r.entries[d.Name] = &d
	r.activated[d.Name] = true
	return true
}

// Get returns a Worker definition by name, or nil.
func (r *Registry) Get(name string) *Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.entries[name]
}

// List returns all registered Worker definitions.
func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.entries))
	for _, d := range r.entries {
		out = append(out, *d)
	}
	return out
}

// ListByTier returns Workers matching the given tier.
func (r *Registry) ListByTier(tier WorkerTier) []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Definition
	for _, d := range r.entries {
		if d.Tier == tier {
			out = append(out, *d)
		}
	}
	return out
}

// IsActivated returns true if the Worker has been activated.
func (r *Registry) IsActivated(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.activated[name]
}

// Count returns the number of registered Workers.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}

// VerifyAll checks that all Workers in the catalog are registered in this registry.
// Returns missing Worker names.
func (r *Registry) VerifyAll(expected *Catalog) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var missing []string
	for _, d := range expected.List() {
		if _, exists := r.entries[d.Name]; !exists {
			missing = append(missing, d.Name)
		}
	}
	return missing
}
