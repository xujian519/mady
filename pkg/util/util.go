package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrorString converts an error to a stable string form for JSON/event payloads.
// It returns an empty string for nil so callers can use it with omitempty fields.
func ErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// DefaultString returns fallback when value is blank after trimming whitespace.
func DefaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// FirstNonEmpty returns the first non-blank string from values.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// EnvOrDefault returns an environment variable when it is set, otherwise fallback.
func EnvOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func ValidateKey(key string) error {
	if key == "" {
		return fmt.Errorf("key must not be empty")
	}
	if key == "." || key == ".." || key != filepath.Base(key) {
		return fmt.Errorf("invalid key %q: must be a single path segment", key)
	}
	return nil
}
