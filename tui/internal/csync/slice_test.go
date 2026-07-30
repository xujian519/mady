package csync

import (
	"sync"
	"testing"
)

func TestSliceAppendAndCopy(t *testing.T) {
	s := NewSlice[int]()
	s.Append(1, 2, 3)
	got := s.Copy()
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("expected [1 2 3], got %v", got)
	}
}

func TestSliceCopyReturnsCopy(t *testing.T) {
	s := NewSlice[int]()
	s.Append(42)
	cp := s.Copy()
	cp[0] = 99 // mutate the copy
	got := s.Copy()
	if got[0] != 42 {
		t.Fatalf("Copy should return independent copy; got %v", got)
	}
}

func TestSliceSetSlice(t *testing.T) {
	s := NewSlice[string]()
	s.Append("a", "b")
	s.SetSlice([]string{"x", "y", "z"})
	got := s.Copy()
	if len(got) != 3 || got[2] != "z" {
		t.Fatalf("expected [x y z], got %v", got)
	}
}

func TestSliceEmpty(t *testing.T) {
	s := NewSlice[float64]()
	got := s.Copy()
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestSliceNewSliceFrom(t *testing.T) {
	orig := []int{10, 20, 30}
	s := NewSliceFrom(orig)
	got := s.Copy()
	if len(got) != 3 || got[1] != 20 {
		t.Fatalf("expected [10 20 30], got %v", got)
	}
	orig[0] = 999 // mutate original
	if got[0] != 10 {
		t.Fatalf("NewSliceFrom should copy input, got %v", got)
	}
}

func TestSliceConcurrentSafe(t *testing.T) {
	s := NewSlice[int]()
	var wg sync.WaitGroup
	n := 100
	wg.Add(n)
	for i := range n {
		go func(v int) {
			defer wg.Done()
			s.Append(v)
		}(i)
	}
	wg.Wait()
	got := s.Copy()
	if len(got) != n {
		t.Fatalf("expected %d items, got %d", n, len(got))
	}
	// Verify all values are present (no duplicates or losses)
	seen := make(map[int]bool, len(got))
	for _, v := range got {
		seen[v] = true
	}
	if len(seen) != n {
		t.Fatalf("expected %d unique values, got %d", n, len(seen))
	}
}
