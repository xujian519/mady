package worker

import (
	"testing"
)

func TestDefaultWorkerRegistration(t *testing.T) {
	catalog := NewCatalog()
	for _, d := range DefaultWorkers() {
		if err := catalog.Register(d); err != nil {
			t.Errorf("注册 Worker %q 失败: %v", d.Name, err)
		}
	}

	// 验证所有 Worker 都有名称和描述
	issues := catalog.Verify()
	for _, iss := range issues {
		t.Errorf("验证问题: %s", iss)
	}

	// 验证 Registry 全量注册
	registry := NewRegistry()
	skipped := RegisterDefaultWorkers(registry, catalog)
	if len(skipped) > 0 {
		t.Logf("惰性注册跳过: %v", skipped)
	}

	if registry.Count() == 0 {
		t.Fatal("Registry 为空的 Worker 数量为 0")
	}

	t.Logf("成功注册 %d 个 Worker:", registry.Count())
	for _, d := range registry.List() {
		t.Logf("  [%s] %s", string(d.Tier), d.Name)
	}
}

func TestRegistryLazyActivate(t *testing.T) {
	registry := NewRegistry()
	catalog := NewCatalog()

	d := Definition{
		Name:        "test-worker",
		Tier:        TierWork,
		Description: "测试 Worker",
		Outputs:     []Output{{Path: "test-output.md"}},
	}

	if err := catalog.Register(d); err != nil {
		t.Fatal(err)
	}

	// 尚未注册
	if registry.Get("test-worker") != nil {
		t.Fatal("不应先存在")
	}

	// 惰性激活
	activated := EnsureWorker(registry, catalog, "test-worker")
	if !activated {
		t.Fatal("应为首次激活")
	}
	if registry.Get("test-worker") == nil {
		t.Fatal("激活后应存在")
	}
}

func TestTierFiltering(t *testing.T) {
	registry := NewRegistry()
	registry.Register(Definition{Name: "w1", Tier: TierWork})
	registry.Register(Definition{Name: "w2", Tier: TierChecker})
	registry.Register(Definition{Name: "w3", Tier: TierWork})

	work := registry.ListByTier(TierWork)
	if len(work) != 2 {
		t.Fatalf("期望 2 个 Work Worker，得到 %d", len(work))
	}
}
