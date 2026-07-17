package agentcore

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// 本文件实现工具契约管理（Tool Contract），对齐 docs/decisions/reasonix-analysis.md
// §9 P2 优先级：内置工具 Schema 的文档化契约 + Schema 规范化 + 调和测试。
//
// 设计原则：
//   - 所有内置工具的 Schema 在注册时自动规范化（CanonicalizeSchema）
//   - SchemaDigest 提供确定性摘要用于契约比对
//   - ToolContract 记录每个工具的已发布契约
//   - 调和测试（contract_test.go）确保新增工具必更新契约

// ToolContract 记录一个工具在契约中的已发布信息。
type ToolContract struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"` // "read" | "write" | "command" | "network" | "other"
	Digest      string `json:"digest"`   // SchemaDigest (SHA-256)
}

// CanonicalizeSchema 规范化一个 JSON Schema map，返回键排序、去空值的副本。
// 用于确保 Schema 在不同运行中产生相同的 Digest。
func CanonicalizeSchema(schema map[string]any) map[string]any {
	return canonicalizeMap(schema).(map[string]any)
}

func canonicalizeMap(m map[string]any) any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if isEmptyValue(v) {
			continue
		}
		out[k] = canonicalizeValue(v)
	}
	return out
}

func canonicalizeValue(v any) any {
	switch vv := v.(type) {
	case map[string]any:
		return canonicalizeMap(vv)
	case []any:
		return canonicalizeSlice(vv)
	default:
		return v
	}
}

func canonicalizeSlice(s []any) any {
	if len(s) == 0 {
		return nil
	}
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = canonicalizeValue(v)
	}
	return out
}

func isEmptyValue(v any) bool {
	if v == nil {
		return true
	}
	switch vv := v.(type) {
	case bool:
		return !vv
	case string:
		return vv == ""
	case []any:
		return len(vv) == 0
	case map[string]any:
		return len(vv) == 0
	}
	return false
}

// SchemaDigest 返回 Schema 的确定性 SHA-256 摘要（64 字符十六进制）。
// 输入 Schema 先经 CanonicalizeSchema 规范化再序列化 JSON。
func SchemaDigest(schema map[string]any) string {
	canon := CanonicalizeSchema(schema)
	// 用 sorted keys 手动序列化避免 Go map 迭代随机性
	jsonStr := serializeSortedJSON(canon, "")
	hash := sha256.Sum256([]byte(jsonStr))
	return fmt.Sprintf("%x", hash)
}

// serializeSortedJSON 将规范化后的值序列化为稳定的 JSON 字符串。
func serializeSortedJSON(v any, indent string) string {
	switch vv := v.(type) {
	case map[string]any:
		if len(vv) == 0 {
			return "{}"
		}
		keys := make([]string, 0, len(vv))
		for k := range vv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			parts = append(parts,
				fmt.Sprintf(`%q:%s`, k, serializeSortedJSON(vv[k], indent+"  ")))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []any:
		if len(vv) == 0 {
			return "[]"
		}
		var parts []string
		for _, item := range vv {
			parts = append(parts, serializeSortedJSON(item, indent+"  "))
		}
		return "[" + strings.Join(parts, ",") + "]"
	case string:
		return fmt.Sprintf("%q", vv)
	case bool:
		if vv {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(vv, 'G', -1, 64)
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%q", fmt.Sprintf("%v", vv))
	}
}

// ClassifyTool 根据工具名和 ReadOnly 标记将工具分类。
func ClassifyTool(name string, readOnly bool) string {
	writeTools := map[string]bool{
		"write_file": true, "edit": true, "delete": true, "move": true,
		"patch": true, "apply_patch": true,
	}
	commandTools := map[string]bool{
		"bash": true, "execute_code": true, "process": true,
	}
	networkTools := map[string]bool{
		"web_search": true, "web_fetch": true, "browser": true,
	}

	if writeTools[name] {
		return "write"
	}
	if commandTools[name] {
		return "command"
	}
	if networkTools[name] {
		return "network"
	}
	if readOnly {
		return "read"
	}
	return "other"
}
