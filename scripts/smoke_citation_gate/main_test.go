package main

import (
	"testing"
)

func TestEnvInt_DefaultOnMissing(t *testing.T) {
	// key should not exist
	n := envInt("MADY_TEST_NONEXISTENT_KEY_XYZ", 42)
	if n != 42 {
		t.Errorf("expected default 42, got %d", n)
	}
}

func TestEnvInt_ReadsValue(t *testing.T) {
	t.Setenv("MADY_TEST_INT", "99")
	n := envInt("MADY_TEST_INT", 10)
	if n != 99 {
		t.Errorf("expected 99, got %d", n)
	}
}

func TestEnvInt_InvalidFallsBack(t *testing.T) {
	t.Setenv("MADY_TEST_BAD", "not-a-number")
	n := envInt("MADY_TEST_BAD", 5)
	if n != 5 {
		t.Errorf("expected default 5, got %d", n)
	}
}

func TestEnvInt_ZeroIsValid(t *testing.T) {
	t.Setenv("MADY_TEST_ZERO", "0")
	n := envInt("MADY_TEST_ZERO", 99)
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

func TestEnvInt_NegativeIsValid(t *testing.T) {
	t.Setenv("MADY_TEST_NEG", "-3")
	n := envInt("MADY_TEST_NEG", 99)
	if n != -3 {
		t.Errorf("expected -3, got %d", n)
	}
}
