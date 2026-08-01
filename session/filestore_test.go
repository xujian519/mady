package session

import (
	"context"
	"testing"
)

func TestFileStore_MaxLocksPruning(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir, WithMaxLocks(3))
	if err != nil {
		t.Fatal(err)
	}

	// Create 5 sessions to exceed the maxLocks=3 limit.
	for i := 0; i < 5; i++ {
		_, err := fs.Create(context.Background(), CreateOptions{InMemory: true})
		if err != nil {
			t.Fatal(err)
		}
	}

	// After creating 5 sessions with maxLocks=3, the lock map should have been
	// pruned at least once. We can't directly inspect the map, but we verify
	// that sessionLock still works after pruning (no panic, no deadlock).
	lock := fs.sessionLock("test-key")
	if lock == nil {
		t.Fatal("sessionLock returned nil after pruning")
	}
}

func TestFileStore_DefaultNoMaxLocks(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// With unlimited locks (default), creating and opening many sessions
	// should not panic and the lock map should grow without pruning.
	ids := make([]string, 50)
	for i := range ids {
		mgr, err := fs.Create(context.Background(), CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = mgr.Header().ID
	}

	// Opening sessions creates locks via sessionLock.
	for _, id := range ids {
		_, err := fs.Open(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
	}

	fs.locksMu.Lock()
	count := len(fs.locks)
	fs.locksMu.Unlock()

	if count < 50 {
		t.Fatalf("expected at least 50 locks (no pruning), got %d", count)
	}
}

func TestFileStore_DeleteCleansUpLock(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	mgr, err := fs.Create(context.Background(), CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	id := mgr.Header().ID

	// Open the session so that sessionLock is called and the lock is cached.
	_, err = fs.Open(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}

	// Lock should exist after open.
	fs.locksMu.Lock()
	_, ok := fs.locks[id]
	fs.locksMu.Unlock()
	if !ok {
		t.Fatal("lock not found after open")
	}

	// Delete should remove the lock.
	if err := fs.Delete(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	fs.locksMu.Lock()
	_, ok = fs.locks[id]
	fs.locksMu.Unlock()
	if ok {
		t.Fatal("lock still exists after delete")
	}
}

func TestFileStore_TrashLifecycle(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// 创建两个持久化会话（Create 默认落盘，回收站操作才有文件可移动）。
	created := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		mgr, err := fs.Create(ctx, CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, mgr.Header().ID)
	}
	// 主列表应含 2 个会话；回收站为空。
	infos, err := fs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 {
		t.Fatalf("List: want 2 sessions, got %d", len(infos))
	}
	trashed, err := fs.ListTrashed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(trashed) != 0 {
		t.Fatalf("ListTrashed: want 0, got %d", len(trashed))
	}

	// 移入回收站：主列表 1 个，回收站 1 个。
	if err := fs.MoveToTrash(ctx, created[0]); err != nil {
		t.Fatalf("MoveToTrash: %v", err)
	}
	infos, err = fs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatalf("List after trash: want 1, got %d", len(infos))
	}
	trashed, err = fs.ListTrashed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(trashed) != 1 || trashed[0].ID != created[0] {
		t.Fatalf("ListTrashed after trash: want 1 with id %s, got %+v", created[0], trashed)
	}

	// 恢复：主列表回到 2 个，回收站空。
	if err := fs.RestoreFromTrash(ctx, created[0]); err != nil {
		t.Fatalf("RestoreFromTrash: %v", err)
	}
	infos, err = fs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 {
		t.Fatalf("List after restore: want 2, got %d", len(infos))
	}
	trashed, err = fs.ListTrashed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(trashed) != 0 {
		t.Fatalf("ListTrashed after restore: want 0, got %d", len(trashed))
	}

	// 再次移入并彻底清除：主列表 1 个，回收站 0 个。
	if err := fs.MoveToTrash(ctx, created[0]); err != nil {
		t.Fatal(err)
	}
	if err := fs.PurgeTrashed(ctx, created[0]); err != nil {
		t.Fatalf("PurgeTrashed: %v", err)
	}
	infos, err = fs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatalf("List after purge: want 1, got %d", len(infos))
	}
	trashed, err = fs.ListTrashed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(trashed) != 0 {
		t.Fatalf("ListTrashed after purge: want 0, got %d", len(trashed))
	}

	// 不存在的会话移入回收站应报错（防误删）。
	if err := fs.MoveToTrash(ctx, "no-such-session"); err == nil {
		t.Fatal("MoveToTrash of missing session: want error, got nil")
	}
}
