package main

// settings_store.go implements a two-level (global + session) settings store for
// the Mady TUI. Global settings persist to ~/.mady/settings.json; session
// overrides are in-memory only and discarded when the session ends.
//
// Key design:
//   - Read cascades: session → global → default value.
//   - Global writes use atomic file replacement (tmp → rename).
//   - All slash commands and the settings panel read/write through this store,
//     eliminating the toggle double-trigger problem.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// SettingsScope indicates whether a setting applies globally or per-session.
type SettingsScope int

const (
	SettingsScopeGlobal  SettingsScope = iota // persisted to ~/.mady/settings.json
	SettingsScopeSession                      // in-memory only, discarded on exit
)

// Known setting keys and their default values.
const (
	SettingKeyTheme    = "theme"
	SettingKeyPlan     = "plan"
	SettingKeyReview   = "review"
	SettingKeyThinking = "thinking"

	DefaultTheme    = "dark"
	DefaultPlan     = "off"
	DefaultReview   = "off"
	DefaultThinking = "default"
)

// defaultValues maps each setting key to its factory-default value.
var defaultValues = map[string]string{
	SettingKeyTheme:    DefaultTheme,
	SettingKeyPlan:     DefaultPlan,
	SettingKeyReview:   DefaultReview,
	SettingKeyThinking: DefaultThinking,
}

// validValues maps each setting key to its set of accepted values.
var validValues = map[string][]string{
	SettingKeyTheme:    {"dark", "light", "mady-dark"},
	SettingKeyPlan:     {"on", "off"},
	SettingKeyReview:   {"on", "off"},
	SettingKeyThinking: {"default", "summarized", "omitted"},
}

// SettingsStore is the single source of truth for all TUI settings.
// It is safe for concurrent use.
type SettingsStore struct {
	mu       sync.RWMutex
	global   map[string]string // loaded from / persisted to ~/.mady/settings.json
	session  map[string]string // in-memory overrides, cleared on Reset
	filePath string
}

// NewSettingsStore creates a store backed by filePath. If the file does not
// exist, defaults are used and the file is created on the first Set call.
func NewSettingsStore(filePath string) (*SettingsStore, error) {
	s := &SettingsStore{
		global:   copyMap(defaultValues),
		session:  make(map[string]string),
		filePath: filePath,
	}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

// Get returns the effective value for key: session override > global > default.
func (s *SettingsStore) Get(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.session[key]; ok && v != "" {
		return v
	}
	if v, ok := s.global[key]; ok && v != "" {
		return v
	}
	return defaultValues[key]
}

// Set writes a value. Global-scope changes are persisted immediately with atomic
// file replacement. Session-scope changes are in-memory only.
func (s *SettingsStore) Set(key, value string, scope SettingsScope) error {
	if err := s.validate(key, value); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch scope {
	case SettingsScopeSession:
		s.session[key] = value
	case SettingsScopeGlobal:
		s.global[key] = value
		delete(s.session, key) // global write clears any session override
		return s.saveLocked()
	}
	return nil
}

// Reset clears all session overrides and resets global to defaults, then persists.
func (s *SettingsStore) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.global = copyMap(defaultValues)
	s.session = make(map[string]string)
	return s.saveLocked()
}

// Export returns a snapshot of the current effective settings as a map.
func (s *SettingsStore) Export() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyMap(defaultValues)
	for k, v := range s.global {
		out[k] = v
	}
	for k, v := range s.session {
		out[k] = v
	}
	return out
}

// validate checks that the key is known and the value is accepted.
func (s *SettingsStore) validate(key, value string) error {
	valid, ok := validValues[key]
	if !ok {
		return fmt.Errorf("settings: unknown key %q", key)
	}
	for _, v := range valid {
		if v == value {
			return nil
		}
	}
	return fmt.Errorf("settings: invalid value %q for %s (accepted: %v)", value, key, valid)
}

// load reads the JSON file into s.global, keeping defaults for missing keys.
func (s *SettingsStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("settings: parse %s: %w", s.filePath, err)
	}
	for k, v := range raw {
		if s.validate(k, v) == nil {
			s.global[k] = v
		}
	}
	return nil
}

// saveLocked writes s.global to the JSON file atomically. Caller must hold s.mu.
func (s *SettingsStore) saveLocked() error {
	if s.filePath == "" {
		return nil
	}
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.global, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.filePath)
}

func copyMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
