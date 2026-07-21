package graph

import (
	"sync"
	"testing"
)

func TestNewStateSchema(t *testing.T) {
	s := NewStateSchema()
	if s == nil {
		t.Fatal("NewStateSchema returned nil")
	}
	if len(s.keys) != 0 {
		t.Fatal("new schema should have no keys")
	}
}

func TestStateSchema_Add(t *testing.T) {
	s := NewStateSchema()
	s.Add("key1", ReducerAppend).
		Add("key2", ReducerUnion).
		Add("key3", ReducerFailOnConflict)

	if r := s.ReducerFor("key1"); r != ReducerAppend {
		t.Errorf("key1: expected append, got %v", r)
	}
	if r := s.ReducerFor("key2"); r != ReducerUnion {
		t.Errorf("key2: expected union, got %v", r)
	}
	if r := s.ReducerFor("key3"); r != ReducerFailOnConflict {
		t.Errorf("key3: expected fail_on_conflict, got %v", r)
	}
}

func TestStateSchema_ReducerFor_UnknownKey(t *testing.T) {
	s := NewStateSchema()
	s.Add("known", ReducerAppend)

	// 未注册的 key 默认 LastWriteWins
	if r := s.ReducerFor("unknown"); r != ReducerLastWriteWins {
		t.Errorf("unknown key: expected last_write_wins, got %v", r)
	}
}

func TestStateSchema_ReducerFor_NilSchema(t *testing.T) {
	var s *StateSchema
	if r := s.ReducerFor("any"); r != ReducerLastWriteWins {
		t.Errorf("nil schema: expected last_write_wins, got %v", r)
	}
}

func TestStateSchema_DefinedKeys(t *testing.T) {
	s := NewStateSchema()
	s.Add("c", ReducerAppend)
	s.Add("a", ReducerUnion)
	s.Add("b", ReducerFailOnConflict)

	keys := s.DefinedKeys()
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	// 确认字典序：a, b, c
	expected := []string{"a", "b", "c"}
	for i, k := range keys {
		if k != expected[i] {
			t.Errorf("keys[%d]: expected %q, got %q", i, expected[i], k)
		}
	}
}

func TestStateSchema_DefinedKeys_Nil(t *testing.T) {
	var s *StateSchema
	if keys := s.DefinedKeys(); keys != nil {
		t.Errorf("nil schema: expected nil, got %v", keys)
	}
}

// =============================================================================
// mergeWithSchema tests
// =============================================================================

func TestMergeWithSchema_LastWriteWins_Deterministic(t *testing.T) {
	schema := NewStateSchema()
	// 不注册任何 key → 默认 LastWriteWins

	state := PregelState{"x": "initial"}
	results := map[string]PregelState{
		"node_a": {"shared": "from_a"},
		"node_b": {"shared": "from_b"}, // 应该胜出（b > a 字典序）
	}

	if err := mergeWithSchema(state, results, schema); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	// node_b 在 node_a 之后（字典序），b 胜出
	if state["shared"] != "from_b" {
		t.Errorf("expected 'from_b' (last writer), got %v", state["shared"])
	}
}

func TestMergeWithSchema_FailOnConflict_Error(t *testing.T) {
	schema := NewStateSchema().Add("conflict_key", ReducerFailOnConflict)

	state := PregelState{}
	results := map[string]PregelState{
		"node_a": {"conflict_key": "a"},
		"node_b": {"conflict_key": "b"},
	}

	err := mergeWithSchema(state, results, schema)
	if err == nil {
		t.Fatal("expected error on conflict, got nil")
	}
	if !errorsIs(err, ErrStateConflict) {
		t.Errorf("expected ErrStateConflict, got %v", err)
	}
}

func TestMergeWithSchema_FailOnConflict_NoConflict(t *testing.T) {
	schema := NewStateSchema().Add("conflict_key", ReducerFailOnConflict)

	state := PregelState{}
	results := map[string]PregelState{
		"node_a": {"conflict_key": "only_writer"},
	}

	if err := mergeWithSchema(state, results, schema); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state["conflict_key"] != "only_writer" {
		t.Errorf("expected 'only_writer', got %v", state["conflict_key"])
	}
}

func TestMergeWithSchema_Append(t *testing.T) {
	schema := NewStateSchema().Add("items", ReducerAppend)

	state := PregelState{"items": []any{"a", "b"}}
	results := map[string]PregelState{
		"node_a": {"items": []any{"c"}},
		"node_b": {"items": []any{"d"}},
	}

	if err := mergeWithSchema(state, results, schema); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	items, ok := state["items"].([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", state["items"])
	}
	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}
	expected := []string{"a", "b", "c", "d"}
	for i, item := range items {
		if item.(string) != expected[i] {
			t.Errorf("items[%d]: expected %q, got %v", i, expected[i], item)
		}
	}
}

func TestMergeWithSchema_Append_StringSlice(t *testing.T) {
	schema := NewStateSchema().Add("items", ReducerAppend)

	state := PregelState{"items": []string{"a"}}
	results := map[string]PregelState{
		"node_a": {"items": []string{"b"}},
	}

	if err := mergeWithSchema(state, results, schema); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	items, ok := state["items"].([]any)
	if !ok {
		t.Fatalf("expected []any after merge, got %T", state["items"])
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestMergeWithSchema_Union(t *testing.T) {
	schema := NewStateSchema().Add("tags", ReducerUnion)

	state := PregelState{"tags": []any{"a", "b"}}
	results := map[string]PregelState{
		"node_a": {"tags": []any{"b", "c"}}, // "b" 重复
	}

	if err := mergeWithSchema(state, results, schema); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	tags, ok := state["tags"].([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", state["tags"])
	}
	if len(tags) != 3 {
		t.Fatalf("expected 3 unique tags, got %d: %v", len(tags), tags)
	}
}

func TestMergeWithSchema_MergeMap(t *testing.T) {
	schema := NewStateSchema().Add("meta", ReducerMergeMap)

	state := PregelState{"meta": map[string]any{"a": 1}}
	results := map[string]PregelState{
		"node_a": {"meta": map[string]any{"b": 2}},
		"node_b": {"meta": map[string]any{"c": 3}},
	}

	if err := mergeWithSchema(state, results, schema); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	meta, ok := state["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", state["meta"])
	}
	if len(meta) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(meta))
	}
	if meta["a"] != 1 || meta["b"] != 2 || meta["c"] != 3 {
		t.Errorf("unexpected meta: %v", meta)
	}
}

func TestMergeWithSchema_MergeMap_TypeMismatch(t *testing.T) {
	schema := NewStateSchema().Add("meta", ReducerMergeMap)

	state := PregelState{"meta": "not_a_map"}
	results := map[string]PregelState{
		"node_a": {"meta": map[string]any{"b": 2}},
	}

	err := mergeWithSchema(state, results, schema)
	if err == nil {
		t.Fatal("expected error on type mismatch, got nil")
	}
}

func TestMergeWithSchema_NoConflict_DifferentKeys(t *testing.T) {
	schema := NewStateSchema()

	state := PregelState{}
	results := map[string]PregelState{
		"node_a": {"key_a": "value_a"},
		"node_b": {"key_b": "value_b"},
	}

	if err := mergeWithSchema(state, results, schema); err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	if state["key_a"] != "value_a" {
		t.Errorf("key_a: expected 'value_a', got %v", state["key_a"])
	}
	if state["key_b"] != "value_b" {
		t.Errorf("key_b: expected 'value_b', got %v", state["key_b"])
	}
}

func TestMergeWithSchema_NewKeyFromNode(t *testing.T) {
	// 节点引入了 state 中不存在的 key，应该正常添加
	schema := NewStateSchema()

	state := PregelState{"existing": "old"}
	results := map[string]PregelState{
		"node_a": {"new_key": "new_value"},
	}

	if err := mergeWithSchema(state, results, schema); err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	if state["existing"] != "old" {
		t.Errorf("existing key was mutated")
	}
	if state["new_key"] != "new_value" {
		t.Errorf("new_key: expected 'new_value', got %v", state["new_key"])
	}
}

// =============================================================================
// Concurrency test
// =============================================================================

func TestMergeWithSchema_ConcurrentSafety(t *testing.T) {
	// 模拟 Pregel 模式：多个 goroutine 同时写入 results，然后单线程 merge。
	// mergeWithSchema 自身不需要 mutex（单线程调用），
	// 本测试验证多次运行结果一致（确定性）。
	schema := NewStateSchema().Add("shared", ReducerLastWriteWins)

	for i := 0; i < 100; i++ {
		state := PregelState{}
		results := map[string]PregelState{
			"node_a": {"shared": "a", "only_a": "x"},
			"node_b": {"shared": "b", "only_b": "y"},
		}

		if err := mergeWithSchema(state, results, schema); err != nil {
			t.Fatalf("iteration %d: merge failed: %v", i, err)
		}

		// 确定性验证：node_b 字典序在 node_a 之后，应该胜出
		if state["shared"] != "b" {
			t.Errorf("iteration %d: expected 'b' (deterministic last-write), got %v", i, state["shared"])
		}
		if state["only_a"] != "x" || state["only_b"] != "y" {
			t.Errorf("iteration %d: non-shared keys corrupted", i)
		}
	}
}

func TestReducer_String(t *testing.T) {
	tests := []struct {
		r        Reducer
		expected string
	}{
		{ReducerLastWriteWins, "last_write_wins"},
		{ReducerAppend, "append"},
		{ReducerUnion, "union"},
		{ReducerMergeMap, "merge_map"},
		{ReducerFailOnConflict, "fail_on_conflict"},
		{Reducer(99), "unknown(99)"},
	}
	for _, tt := range tests {
		if s := tt.r.String(); s != tt.expected {
			t.Errorf("Reducer(%d).String() = %q, want %q", tt.r, s, tt.expected)
		}
	}
}

// errorsIs is a local helper to check error wrapping without importing errors.
func errorsIs(err, target error) bool {
	for {
		if err == target {
			return true
		}
		// Simple unwrap support
		type wrapper interface {
			Unwrap() error
		}
		if w, ok := err.(wrapper); ok {
			err = w.Unwrap()
			if err == nil {
				return false
			}
		} else {
			return false
		}
	}
}

// Ensure ErrStateConflict implements the error interface correctly.
var _ error = ErrStateConflict

// Ensure mergeWithSchema is not racy under concurrent map access.
func TestMergeWithSchema_Race(t *testing.T) {
	// 多个 goroutine 构建 results map，单线程 merge
	// 验证在 -race 下无数据竞争。
	schema := NewStateSchema().
		Add("append_key", ReducerAppend).
		Add("union_key", ReducerUnion).
		Add("merge_key", ReducerMergeMap).
		Add("fail_key", ReducerFailOnConflict)

	var wg sync.WaitGroup
	for round := 0; round < 50; round++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results := map[string]PregelState{
				"a": {
					"append_key": []any{"x"},
					"union_key":  []any{"tag1"},
					"merge_key":  map[string]any{"m1": 1},
					"fail_key":   "only_writer",
					"lww_key":    "from_a",
				},
				"b": {
					"append_key": []any{"y"},
					"union_key":  []any{"tag2"},
					"merge_key":  map[string]any{"m2": 2},
					"lww_key":    "from_b",
				},
			}
			state := PregelState{}
			if err := mergeWithSchema(state, results, schema); err != nil {
				t.Errorf("merge failed: %v", err)
			}
		}()
	}
	wg.Wait()
}
