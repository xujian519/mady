package main

import (
	"testing"
)

func TestSortedPairs_Empty(t *testing.T) {
	result := sortedPairs(nil)
	if len(result) != 0 {
		t.Errorf("expected empty, got %d", len(result))
	}
}

func TestSortedPairs_Single(t *testing.T) {
	input := map[string]string{"z": "value"}
	result := sortedPairs(input)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0][0] != "z" || result[0][1] != "value" {
		t.Errorf("unexpected: %v", result[0])
	}
}

func TestSortedPairs_SortsByKey(t *testing.T) {
	input := map[string]string{
		"b": "second",
		"a": "first",
		"c": "third",
	}
	result := sortedPairs(input)
	if len(result) != 3 {
		t.Fatalf("expected 3, got %d", len(result))
	}
	if result[0][0] != "a" || result[0][1] != "first" {
		t.Errorf("first should be 'a', got %v", result[0])
	}
	if result[1][0] != "b" || result[1][1] != "second" {
		t.Errorf("second should be 'b', got %v", result[1])
	}
	if result[2][0] != "c" || result[2][1] != "third" {
		t.Errorf("third should be 'c', got %v", result[2])
	}
}
