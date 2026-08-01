//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTabStore_BasicLifecycle(t *testing.T) {
	ts := newTabStore("") // 内存模式（不持久化）

	if got := len(ts.List()); got != 1 {
		t.Fatalf("new store: want 1 default tab, got %d", got)
	}
	first := ts.List()[0]
	if ts.ActiveID() != first.ID {
		t.Fatalf("active: want %s, got %s", first.ID, ts.ActiveID())
	}

	second := ts.Create()
	if got := len(ts.List()); got != 2 {
		t.Fatalf("after create: want 2 tabs, got %d", got)
	}
	if ts.ActiveID() != second.ID {
		t.Fatalf("after create: active want %s, got %s", second.ID, ts.ActiveID())
	}

	// 激活回第一个标签
	if err := ts.Activate(first.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if ts.ActiveID() != first.ID {
		t.Fatalf("active after activate: want %s, got %s", first.ID, ts.ActiveID())
	}

	// 关闭激活标签（first，位于索引 0）→ 激活相邻（second）
	if err := ts.Close(first.ID); err != nil {
		t.Fatalf("close: %v", err)
	}
	if got := len(ts.List()); got != 1 {
		t.Fatalf("after close: want 1 tab, got %d", got)
	}
	if ts.ActiveID() != second.ID {
		t.Fatalf("active after close: want %s, got %s", second.ID, ts.ActiveID())
	}

	// 最后一个标签不可关闭
	if err := ts.Close(second.ID); err == nil {
		t.Fatal("close last tab: want error, got nil")
	}

	// 不存在的标签报错
	if err := ts.Activate("no-such-tab"); err == nil {
		t.Fatal("activate missing tab: want error, got nil")
	}
	if err := ts.Close("no-such-tab"); err == nil {
		t.Fatal("close missing tab: want error, got nil")
	}
}

func TestTabStore_PersistenceRoundTrip(t *testing.T) {
	home := t.TempDir()
	ts := newTabStore(home)
	created := ts.Create() // 第二个标签
	_ = ts.Create()        // 第三个标签
	_ = ts.Activate(created.ID)

	// 重新加载（模拟重启）
	ts2 := newTabStore(home)
	if got := len(ts2.List()); got != 3 {
		t.Fatalf("reloaded: want 3 tabs, got %d", got)
	}
	if ts2.ActiveID() != created.ID {
		t.Fatalf("reloaded active: want %s, got %s", created.ID, ts2.ActiveID())
	}

	// 关闭后持久化再加载
	if err := ts2.Close(created.ID); err != nil {
		t.Fatal(err)
	}
	ts3 := newTabStore(home)
	if got := len(ts3.List()); got != 2 {
		t.Fatalf("reloaded after close: want 2 tabs, got %d", got)
	}
}

func TestTabStore_CorruptFileFallsBackToDefault(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "desktop-tabs.json")
	if err := os.WriteFile(path, []byte("{corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	ts := newTabStore(home)
	// 坏文件被忽略 → 回退到默认单标签（不崩溃）
	if got := len(ts.List()); got != 1 {
		t.Fatalf("corrupt file: want 1 default tab, got %d", got)
	}
}

func TestTabStore_EmptyHomeIsMemoryOnly(t *testing.T) {
	ts := newTabStore("")
	_ = ts.Create()
	if ts.path != "" {
		t.Fatalf("memory mode: path should be empty, got %q", ts.path)
	}
	// 不落盘：文件不应存在（无可写路径，仅验证不 panic）
	ts.List()
}

func TestTabStore_GetAndSetThreadID(t *testing.T) {
	ts := newTabStore("") // 内存模式
	first := ts.List()[0]

	// Get 返回快照
	got, err := ts.Get(first.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != first.ID {
		t.Fatalf("Get: id mismatch %s != %s", got.ID, first.ID)
	}

	// SetThreadID 关联会话
	if err := ts.SetThreadID(first.ID, "thread-abc"); err != nil {
		t.Fatalf("SetThreadID: %v", err)
	}
	got, err = ts.Get(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ThreadID != "thread-abc" {
		t.Fatalf("SetThreadID: want thread-abc, got %q", got.ThreadID)
	}

	// 空 threadID / 不存在标签报错
	if err := ts.SetThreadID(first.ID, ""); err == nil {
		t.Fatal("SetThreadID empty: want error")
	}
	if err := ts.SetThreadID("no-such", "t"); err == nil {
		t.Fatal("SetThreadID missing tab: want error")
	}
	if _, err := ts.Get("no-such"); err == nil {
		t.Fatal("Get missing tab: want error")
	}
}
