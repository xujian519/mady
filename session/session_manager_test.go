package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// ---------------------------------------------------------------------------
// Manager 创建与加载
// ---------------------------------------------------------------------------

func TestNewManager_CreateSession(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	mgr, err := fs.Create(ctx, CreateOptions{ID: "test-session-1", Cwd: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}

	h := mgr.Header()
	if h.ID != "test-session-1" {
		t.Errorf("header.ID = %q, want %q", h.ID, "test-session-1")
	}
	if h.Version != CurrentVersion {
		t.Errorf("header.Version = %d, want %d", h.Version, CurrentVersion)
	}
	if h.Cwd != "/tmp" {
		t.Errorf("header.Cwd = %q, want /tmp", h.Cwd)
	}
	if h.Type != EntryHeader {
		t.Errorf("header.Type = %q, want %q", h.Type, EntryHeader)
	}

	if leaf := mgr.LeafID(); leaf != "" {
		t.Errorf("expected empty leafID for new session, got %q", leaf)
	}
}

func TestNewManager_LoadSession(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	mgr, err := fs.Create(ctx, CreateOptions{ID: "load-test", Cwd: "/home"})
	if err != nil {
		t.Fatal(err)
	}

	msg1 := agentcore.Message{Role: agentcore.RoleUser, Content: "hello"}
	msg2 := agentcore.Message{Role: agentcore.RoleAssistant, Content: "world"}
	if err := mgr.AppendMessage(ctx, msg1); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AppendMessage(ctx, msg2); err != nil {
		t.Fatal(err)
	}

	loaded, err := fs.Open(ctx, "load-test")
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Header().ID != "load-test" {
		t.Errorf("loaded header.ID = %q", loaded.Header().ID)
	}

	msgs := loaded.MessagesOnPath()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "hello" || msgs[1].Content != "world" {
		t.Errorf("messages mismatch: %+v", msgs)
	}
}

// ---------------------------------------------------------------------------
// Append rollback — 持久化失败时恢复内存状态
// ---------------------------------------------------------------------------

func TestAppendRollback_PersistFail(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	mgr, err := fs.Create(ctx, CreateOptions{ID: "rollback-test", Cwd: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}

	// Append a first message successfully to trigger lazy flush.
	msg1 := agentcore.Message{Role: agentcore.RoleUser, Content: "first"}
	if err := mgr.AppendMessage(ctx, msg1); err != nil {
		t.Fatal(err)
	}
	initialLeaf := mgr.LeafID()
	_ = initialLeaf

	// Now cause a persist failure by making the session directory unwritable.
	// We'll set the manager's filePath to a non-existent directory.
	// Since Manager's fields are private, we'll achieve this by corrupting
	// the directory permission on the JSONL file.
	sessionFile := mgr.filePath
	if sessionFile == "" {
		t.Fatal("expected non-empty filePath after flush")
	}

	// Remove the file so the append fails (os.OpenFile with O_APPEND on non-existent
	// file with O_CREATE should work, but let's make the parent read-only).
	parentDir := filepath.Dir(sessionFile)
	if err := os.Chmod(parentDir, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parentDir, 0o755) })

	// This Append should fail persistence and trigger rollback.
	err = mgr.AppendMessage(ctx, agentcore.Message{Role: agentcore.RoleAssistant, Content: "should-rollback"})
	if err == nil {
		t.Fatal("expected error from Append with unwritable directory, got nil")
	}

	// Verify rollback: leafID should be unchanged.
	afterLeaf := mgr.LeafID()
	if afterLeaf != initialLeaf {
		t.Errorf("leafID changed after rollback: was %q, now %q", initialLeaf, afterLeaf)
	}

	// Verify the entry was not added.
	entries := mgr.Entries()
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after rollback, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// 版本迁移 v1→v4
// ---------------------------------------------------------------------------

func TestVersionMigrationV1ToV4(t *testing.T) {
	dir := t.TempDir()
	sessionFile := filepath.Join(dir, "migrate-test.jsonl")

	// Write a v1-format session file (no header, minimal entries).
	// In v1, the data field is a raw JSON object (not string-encoded).
	v1Data := `{"type":"message","data":{"role":"user","content":"hello"},"timestamp":"2024-01-01T00:00:00Z"}
{"type":"message","data":{"role":"assistant","content":"hi"},"timestamp":"2024-01-01T00:00:01Z"}
{"type":"message","data":{"role":"hookMessage","content":"old"},"timestamp":"2024-01-01T00:00:02Z"}
`
	if err := os.WriteFile(sessionFile, []byte(v1Data), 0o600); err != nil {
		t.Fatal(err)
	}

	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	mgr, err := fs.Open(context.Background(), "migrate-test")
	if err != nil {
		t.Fatal(err)
	}

	if mgr.Header().Version != CurrentVersion {
		t.Errorf("header version = %d, want %d", mgr.Header().Version, CurrentVersion)
	}

	entries := mgr.Entries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// All entries should be at v4.
	for i, e := range entries {
		if e.Version != CurrentVersion {
			t.Errorf("entry[%d].Version = %d, want %d", i, e.Version, CurrentVersion)
		}
		if e.ID == "" {
			t.Errorf("entry[%d] missing ID after migration", i)
		}
	}

	// The hookMessage role should have been migrated to "custom".
	var msgData map[string]any
	if err := json.Unmarshal(entries[2].Data, &msgData); err != nil {
		t.Fatalf("unmarshal message data: %v (raw: %s)", err, string(entries[2].Data))
	}
	if role, ok := msgData["role"].(string); !ok || role != "custom" {
		t.Errorf("migrated hookMessage role = %q, want %q", role, "custom")
	}
}

func TestVersionMigrationV1NoHeader(t *testing.T) {
	dir := t.TempDir()
	sessionFile := filepath.Join(dir, "no-header.jsonl")

	// v1 format without any header entry — Open should auto-detect.
	v1Data := `{"type":"message","data":{"role":"user","content":"test"},"timestamp":"2024-01-01T00:00:00Z"}
`
	if err := os.WriteFile(sessionFile, []byte(v1Data), 0o600); err != nil {
		t.Fatal(err)
	}

	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	mgr, err := fs.Open(context.Background(), "no-header")
	if err != nil {
		t.Fatal(err)
	}

	if mgr.Header().Version != CurrentVersion {
		t.Errorf("header version = %d, want %d", mgr.Header().Version, CurrentVersion)
	}

	msgs := mgr.MessagesOnPath()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "test" {
		t.Errorf("message content = %q", msgs[0].Content)
	}
}

// ---------------------------------------------------------------------------
// Branch / CreateBranchedSession
// ---------------------------------------------------------------------------

func TestBranch_Basic(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	mgr, err := fs.Create(ctx, CreateOptions{InMemory: true})
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.AppendMessage(ctx, agentcore.Message{Role: agentcore.RoleUser, Content: "m1"}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AppendMessage(ctx, agentcore.Message{Role: agentcore.RoleAssistant, Content: "r1"}); err != nil {
		t.Fatal(err)
	}
	midLeaf := mgr.LeafID()

	if err := mgr.AppendMessage(ctx, agentcore.Message{Role: agentcore.RoleUser, Content: "m2"}); err != nil {
		t.Fatal(err)
	}

	// Branch back to midLeaf.
	if err := mgr.Branch(midLeaf); err != nil {
		t.Fatal(err)
	}
	if mgr.LeafID() != midLeaf {
		t.Errorf("leafID after branch = %q, want %q", mgr.LeafID(), midLeaf)
	}

	// Entries should still exist after branching.
	entries := mgr.Entries()
	if len(entries) != 3 {
		t.Errorf("expected 3 entries after branch, got %d", len(entries))
	}
}

func TestBranch_InvalidID(t *testing.T) {
	mgr := newManager(Header{}, "", false)
	if err := mgr.Branch("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent branch target")
	}
}

func TestCreateBranchedSession(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	mgr, err := fs.Create(ctx, CreateOptions{ID: "original", Cwd: "/proj"})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 4; i++ {
		msg := agentcore.Message{Role: agentcore.RoleUser, Content: string(rune('a' + i))}
		if err := mgr.AppendMessage(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}

	// Get first entry ID to branch from.
	entries := mgr.Entries()
	branchFromID := entries[1].ID

	branchID, err := mgr.CreateBranchedSession(ctx, fs)
	if err != nil {
		t.Fatal(err)
	}
	if branchID == "" {
		t.Fatal("expected non-empty branch session ID")
	}
	if branchID == "original" {
		t.Errorf("branch ID should differ from original")
	}

	// Branch back and create another branch.
	if err := mgr.Branch(branchFromID); err != nil {
		t.Fatal(err)
	}

	branchID2, err := mgr.CreateBranchedSession(ctx, fs)
	if err != nil {
		t.Fatal(err)
	}
	if branchID2 == branchID {
		t.Errorf("second branch should have different ID")
	}

	// Verify the branch is loadable.
	branchMgr, err := fs.Open(ctx, branchID2)
	if err != nil {
		t.Fatal(err)
	}
	branchMsgs := branchMgr.MessagesOnPath()
	if len(branchMsgs) != 2 {
		t.Fatalf("expected 2 messages in branched session, got %d", len(branchMsgs))
	}
}

// ---------------------------------------------------------------------------
// MessagesOnPath 压缩感知
// ---------------------------------------------------------------------------

func TestMessagesOnPath_WithCompaction(t *testing.T) {
	mgr := newManager(Header{}, "", false)

	ctx := context.Background()

	// Add messages.
	msgs := []agentcore.Message{
		{Role: agentcore.RoleUser, Content: "msg1"},
		{Role: agentcore.RoleAssistant, Content: "res1"},
		{Role: agentcore.RoleUser, Content: "msg2"},
		{Role: agentcore.RoleAssistant, Content: "res2"},
		{Role: agentcore.RoleUser, Content: "msg3"},
		{Role: agentcore.RoleAssistant, Content: "res3"},
	}
	for _, msg := range msgs {
		if err := mgr.AppendMessage(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}

	// The first 4 entries have IDs we can reference.
	entries := mgr.Entries()
	firstKeptID := entries[4].ID // "msg3" entry

	// Append a compaction entry that summarizes [0:4) and keeps from entry 4.
	if err := mgr.AppendCompaction(ctx, CompactionData{
		Summary:          "上下文压缩摘要: msg1, res1, msg2, res2 已被压缩",
		FirstKeptEntryID: firstKeptID,
		KeptCount:        2,
	}); err != nil {
		t.Fatal(err)
	}

	// Append more messages after compaction.
	if err := mgr.AppendMessage(ctx, agentcore.Message{Role: agentcore.RoleUser, Content: "msg4"}); err != nil {
		t.Fatal(err)
	}

	// MessagesOnPath should return: summary system message + kept messages + new messages.
	pathMsgs := mgr.MessagesOnPath()
	if len(pathMsgs) < 3 {
		t.Fatalf("expected at least 3 messages on path (summary + 2 kept + new), got %d", len(pathMsgs))
	}

	// First message should be the compaction summary.
	if pathMsgs[0].Type != agentcore.MessageTypeCompactionSummary {
		t.Errorf("expected compaction summary, got type %q", pathMsgs[0].Type)
	}
	if !strings.Contains(pathMsgs[0].Content, "上下文压缩摘要") {
		t.Errorf("summary content missing expected text: %s", pathMsgs[0].Content)
	}

	// The last message should be msg4.
	lastMsg := pathMsgs[len(pathMsgs)-1]
	if lastMsg.Content != "msg4" {
		t.Errorf("last message = %q, want %q", lastMsg.Content, "msg4")
	}
}

func TestMessagesOnPath_WithBranchSummary(t *testing.T) {
	mgr := newManager(Header{}, "", false)
	ctx := context.Background()

	if err := mgr.AppendMessage(ctx, agentcore.Message{Role: agentcore.RoleUser, Content: "main"}); err != nil {
		t.Fatal(err)
	}

	// Append a branch summary entry.
	if err := mgr.AppendBranchSummary(ctx, BranchSummaryData{
		Summary:  "分支摘要: 实验性路径探索",
		BranchID: "branch-1",
	}); err != nil {
		t.Fatal(err)
	}

	pathMsgs := mgr.MessagesOnPath()

	// Should find the branch summary message.
	foundBranchSummary := false
	for _, msg := range pathMsgs {
		if msg.Type == agentcore.MessageTypeBranchSummary &&
			strings.Contains(msg.Content, "分支摘要") {
			foundBranchSummary = true
			break
		}
	}
	if !foundBranchSummary {
		t.Errorf("expected branch summary in MessagesOnPath, got: %+v", pathMsgs)
	}
}

func TestMessagesOnPath_MultipleCompactions(t *testing.T) {
	mgr := newManager(Header{}, "", false)
	ctx := context.Background()

	// Add messages and compact twice.
	for i := 0; i < 4; i++ {
		if err := mgr.AppendMessage(ctx, agentcore.Message{
			Role:    agentcore.RoleUser,
			Content: string(rune('A' + i)),
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Compaction 1: keep the last message.
	entries := mgr.Entries()
	firstKept1 := entries[3].ID
	if err := mgr.AppendCompaction(ctx, CompactionData{
		Summary:          "第一次压缩",
		FirstKeptEntryID: firstKept1,
		KeptCount:        1,
	}); err != nil {
		t.Fatal(err)
	}

	// Add more messages.
	for i := 0; i < 3; i++ {
		if err := mgr.AppendMessage(ctx, agentcore.Message{
			Role:    agentcore.RoleUser,
			Content: string(rune('D' + i)),
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Compaction 2: keep the last two messages.
	entries2 := mgr.Entries()
	firstKept2 := entries2[len(entries2)-2].ID
	if err := mgr.AppendCompaction(ctx, CompactionData{
		Summary:          "第二次压缩",
		FirstKeptEntryID: firstKept2,
		KeptCount:        2,
	}); err != nil {
		t.Fatal(err)
	}

	pathMsgs := mgr.MessagesOnPath()

	// Only the latest compaction summary should be used.
	lastCompactionIdx := -1
	for i, msg := range pathMsgs {
		if msg.Type == agentcore.MessageTypeCompactionSummary {
			lastCompactionIdx = i
		}
	}
	if lastCompactionIdx < 0 {
		t.Fatal("expected at least one compaction summary")
	}

	// The last compaction summary should be from the second compaction.
	if !strings.Contains(pathMsgs[lastCompactionIdx].Content, "第二次压缩") {
		t.Errorf("expected second compaction summary, got: %s", pathMsgs[lastCompactionIdx].Content)
	}
}

// ---------------------------------------------------------------------------
// pathCache 行为验证
// ---------------------------------------------------------------------------

func TestPathCache_InvalidateOnAppend(t *testing.T) {
	mgr := newManager(Header{}, "", false)
	ctx := context.Background()

	// After creation, cache should be nil.
	if pc := mgr.pathCache.Load(); pc != nil {
		t.Error("expected nil pathCache after creation")
	}

	// Add an entry so the leaf is non-empty and pathToLeaf will build the cache.
	if err := mgr.AppendMessage(ctx, agentcore.Message{Role: agentcore.RoleUser, Content: "first"}); err != nil {
		t.Fatal(err)
	}

	// Reading path should build cache.
	_ = mgr.MessagesOnPath()
	pc1 := mgr.pathCache.Load()
	if pc1 == nil {
		t.Error("expected non-nil pathCache after pathToLeaf call")
	}

	// Appending should invalidate cache.
	if err := mgr.AppendMessage(ctx, agentcore.Message{Role: agentcore.RoleUser, Content: "second"}); err != nil {
		t.Fatal(err)
	}
	if pc := mgr.pathCache.Load(); pc != nil {
		t.Error("expected nil pathCache after Append (invalidated)")
	}

	// Another read should rebuild.
	_ = mgr.MessagesOnPath()
	pc2 := mgr.pathCache.Load()
	if pc2 == nil {
		t.Error("expected non-nil pathCache after second read")
	}
	_ = pc1
	_ = pc2
}

func TestPathCache_ConcurrentReads(t *testing.T) {
	mgr := newManager(Header{}, "", false)

	// Build some entries.
	for i := 0; i < 10; i++ {
		id := mgr.generateID()
		mgr.entries = append(mgr.entries, Entry{
			ID:       id,
			ParentID: mgr.leafID,
			Type:     EntryMessage,
			Version:  CurrentVersion,
		})
		mgr.index[id] = &mgr.entries[len(mgr.entries)-1]
		mgr.leafID = id
	}
	mgr.invalidatePathCache()

	// Concurrent reads.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.mu.RLock()
			_ = mgr.pathToLeaf()
			mgr.mu.RUnlock()
		}()
	}
	wg.Wait()

	// Cache should have been built.
	pc := mgr.pathCache.Load()
	if pc == nil {
		t.Error("expected non-nil pathCache after concurrent reads")
	}
}

func TestPathCache_LeafID(t *testing.T) {
	mgr := newManager(Header{}, "", false)

	// Empty session: pathToLeaf returns nil.
	if path := func() []Entry {
		mgr.mu.RLock()
		defer mgr.mu.RUnlock()
		return mgr.pathToLeaf()
	}(); path != nil {
		t.Errorf("expected nil path for empty session, got %d entries", len(path))
	}

	// Add one entry.
	mgr.entries = append(mgr.entries, Entry{
		ID:      "root",
		Type:    EntryMessage,
		Version: CurrentVersion,
	})
	mgr.index["root"] = &mgr.entries[0]
	mgr.leafID = "root"
	mgr.invalidatePathCache()

	path := func() []Entry {
		mgr.mu.RLock()
		defer mgr.mu.RUnlock()
		return mgr.pathToLeaf()
	}()
	if len(path) != 1 || path[0].ID != "root" {
		t.Errorf("expected path [root], got %+v", path)
	}
}

// ---------------------------------------------------------------------------
// Header, LeafID, Entries, Info 基本操作
// ---------------------------------------------------------------------------

func TestManager_HeaderAndLeaf(t *testing.T) {
	h := Header{ID: "h1", Version: 4, Cwd: "/test"}
	mgr := newManager(h, "", false)
	if mgr.Header().ID != "h1" {
		t.Errorf("Header().ID = %q", mgr.Header().ID)
	}
	if mgr.LeafID() != "" {
		t.Errorf("expected empty leaf, got %q", mgr.LeafID())
	}
}

func TestManager_Entries(t *testing.T) {
	mgr := newManager(Header{ID: "test"}, "", false)
	if len(mgr.Entries()) != 0 {
		t.Errorf("expected empty entries")
	}
}

func TestManager_Info(t *testing.T) {
	mgr := newManager(Header{ID: "info-test", Version: 4}, "", false)

	// Empty session: info should still have ID.
	info := mgr.Info()
	if info.ID != "info-test" {
		t.Errorf("info.ID = %q", info.ID)
	}
	if info.MessageCount != 0 {
		t.Errorf("expected 0 messages, got %d", info.MessageCount)
	}
}

// ---------------------------------------------------------------------------
// SetLabel / GetLabel
// ---------------------------------------------------------------------------

func TestManager_SetLabel(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	mgr, err := fs.Create(ctx, CreateOptions{InMemory: true})
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.AppendMessage(ctx, agentcore.Message{Role: agentcore.RoleUser, Content: "test"}); err != nil {
		t.Fatal(err)
	}

	entries := mgr.Entries()
	targetID := entries[0].ID

	// Set label.
	if err := mgr.SetLabel(ctx, targetID, "important"); err != nil {
		t.Fatal(err)
	}
	if label := mgr.GetLabel(targetID); label != "important" {
		t.Errorf("GetLabel = %q, want %q", label, "important")
	}
}

func TestManager_RemoveLabel(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	mgr, err := fs.Create(ctx, CreateOptions{InMemory: true})
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.AppendMessage(ctx, agentcore.Message{Role: agentcore.RoleUser, Content: "t"}); err != nil {
		t.Fatal(err)
	}

	entries := mgr.Entries()
	targetID := entries[0].ID

	if err := mgr.SetLabel(ctx, targetID, "important"); err != nil {
		t.Fatal(err)
	}
	// Remove label by setting empty string.
	if err := mgr.SetLabel(ctx, targetID, ""); err != nil {
		t.Fatal(err)
	}
	if label := mgr.GetLabel(targetID); label != "" {
		t.Errorf("expected empty label after removal, got %q", label)
	}
}

// ---------------------------------------------------------------------------
// generateID
// ---------------------------------------------------------------------------

func TestGenerateID(t *testing.T) {
	mgr := newManager(Header{}, "", false)
	id1 := mgr.generateID()
	id2 := mgr.generateID()
	if id1 == id2 {
		t.Error("generateID should produce unique IDs")
	}
	if id1 == "" || id2 == "" {
		t.Error("generateID should not return empty string")
	}
}

// ---------------------------------------------------------------------------
// 并发安全
// ---------------------------------------------------------------------------

func TestManager_ConcurrentAppend(t *testing.T) {
	mgr := newManager(Header{ID: "concurrent"}, "", false)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			msg := agentcore.Message{Role: agentcore.RoleUser, Content: string(rune('a' + n))}
			mgr.AppendMessage(ctx, msg) //nolint:errcheck
		}(i)
	}
	wg.Wait()

	entries := mgr.Entries()
	if len(entries) != 20 {
		t.Errorf("expected 20 entries after concurrent appends, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// InMemory session
// ---------------------------------------------------------------------------

func TestManager_InMemorySession(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	mgr, err := fs.Create(ctx, CreateOptions{InMemory: true})
	if err != nil {
		t.Fatal(err)
	}

	// InMemory sessions have no file path.
	if mgr.filePath != "" {
		t.Errorf("InMemory session should not have a file path, got %q", mgr.filePath)
	}

	// Appending should work without persistence.
	if err := mgr.AppendMessage(ctx, agentcore.Message{Role: agentcore.RoleUser, Content: "test"}); err != nil {
		t.Fatal(err)
	}

	if len(mgr.Entries()) != 1 {
		t.Errorf("expected 1 entry, got %d", len(mgr.Entries()))
	}
}

// ---------------------------------------------------------------------------
// Entry with explicit ID and ParentID
// ---------------------------------------------------------------------------

func TestManager_AppendWithExplicitIDs(t *testing.T) {
	mgr := newManager(Header{ID: "explicit"}, "", false)
	ctx := context.Background()

	entry := Entry{
		ID:       "custom-id",
		ParentID: "custom-parent",
		Type:     EntryCustom,
		Data:     json.RawMessage(`{"key":"val"}`),
	}
	if err := mgr.Append(ctx, entry); err != nil {
		t.Fatal(err)
	}

	if mgr.LeafID() != "custom-id" {
		t.Errorf("leafID = %q, want %q", mgr.LeafID(), "custom-id")
	}

	entries := mgr.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ID != "custom-id" {
		t.Errorf("entry.ID = %q", entries[0].ID)
	}
	if entries[0].ParentID != "custom-parent" {
		t.Errorf("entry.ParentID = %q", entries[0].ParentID)
	}
}

// ---------------------------------------------------------------------------
// MessagesOnPath 空会话
// ---------------------------------------------------------------------------

func TestMessagesOnPath_EmptySession(t *testing.T) {
	mgr := newManager(Header{}, "", false)
	msgs := mgr.MessagesOnPath()
	if len(msgs) != 0 {
		t.Errorf("expected empty messages, got %d", len(msgs))
	}
}
