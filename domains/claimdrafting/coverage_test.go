package claimdrafting

import (
	"reflect"
	"testing"
)

func TestCoverageChecker_NormalFull(t *testing.T) {
	c := NewCoverageChecker()
	entries := []ClaimCoverageEntry{{
		ClaimID:        "claim_1",
		Features:       []string{"导电涂层", "散热结构"},
		EmbodimentRefs: []string{"实施例1记载导电涂层采用石墨烯", "实施例2记载散热结构为翅片"},
	}}
	rep := c.Check(nil, entries)
	if len(rep.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(rep.Items))
	}
	it := rep.Items[0]
	if !it.Valid {
		t.Fatalf("expected valid, reason=%s", it.InvalidReason)
	}
	if it.ActualCoverage != "full" {
		t.Errorf("expected full, got %s", it.ActualCoverage)
	}
	if rep.FullCount != 1 || rep.NoneCount != 0 {
		t.Errorf("full=%d none=%d", rep.FullCount, rep.NoneCount)
	}
}

func TestCoverageChecker_Partial(t *testing.T) {
	c := NewCoverageChecker()
	entries := []ClaimCoverageEntry{{
		ClaimID:        "claim_1",
		Features:       []string{"导电涂层", "散热结构"},
		EmbodimentRefs: []string{"实施例1记载导电涂层"},
	}}
	rep := c.Check(nil, entries)
	it := rep.Items[0]
	if it.ActualCoverage != "partial" {
		t.Errorf("expected partial, got %s", it.ActualCoverage)
	}
	if !reflect.DeepEqual(it.Uncovered, []string{"散热结构"}) {
		t.Errorf("uncovered=%v", it.Uncovered)
	}
}

func TestCoverageChecker_EmptyEmbodimentRefs(t *testing.T) {
	c := NewCoverageChecker()
	entries := []ClaimCoverageEntry{{
		ClaimID:        "claim_1",
		Features:       []string{"导电涂层", "散热结构"},
		EmbodimentRefs: nil,
	}}
	rep := c.Check(nil, entries)
	it := rep.Items[0]
	if it.ActualCoverage != "none" {
		t.Errorf("expected none for empty refs, got %s", it.ActualCoverage)
	}
	if len(it.Uncovered) != 2 {
		t.Errorf("expected 2 uncovered, got %v", it.Uncovered)
	}
	if rep.NoneCount != 1 {
		t.Errorf("expected NoneCount 1")
	}
}

func TestCoverageChecker_DuplicateFeatures(t *testing.T) {
	c := NewCoverageChecker()
	entries := []ClaimCoverageEntry{{
		ClaimID:        "claim_1",
		Features:       []string{"导电涂层", "导电涂层", "散热结构"},
		EmbodimentRefs: []string{"导电涂层", "散热结构"},
	}}
	rep := c.Check(nil, entries)
	it := rep.Items[0]
	if it.ActualCoverage != "full" {
		t.Errorf("expected full after dedupe, got %s", it.ActualCoverage)
	}
	if len(rep.Items[0].Uncovered) != 0 {
		t.Errorf("expected no uncovered after dedupe")
	}
}

func TestCoverageChecker_GapDetection(t *testing.T) {
	c := NewCoverageChecker()
	entries := []ClaimCoverageEntry{
		{ClaimID: "claim_1", Features: []string{"a"}, EmbodimentRefs: []string{"a"}},
		{ClaimID: "claim_3", Features: []string{"b"}, EmbodimentRefs: []string{"b"}},
	}
	rep := c.Check(nil, entries)
	if len(rep.Gaps) != 1 || rep.Gaps[0] != 2 {
		t.Errorf("expected gap 2, got %v", rep.Gaps)
	}
}

func TestCoverageChecker_ExceedMaxClaimNo(t *testing.T) {
	c := NewCoverageChecker()
	entries := []ClaimCoverageEntry{{
		ClaimID:  "claim_1001",
		Features: []string{"a"}, EmbodimentRefs: []string{"a"},
	}}
	rep := c.Check(nil, entries)
	if rep.Items[0].Valid {
		t.Errorf("expected invalid for claim_1001")
	}
	if rep.Items[0].InvalidReason == "" {
		t.Errorf("expected invalid reason")
	}
}

func TestCoverageChecker_NumExceedsClaimsCount(t *testing.T) {
	c := NewCoverageChecker()
	entries := []ClaimCoverageEntry{{
		ClaimID: "claim_5", Features: []string{"a"}, EmbodimentRefs: []string{"a"},
	}}
	rep := c.Check([]string{"c1", "c2"}, entries) // 只有 2 条权利要求
	if rep.Items[0].Valid {
		t.Errorf("expected invalid when claim num exceeds claims count")
	}
}
