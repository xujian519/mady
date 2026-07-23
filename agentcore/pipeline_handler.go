package agentcore

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
)

// StageHandler executes a single pipeline stage identified by an atom name.
// StageHandler is separate from the Atom interface: Atom is a schema/metadata
// contract for validation and documentation, while StageHandler is the
// runtime executor. This separation keeps atoms lightweight and allows
// multiple handler implementations for the same atom.
type StageHandler interface {
	// Name returns the atom name this handler implements (e.g., "search").
	Name() string

	// Execute runs this stage with the given input state and provider,
	// returning the output state to be merged into the pipeline state.
	Execute(ctx context.Context, state PipelineState, provider Provider) (PipelineState, error)
}

// PipelineState is the key-value state passed between pipeline stages.
// It mirrors graph.PregelState's pattern: string keys, arbitrary values.
type PipelineState map[string]any

// GetString returns the string value for key, or "" if missing.
func (s PipelineState) GetString(key string) string {
	if s == nil {
		return ""
	}
	v, ok := s[key]
	if !ok {
		return ""
	}
	str, ok := v.(string)
	if !ok {
		return ""
	}
	return str
}

// SetString sets a string value in the state.
func (s PipelineState) SetString(key, value string) {
	if s == nil {
		return
	}
	s[key] = value
}

// =============================================================================
// StageHandler Registry
// =============================================================================

var (
	handlerRegistry   = make(map[string]StageHandler)
	handlerRegistryMu sync.RWMutex
)

// RegisterStageHandler registers a StageHandler by atom name.
// Duplicate names silently overwrite.
func RegisterStageHandler(h StageHandler) {
	handlerRegistryMu.Lock()
	defer handlerRegistryMu.Unlock()
	if LookupAtom(h.Name()) == nil {
		slog.Warn("pipeline: registered handler for undeclared atom", "handler", h.Name())
	}
	handlerRegistry[h.Name()] = h
}

// LookupStageHandler returns the handler for the given atom name, or nil.
func LookupStageHandler(name string) StageHandler {
	handlerRegistryMu.RLock()
	defer handlerRegistryMu.RUnlock()
	return handlerRegistry[name]
}

// ListStageHandlers returns all registered handlers sorted by name.
func ListStageHandlers() []StageHandler {
	handlerRegistryMu.RLock()
	defer handlerRegistryMu.RUnlock()
	hh := make([]StageHandler, 0, len(handlerRegistry))
	for _, h := range handlerRegistry {
		hh = append(hh, h)
	}
	sort.Slice(hh, func(i, j int) bool {
		return hh[i].Name() < hh[j].Name()
	})
	return hh
}

// =============================================================================
// Errors
// =============================================================================

// StageError wraps errors from stage execution with the stage ID for tracing.
type StageError struct {
	StageID string
	Atom    string
	Err     error
}

func (e *StageError) Error() string {
	return fmt.Sprintf("stage %q (atom:%s): %v", e.StageID, e.Atom, e.Err)
}

func (e *StageError) Unwrap() error { return e.Err }

// InterruptStageError is returned by approval-gate stages to pause the
// pipeline for human review, mirroring agentcore.InterruptError.
type InterruptStageError struct {
	StageID string
	Message string
	Data    map[string]any
}

func (e *InterruptStageError) Error() string {
	return fmt.Sprintf("pipeline interrupted at stage %q: %s", e.StageID, e.Message)
}

// IsInterruptStage reports whether err is a pipeline interruption.
func IsInterruptStage(err error) bool {
	_, ok := err.(*InterruptStageError)
	return ok
}
