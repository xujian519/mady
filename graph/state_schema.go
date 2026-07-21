package graph

import (
	"fmt"
	"sort"
)

// Reducer 定义 Pregel 同超步内并发写入同一 key 时的合并策略。
// 当未配置 Schema 或 key 不在 Schema 中时，默认行为为 ReducerLastWriteWins（按节点名排序，确定性）。
type Reducer int

const (
	// ReducerLastWriteWins 最后写入者胜出。按节点名字典序排序后合并，保证确定性。
	ReducerLastWriteWins Reducer = iota
	// ReducerAppend 将值追加到已有的 slice 末尾。
	ReducerAppend
	// ReducerUnion 将两个 slice 合并并去重（保持顺序）。
	ReducerUnion
	// ReducerMergeMap 浅合并两个 map[string]any。
	ReducerMergeMap
	// ReducerFailOnConflict 同 key 出现多次写入时立即返回错误。
	ReducerFailOnConflict
)

// String 返回 Reducer 的可读名称。
func (r Reducer) String() string {
	switch r {
	case ReducerLastWriteWins:
		return "last_write_wins"
	case ReducerAppend:
		return "append"
	case ReducerUnion:
		return "union"
	case ReducerMergeMap:
		return "merge_map"
	case ReducerFailOnConflict:
		return "fail_on_conflict"
	default:
		return fmt.Sprintf("unknown(%d)", r)
	}
}

// StateKeyDef 定义单个 state key 的合并策略。
type StateKeyDef struct {
	Reducer Reducer
}

// StateSchema 定义 PregelState 中各 key 的合并策略。
// nil Schema 等同于所有 key 使用 ReducerLastWriteWins（确定性排序）。
type StateSchema struct {
	keys map[string]StateKeyDef
}

// NewStateSchema 创建空的 StateSchema。
func NewStateSchema() *StateSchema {
	return &StateSchema{keys: make(map[string]StateKeyDef)}
}

// Add 注册一个 key 的合并策略，返回 Schema 自身以支持链式调用。
func (s *StateSchema) Add(key string, reducer Reducer) *StateSchema {
	s.keys[key] = StateKeyDef{Reducer: reducer}
	return s
}

// ReducerFor 返回指定 key 的 Reducer。未注册的 key 返回 ReducerLastWriteWins。
func (s *StateSchema) ReducerFor(key string) Reducer {
	if s == nil {
		return ReducerLastWriteWins
	}
	if def, ok := s.keys[key]; ok {
		return def.Reducer
	}
	return ReducerLastWriteWins
}

// DefinedKeys 返回所有已注册的 key（按字典序排序，用于确定性遍历）。
func (s *StateSchema) DefinedKeys() []string {
	if s == nil {
		return nil
	}
	keys := make([]string, 0, len(s.keys))
	for k := range s.keys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ErrStateConflict 在同超步内同一 key 被多个节点写入且 Reducer 为 ReducerFailOnConflict 时返回。
var ErrStateConflict = fmt.Errorf("pregel: state conflict — multiple nodes wrote to the same key with fail_on_conflict reducer")

// mergeWithSchema 使用 Schema 将节点输出合并到目标 state。
// results 按 nodeName 排序以保证合并的确定性。
// 返回 error 仅当遇到 ReducerFailOnConflict 且确实冲突时。
func mergeWithSchema(state PregelState, results map[string]PregelState, schema *StateSchema) error {
	// 按节点名排序，保证遍历顺序确定性。
	sorted := make([]string, 0, len(results))
	for name := range results {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	for _, nodeName := range sorted {
		out := results[nodeName]
		for k, v := range out {
			reducer := schema.ReducerFor(k)

			existing, hasExisting := state[k]

			if !hasExisting {
				state[k] = v
				continue
			}

			// 有冲突：根据 Reducer 决定行为。
			switch reducer {
			case ReducerFailOnConflict:
				return fmt.Errorf("%w: key=%q node=%s", ErrStateConflict, k, nodeName)

			case ReducerLastWriteWins:
				// 确定性：已按 nodeName 排序，后者胜出。
				state[k] = v

			case ReducerAppend:
				existingSlice, _ := toAnySlice(existing)
				newSlice, _ := toAnySlice(v)
				state[k] = append(existingSlice, newSlice...)

			case ReducerUnion:
				existingSlice, _ := toAnySlice(existing)
				newSlice, _ := toAnySlice(v)
				state[k] = unionSlices(existingSlice, newSlice)

			case ReducerMergeMap:
				existingMap, ok1 := existing.(map[string]any)
				newMap, ok2 := v.(map[string]any)
				if !ok1 || !ok2 {
					return fmt.Errorf("pregel: merge_map reducer requires map[string]any values, got %T and %T for key=%q node=%s",
						existing, v, k, nodeName)
				}
				merged := make(map[string]any, len(existingMap)+len(newMap))
				for mk, mv := range existingMap {
					merged[mk] = mv
				}
				for mk, mv := range newMap {
					merged[mk] = mv
				}
				state[k] = merged

			default:
				return fmt.Errorf("pregel: unknown reducer %v for key=%q node=%s", reducer, k, nodeName)
			}
		}
	}

	return nil
}

// toAnySlice 尝试将 value 转换为 []any。支持 []any 和 []string 等常见类型。
func toAnySlice(v any) ([]any, bool) {
	switch s := v.(type) {
	case []any:
		return s, true
	case []string:
		result := make([]any, len(s))
		for i, item := range s {
			result[i] = item
		}
		return result, true
	default:
		return nil, false
	}
}

// unionSlices 合并两个 slice 并去重（保持 a 的顺序，再追加 b 中不重复的元素）。
func unionSlices(a, b []any) []any {
	seen := make(map[string]bool)
	result := make([]any, 0, len(a)+len(b))
	for _, item := range a {
		key := fmt.Sprintf("%v", item)
		if !seen[key] {
			seen[key] = true
			result = append(result, item)
		}
	}
	for _, item := range b {
		key := fmt.Sprintf("%v", item)
		if !seen[key] {
			seen[key] = true
			result = append(result, item)
		}
	}
	return result
}
