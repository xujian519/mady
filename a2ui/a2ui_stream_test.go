package a2ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestSurfaceStoreSurfacesCopy(t *testing.T) {
	store := NewSurfaceStore()
	_ = store.Apply(NewCreateSurface("a", BasicCatalogID))
	_ = store.Apply(NewCreateSurface("b", BasicCatalogID))

	got := store.Surfaces()
	if len(got) != 2 {
		t.Fatalf("expected 2 surfaces, got %d", len(got))
	}
	if _, ok := got["a"]; !ok {
		t.Fatal("surface 'a' missing")
	}
	// Verify it's a copy: modifying the returned map doesn't affect the store.
	delete(got, "a")
	if _, ok := store.Surface("a"); !ok {
		t.Fatal("store surface 'a' was affected by delete on returned copy")
	}
}

func TestValidationErrorError(t *testing.T) {
	e1 := ValidationError{Code: CodeValidationFailed, SurfaceID: "s", Path: "/a", Message: "bad"}
	s1 := e1.Error()
	if s1 != "VALIDATION_FAILED at /a (surface \"s\"): bad" {
		t.Fatalf("unexpected error with path: %q", s1)
	}
	e2 := ValidationError{Code: "ERR", SurfaceID: "s", Message: "oops"}
	s2 := e2.Error()
	if s2 != "ERR (surface \"s\"): oops" {
		t.Fatalf("unexpected error without path: %q", s2)
	}
}

func TestStreamMore(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(`{"version":"v0.9.1","deleteSurface":{"surfaceId":"s"}}` + "\n")
	buf.WriteString(`{"version":"v0.9.1","deleteSurface":{"surfaceId":"t"}}` + "\n")

	dec := NewDecoder(&buf)
	if !dec.More() {
		t.Fatal("expected More() to be true before first decode")
	}
	_, _ = dec.Decode()
	if !dec.More() {
		t.Fatal("expected More() to be true after first decode")
	}
	_, _ = dec.Decode()
	if dec.More() {
		t.Fatal("expected More() to be false after exhausting stream")
	}
}

func TestStreamEncodeVersionFill(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	// Envelope with empty version should have it filled in by Encode.
	env := Envelope{DeleteSurface: &DeleteSurface{SurfaceID: "s"}}
	if err := enc.Encode(env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"version":"v0.9.1"`) {
		t.Fatal("version not filled by Encode")
	}
	// Also test with version already set (no-op branch).
	buf.Reset()
	if err := enc.Encode(NewDeleteSurface("t")); err != nil {
		t.Fatal(err)
	}
}

type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) { return 0, errors.New("write error") }

func TestStreamEncodeAllError(t *testing.T) {
	enc := NewEncoder(errWriter{})
	// First envelope: Encode should fail due to write error.
	env := Envelope{DeleteSurface: &DeleteSurface{SurfaceID: "s"}}
	err := enc.Encode(env)
	if err == nil {
		t.Fatal("expected encode error")
	}
	// EncodeAll should also fail.
	err = enc.EncodeAll([]Envelope{NewDeleteSurface("s"), NewDeleteSurface("t")})
	if err == nil {
		t.Fatal("expected encode error from EncodeAll")
	}
}

func TestStreamDecodeWithDefaultVersion(t *testing.T) {
	// Envelope without version; Decode should set the default.
	data := `{"deleteSurface":{"surfaceId":"s"}}` + "\n"
	dec := NewDecoder(strings.NewReader(data))
	env, err := dec.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if env.Version != Version {
		t.Fatalf("Decode did not set default version: got %q, want %q", env.Version, Version)
	}
}

func TestStreamDecodeError(t *testing.T) {
	dec := NewDecoder(strings.NewReader("not valid json\n"))
	if _, err := dec.Decode(); err == nil {
		t.Fatal("expected decode error")
	}
}
