package ipc

import "testing"

// TestInvariant_AllDomainsCompleteness 是 companion 检查的直接用例：
// 内置领域表必须满足八类齐全、关键词非空且无重复。
func TestInvariant_AllDomainsCompleteness(t *testing.T) {
	if err := checkAllDomainsCompleteness(); err != nil {
		t.Fatalf("built-in IPC domain table violates completeness: %v", err)
	}
}
