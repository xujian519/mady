package main

import (
	"strings"
	"testing"
)

func TestTasklistDirForCWDPartitionsByCWD(t *testing.T) {
	base := "/tmp/mady/sessions"

	dir1 := tasklistDirForCWD(base, "/Users/xujian/projects/caseA")
	dir2 := tasklistDirForCWD(base, "/Users/xujian/projects/caseB")

	if dir1 == dir2 {
		t.Fatalf("tasklist dir should differ for different cwd: got %q and %q", dir1, dir2)
	}
	if !strings.HasPrefix(dir1, base+"/by-cwd/") {
		t.Fatalf("expected by-cwd partition prefix, got %q", dir1)
	}
	if !strings.HasSuffix(dir1, "/tasks") {
		t.Fatalf("expected /tasks suffix, got %q", dir1)
	}

	// 空 cwd 时回退到未分区旧目录
	dirEmpty := tasklistDirForCWD(base, "")
	if dirEmpty != base+"/tasks" {
		t.Fatalf("expected fallback dir %q, got %q", base+"/tasks", dirEmpty)
	}
}
