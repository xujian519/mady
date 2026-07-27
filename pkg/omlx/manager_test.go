package omlx

import (
	"context"
	"os"
	"testing"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager(8000, "test-key")
	if mgr.port != 8000 {
		t.Fatalf("expected port 8000, got %d", mgr.port)
	}
	if mgr.apiKey != "test-key" {
		t.Fatalf("expected apiKey test-key, got %s", mgr.apiKey)
	}
	if mgr.healthURL != "http://127.0.0.1:8000/v1/models" {
		t.Fatalf("unexpected healthURL: %s", mgr.healthURL)
	}
}

func TestIsRunning_NoAPIKey(t *testing.T) {
	mgr := NewManager(8000, "")
	if mgr.IsRunning() {
		t.Fatal("IsRunning should return false when apiKey is empty")
	}
}

func TestStart_NoAPIKey(t *testing.T) {
	mgr := NewManager(8000, "")
	err := mgr.Start(context.Background())
	if err != ErrNoAPIKey {
		t.Fatalf("expected ErrNoAPIKey, got %v", err)
	}
}

func TestEnsureRunning_NoAPIKey(t *testing.T) {
	// Should not panic or error with empty API key.
	mgr := NewManager(8000, "")
	mgr.EnsureRunning(context.Background())
}

func TestEnsureRunning_AlreadyRunning(t *testing.T) {
	// If oMLX is already running locally, EnsureRunning should detect it.
	// This test is best-effort — it's OK if oMLX is not running.
	apiKey := os.Getenv("OMLX_API_KEY")
	if apiKey == "" {
		t.Skip("OMLX_API_KEY not set, skipping")
	}
	mgr := NewManager(8000, apiKey)
	mgr.EnsureRunning(context.Background())
	// Should not panic; the result depends on whether oMLX is running.
}

func TestCheckHealth_NoServer(t *testing.T) {
	// Point to a port that's not running to verify health check failure.
	mgr := NewManager(19999, "test-key")
	if mgr.IsRunning() {
		t.Fatal("IsRunning should return false when no server on port")
	}
}

func TestStop_NoProcess(t *testing.T) {
	mgr := NewManager(8000, "test-key")
	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop with no process should be a no-op, got: %v", err)
	}
}
