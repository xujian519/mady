package csync

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestValueGetSet(t *testing.T) {
	v := NewValue(42)
	if got := v.Get(); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}

	v.Set(100)
	if got := v.Get(); got != 100 {
		t.Fatalf("expected 100, got %d", got)
	}
}

func TestValueString(t *testing.T) {
	v := NewValue("hello")
	if got := v.Get(); got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestValueStruct(t *testing.T) {
	type S struct{ X, Y int }
	v := NewValue(S{X: 1, Y: 2})
	v.Set(S{X: 3, Y: 4})
	if got := v.Get(); got.X != 3 || got.Y != 4 {
		t.Fatalf("unexpected value: %+v", got)
	}
}

func TestValuePanicsOnPointer(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for pointer type")
		}
	}()
	NewValue(&struct{}{})
}

func TestValuePanicsOnSlice(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for slice type")
		}
	}()
	NewValue([]int{})
}

func TestValuePanicsOnMap(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for map type")
		}
	}()
	NewValue(map[string]int{})
}

func TestValueConcurrent(t *testing.T) {
	v := NewValue(0)
	var wg sync.WaitGroup
	const n = 100

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			v.Set(val)
		}(i)
	}
	wg.Wait()
	// Final value should be one of the set values (concurrent, not deterministic).
	_ = v.Get()
}

func TestSliceAppend(t *testing.T) {
	s := NewSlice[int]()
	s.Append(1, 2, 3)
	if n := s.Len(); n != 3 {
		t.Fatalf("expected len 3, got %d", n)
	}
	v, ok := s.Get(1)
	if !ok || v != 2 {
		t.Fatalf("expected 2 at index 1, got %d (ok=%v)", v, ok)
	}
}

func TestSliceGetOutOfBounds(t *testing.T) {
	s := NewSlice[int]()
	_, ok := s.Get(0)
	if ok {
		t.Fatal("expected false for out-of-bounds Get")
	}
	_, ok = s.Get(-1)
	if ok {
		t.Fatal("expected false for negative index")
	}
}

func TestSliceCopy(t *testing.T) {
	s := NewSliceFrom([]int{1, 2, 3})
	copied := s.Copy()
	if len(copied) != 3 || copied[0] != 1 || copied[1] != 2 || copied[2] != 3 {
		t.Fatalf("unexpected copy: %v", copied)
	}
	// Mutating copy should not affect original.
	copied[0] = 99
	if v, _ := s.Get(0); v != 1 {
		t.Fatal("copy should be independent")
	}
}

func TestSliceSetSlice(t *testing.T) {
	s := NewSliceFrom([]int{1, 2, 3})
	s.SetSlice([]int{4, 5})
	if n := s.Len(); n != 2 {
		t.Fatalf("expected len 2 after SetSlice, got %d", n)
	}
	v, _ := s.Get(0)
	if v != 4 {
		t.Fatalf("expected 4, got %d", v)
	}
}

func TestSliceConcurrent(t *testing.T) {
	s := NewSlice[int]()
	var wg sync.WaitGroup
	const n = 100

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			s.Append(val)
		}(i)
	}
	wg.Wait()
	if s.Len() != n {
		t.Fatalf("expected len %d, got %d", n, s.Len())
	}
}

func TestMapSetGet(t *testing.T) {
	m := NewMap[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)

	v, ok := m.Get("a")
	if !ok || v != 1 {
		t.Fatalf("expected 1, got %d (ok=%v)", v, ok)
	}

	_, ok = m.Get("nonexistent")
	if ok {
		t.Fatal("expected false for missing key")
	}

	if n := m.Len(); n != 2 {
		t.Fatalf("expected len 2, got %d", n)
	}
}

func TestMapDel(t *testing.T) {
	m := NewMap[string, int]()
	m.Set("a", 1)
	m.Del("a")
	if _, ok := m.Get("a"); ok {
		t.Fatal("expected key to be deleted")
	}
	m.Del("nonexistent") // idempotent
}

func TestMapGetOrSet(t *testing.T) {
	m := NewMap[string, int]()
	v := m.GetOrSet("a", func() int { return 42 })
	if v != 42 {
		t.Fatalf("expected 42, got %d", v)
	}
	// Second call should return existing value without calling fn.
	called := false
	v = m.GetOrSet("a", func() int { called = true; return 99 })
	if v != 42 || called {
		t.Fatalf("expected 42 without calling fn, got %d (called=%v)", v, called)
	}
}

func TestMapTake(t *testing.T) {
	m := NewMap[string, int]()
	m.Set("a", 1)
	v, ok := m.Take("a")
	if !ok || v != 1 {
		t.Fatalf("expected 1, got %d (ok=%v)", v, ok)
	}
	if _, ok := m.Get("a"); ok {
		t.Fatal("expected key to be removed after Take")
	}
}

func TestMapCopy(t *testing.T) {
	m := NewMapFrom(map[string]int{"a": 1, "b": 2})
	copied := m.Copy()
	if len(copied) != 2 || copied["a"] != 1 {
		t.Fatalf("unexpected copy: %v", copied)
	}
	copied["c"] = 3
	if _, ok := m.Get("c"); ok {
		t.Fatal("copy should be independent")
	}
}

func TestMapReset(t *testing.T) {
	m := NewMap[string, int]()
	m.Set("a", 1)
	m.Reset(map[string]int{"b": 2})
	if n := m.Len(); n != 1 {
		t.Fatalf("expected len 1 after Reset, got %d", n)
	}
	v, _ := m.Get("b")
	if v != 2 {
		t.Fatalf("expected 2, got %d", v)
	}
}

func TestMapJSON(t *testing.T) {
	m := NewMap[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var restored Map[string, int]
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if restored.Len() != 2 {
		t.Fatalf("expected 2 keys after unmarshal, got %d", restored.Len())
	}
	v, _ := restored.Get("a")
	if v != 1 {
		t.Fatalf("expected 1, got %d", v)
	}
}

func TestMapConcurrent(t *testing.T) {
	m := NewMap[int, int]()
	var wg sync.WaitGroup
	const n = 100

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(key int) {
			defer wg.Done()
			m.Set(key, key*2)
		}(i)
	}
	wg.Wait()
	if m.Len() != n {
		t.Fatalf("expected len %d, got %d", n, m.Len())
	}
}
