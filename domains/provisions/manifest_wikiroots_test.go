package provisions

import "testing"

// TestLoadManifest_WikiRoots 验证 Step 4.1：事实源 manifest 并入 Kimi 拷贝版的
// wiki_roots 增量后，LoadManifest（默认路径）能读到。
func TestLoadManifest_WikiRoots(t *testing.T) {
	m, err := LoadManifest("")
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(m.Provisions) == 0 {
		t.Fatal("manifest 无 provision 条目")
	}
	seen := false
	for _, p := range m.Provisions {
		if p.ID == "P-A01" {
			seen = len(p.WikiRoots) > 0
			break
		}
	}
	if !seen {
		t.Error("P-A01 应能读到合并后的 wiki_roots（增量未并入）")
	}
}
