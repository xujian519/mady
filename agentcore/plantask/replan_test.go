package plantask

import "testing"

func stepSnapshot(order int, desc string) StepSnapshot {
	s := StepSnapshot{Order: order, Strategy: "chain", Description: desc}
	s.Hash = StepHash(order, s.Strategy, desc)
	return s
}

// TestReplanMerge_KeepAllDone 场景 1：无反馈改动 → 全部保留 done。
func TestReplanMerge_KeepAllDone(t *testing.T) {
	old := []StepSnapshot{stepSnapshot(1, "检索"), stepSnapshot(2, "比对")}
	new := []StepSnapshot{stepSnapshot(1, "检索"), stepSnapshot(2, "比对")}
	completed := []string{old[0].Hash, old[1].Hash}

	skip, removed := ReplanMerge(old, completed, new, "")
	if len(skip) != 2 {
		t.Errorf("expected 2 skip, got %d", len(skip))
	}
	if len(removed) != 0 {
		t.Errorf("expected 0 removed, got %d", len(removed))
	}
}

// TestReplanMerge_PathChanged 场景 2：路径变更（新步骤哈希不一致）→ 移除完成标记。
func TestReplanMerge_PathChanged(t *testing.T) {
	old := []StepSnapshot{stepSnapshot(1, "检索"), stepSnapshot(2, "比对")}
	// 新计划：步骤 1 描述变了（哈希不一致），步骤 2 保留。
	new := []StepSnapshot{stepSnapshot(1, "检索（含美国同族）"), stepSnapshot(2, "比对")}
	completed := []string{old[0].Hash, old[1].Hash}

	skip, removed := ReplanMerge(old, completed, new, "")
	if _, ok := skip[old[0].Hash]; ok {
		t.Error("step 1 hash changed, must not be skipped")
	}
	if _, ok := skip[old[1].Hash]; !ok {
		t.Error("step 2 unchanged, must stay done")
	}
	if _, ok := removed[old[0].Hash]; !ok {
		t.Error("step 1 must be marked removed")
	}
	if len(skip) != 1 || len(removed) != 1 {
		t.Errorf("unexpected sets: skip=%d removed=%d", len(skip), len(removed))
	}
}

// TestReplanMerge_ExplicitRerun 场景 3：反馈显式重跑 → 移除完成标记。
func TestReplanMerge_ExplicitRerun(t *testing.T) {
	old := []StepSnapshot{stepSnapshot(1, "检索"), stepSnapshot(2, "比对")}
	new := []StepSnapshot{stepSnapshot(1, "检索"), stepSnapshot(2, "比对")}
	completed := []string{old[0].Hash, old[1].Hash}

	skip, removed := ReplanMerge(old, completed, new, "重跑:检索")
	if _, ok := skip[old[0].Hash]; ok {
		t.Error("step 1 explicitly rerun, must not be skipped")
	}
	if _, ok := skip[old[1].Hash]; !ok {
		t.Error("step 2 must stay done")
	}
	if _, ok := removed[old[0].Hash]; !ok {
		t.Error("step 1 must be in removed set")
	}
}

// TestReplanMerge_OrderTarget 场景 3b：按步骤序号重跑（重跑:step1 / 重跑:1）。
func TestReplanMerge_OrderTarget(t *testing.T) {
	old := []StepSnapshot{stepSnapshot(1, "检索"), stepSnapshot(2, "比对")}
	new := []StepSnapshot{stepSnapshot(1, "检索"), stepSnapshot(2, "比对")}
	completed := []string{old[0].Hash, old[1].Hash}

	skip, _ := ReplanMerge(old, completed, new, "重跑:step1")
	if _, ok := skip[old[0].Hash]; ok {
		t.Error("step1 must be rerun via 重跑:step1")
	}
	skip2, _ := ReplanMerge(old, completed, new, "重跑:2")
	if _, ok := skip2[old[1].Hash]; ok {
		t.Error("step2 must be rerun via 重跑:2")
	}
}

// TestReplanMerge_CompletedNotInOld 边界：完成标记对应步骤已不存在 → 忽略。
func TestReplanMerge_CompletedNotInOld(t *testing.T) {
	old := []StepSnapshot{stepSnapshot(1, "检索")}
	new := []StepSnapshot{stepSnapshot(1, "检索")}
	completed := []string{"ghost_hash"}

	skip, removed := ReplanMerge(old, completed, new, "")
	if len(skip) != 0 || len(removed) != 0 {
		t.Errorf("ghost completion must be ignored: skip=%d removed=%d", len(skip), len(removed))
	}
}

// TestParseRerunTargets 验证重跑语法解析（多行/逗号分隔）。
func TestParseRerunTargets(t *testing.T) {
	targets := parseRerunTargets("检索范围太窄\n重跑:检索,比对\n另加一步")
	if len(targets) != 2 || targets[0] != "检索" || targets[1] != "比对" {
		t.Errorf("unexpected targets: %v", targets)
	}
	if parseRerunTargets("无重跑语法") != nil {
		t.Error("expected nil targets without prefix")
	}
}
