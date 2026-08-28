package invariant

import (
	"errors"
	"strings"
	"testing"
)

func TestRunAll_ReportsViolations(t *testing.T) {
	// 注册两个检查，一个过一个不过；违规须带归属限定名上报。
	Register(Check{Package: "test", Name: "ok", Fn: func() error { return nil }})
	Register(Check{Package: "test", Name: "broken", Fn: func() error { return errors.New("关系被破坏") }})

	vs := RunAll()
	var found *Violation
	for i := range vs {
		if vs[i].Check.Name == "broken" {
			found = &vs[i]
		}
	}
	if found == nil {
		t.Fatal("expected violation for broken check")
	}
	if !strings.Contains(found.Error(), "test/broken") {
		t.Errorf("violation must report qualified name, got %s", found.Error())
	}
	if !errors.Is(found, found.Err) {
		t.Error("violation should unwrap to underlying error")
	}
}

func TestRunAll_DisablePatternSkips(t *testing.T) {
	t.Setenv("MADY_INVARIANTS_DISABLE", `^test/skipme$`)
	Register(Check{Package: "test", Name: "skipme", Fn: func() error { return errors.New("不应上报") }})

	for _, v := range RunAll() {
		if v.Check.Name == "skipme" {
			t.Error("disabled check must be skipped")
		}
	}
}

func TestRunAll_BadDisablePatternReported(t *testing.T) {
	t.Setenv("MADY_INVARIANTS_DISABLE", `(未闭合`)
	var sawConfigViolation bool
	for _, v := range RunAll() {
		if v.Check.Package == "invariant" && v.Check.Name == "disable-patterns" {
			sawConfigViolation = true
		}
	}
	if !sawConfigViolation {
		t.Error("bad disable pattern must be reported as a violation")
	}
}

func TestChecks_Snapshot(t *testing.T) {
	cs := Checks()
	if len(cs) == 0 {
		t.Fatal("expected registered checks from this and companion packages")
	}
	// 快照修改不应影响注册表。
	cs[0].Name = "mutated"
	if Checks()[0].Name == "mutated" {
		t.Error("Checks must return a snapshot, not the live slice")
	}
}
