//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agui"
	"github.com/xujian519/mady/bootstrap"
	madyserver "github.com/xujian519/mady/server"
)

func TestToKebabCase_RunStarted(t *testing.T) {
	got := toKebabCase("RUN_STARTED")
	if got != "run-started" {
		t.Errorf("toKebabCase(RUN_STARTED) = %q, want %q", got, "run-started")
	}
}

func TestToKebabCase_handoff_start(t *testing.T) {
	got := toKebabCase("handoff_start")
	if got != "handoff-start" {
		t.Errorf("toKebabCase(handoff_start) = %q, want %q", got, "handoff-start")
	}
}

func TestToKebabCase_empty(t *testing.T) {
	if got := toKebabCase(""); got != "" {
		t.Errorf("toKebabCase('') = %q, want ''", got)
	}
}

func TestMapAguiEvent_RunStarted(t *testing.T) {
	ev := agui.RunStartedEvent{
		BaseEvent: agui.BaseEvent{
			Type: agui.EventRunStarted,
		},
		ThreadID: "th-1",
		RunID:    "run-1",
	}
	name := mapAguiEventToWailsName(ev)
	if name != "agui:agent-start" {
		t.Errorf("mapAguiEventToWailsName(RunStartedEvent) = %q, want %q", name, "agui:agent-start")
	}
}

func TestMapAguiEvent_Custom(t *testing.T) {
	ev := agui.CustomEvent{
		BaseEvent: agui.BaseEvent{Type: agui.EventCustom},
		Name:      "a2ui",
		Value:     map[string]any{"kind": "createSurface"},
	}
	name := mapAguiEventToWailsName(ev)
	if name != "agui:a2ui" {
		t.Errorf("mapAguiEventToWailsName(CustomEvent{a2ui}) = %q, want %q", name, "agui:a2ui")
	}
}

func TestMapAguiEvent_HandoffEnd(t *testing.T) {
	ev := agui.CustomEvent{
		BaseEvent: agui.BaseEvent{Type: agui.EventCustom},
		Name:      "handoff_end",
		Value:     map[string]any{},
	}
	name := mapAguiEventToWailsName(ev)
	if name != "agui:handoff-end" {
		t.Errorf("mapAguiEventToWailsName(CustomEvent{handoff_end}) = %q, want %q", name, "agui:handoff-end")
	}
}

func TestMapAguiEvent_DefaultFallback(t *testing.T) {
	// 未显式处理的类型应通过 eventTypeOf 回退
	ev := agui.StepStartedEvent{
		BaseEvent: agui.BaseEvent{Type: agui.EventStepStarted},
		StepName:  "turn_1",
	}
	name := mapAguiEventToWailsName(ev)
	if name != "agui:step-started" {
		t.Errorf("mapAguiEventToWailsName(StepStartedEvent) = %q, want %q", name, "agui:step-started")
	}
}

func TestEventTypeOf(t *testing.T) {
	ev := agui.RunFinishedEvent{
		BaseEvent: agui.BaseEvent{
			Type:      agui.EventRunFinished,
			Timestamp: float64(time.Now().UnixMilli()),
		},
		ThreadID: "th-1",
		RunID:    "run-1",
	}
	typ := eventTypeOf(ev)
	if typ != agui.EventRunFinished {
		t.Errorf("eventTypeOf(RunFinishedEvent) = %q, want %q", typ, agui.EventRunFinished)
	}
}

func TestGenerateRunID(t *testing.T) {
	id := generateRunID()
	if len(id) < 10 {
		t.Errorf("generateRunID() = %q, too short", id)
	}
	// 格式验证：run-<unix_nano>
	if len(id) < 10 || id[:4] != "run-" {
		t.Errorf("generateRunID() = %q, should start with 'run-'", id)
	}
}

// ── 事件映射集成测试 ──────────────────────────────
//
// 验证 agentcore.Event → agui.Converter.Convert → mapAguiEventToWailsName
// 整条链路的正确性。

func TestAgentEventToWailsName(t *testing.T) {
	converter := agui.NewConverter("th-test", "run-test")

	tests := []struct {
		name  string
		event agentcore.Event
		want  string // 期望的 Wails 事件名
	}{
		{
			name:  "AgentStartEvent → agui:agent-start",
			event: &agentcore.AgentStartEvent{},
			want:  "agui:agent-start",
		},
		{
			name:  "MessageDeltaEvent → agui:message-delta",
			event: &agentcore.MessageDeltaEvent{Delta: "hello"},
			want:  "agui:message-delta",
		},
		{
			name:  "ToolCallStartEvent → agui:tool-call-start",
			event: &agentcore.ToolCallStartEvent{ToolCall: agentcore.ToolCall{ID: "tc-1", Name: "test"}},
			want:  "agui:tool-call-start",
		},
		{
			name:  "HandoffStartEvent → agui:handoff-start",
			event: &agentcore.HandoffStartEvent{TargetAgent: "patent"},
			want:  "agui:handoff-start",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aguiEvents := converter.Convert(tt.event)
			if len(aguiEvents) == 0 {
				t.Fatal("expected at least one AGUI event")
			}
			// 遍历所有 AGUI 事件，检查任一匹配
			for _, ev := range aguiEvents {
				got := mapAguiEventToWailsName(ev)
				if got == tt.want {
					return // 匹配成功
				}
			}
			// 未匹配：列出所有实际事件名
			var names []string
			for _, ev := range aguiEvents {
				names = append(names, mapAguiEventToWailsName(ev))
			}
			t.Errorf("no event matched %q; got %v", tt.want, names)
		})
	}
}

func TestEventMappingWithConverter_ThinkingThenText(t *testing.T) {
	converter := agui.NewConverter("th-1", "run-1")

	// Thinking delta → agui:thinking-delta
	thinkingEvents := converter.Convert(&agentcore.MessageDeltaEvent{
		Delta: "thinking step 1",
		Kind:  agentcore.BlockKindThinking,
	})
	found := false
	for _, ev := range thinkingEvents {
		if mapAguiEventToWailsName(ev) == "agui:thinking-delta" {
			found = true
			break
		}
	}
	if !found {
		t.Error("thinking delta event did not produce agui:thinking-delta")
	}

	// Text delta after thinking → agui:message-delta
	converter2 := agui.NewConverter("th-1", "run-1")
	textEvents := converter2.Convert(&agentcore.MessageDeltaEvent{
		Delta: "answer text",
		Kind:  agentcore.BlockKindText,
	})
	found = false
	for _, ev := range textEvents {
		if mapAguiEventToWailsName(ev) == "agui:message-delta" {
			found = true
			break
		}
	}
	if !found {
		t.Error("text delta event did not produce agui:message-delta")
	}
}

func TestEventMapping_HandoffInvisible(t *testing.T) {
	// Invisible Handoff 事件在前端必须静默过滤（由 client.ts 决策）
	// 此处验证映射产生正确的 Wails 事件名，前端 bridge 据此决定不渲染。
	converter := agui.NewConverter("th-1", "run-1")

	events := converter.Convert(&agentcore.HandoffStartEvent{
		TargetAgent: "patent",
	})
	if len(events) == 0 {
		t.Fatal("expected at least one event for HandoffStartEvent")
	}
	name := mapAguiEventToWailsName(events[0])
	if name != "agui:handoff-start" {
		t.Errorf("HandoffStart → Wails event = %q, want %q", name, "agui:handoff-start")
	}
}

func TestEventMapping_CustomEventA2UI(t *testing.T) {
	converter := agui.NewConverter("th-1", "run-1")

	// 模拟发送 A2UI 事件的 CustomEvent
	customAgui := agui.CustomEvent{
		BaseEvent: agui.BaseEvent{Type: agui.EventCustom},
		Name:      "a2ui",
		Value:     map[string]any{"kind": "createSurface"},
	}
	name := mapAguiEventToWailsName(customAgui)
	if name != "agui:a2ui" {
		t.Errorf("CustomEvent{a2ui} → %q, want %q", name, "agui:a2ui")
	}

	// 检查 converter 是否产生了 TextMessageEnd
	endEvents := converter.Convert(&agentcore.AgentEndEvent{Output: "done"})
	hasEnd := false
	for _, ev := range endEvents {
		n := mapAguiEventToWailsName(ev)
		if n == "agui:run-finished" || n == "agui:run-finished-with-success" {
			hasEnd = true
		}
	}
	if !hasEnd {
		t.Error("AgentEndEvent did not produce a run-finished event")
	}
}

// --- T5.1 ReadFile 辅助函数测试 ---

func TestClassifyFileKind(t *testing.T) {
	cases := []struct {
		name     string
		wantKind string
		wantMime string
	}{
		{"README.md", "md", "text/markdown"},
		{"notes.markdown", "md", "text/markdown"},
		{"spec.PDF", "pdf", "application/pdf"},
		{"scan.pdf", "pdf", "application/pdf"},
		{"photo.png", "image", "image/png"},
		{"photo.JPG", "image", "image/jpeg"},
		{"icon.svg", "image", "image/svg+xml"},
		{"anim.webp", "image", "image/webp"},
		{"main.go", "text", "text/plain"},
		{"config.yaml", "text", "text/plain"},
		{"noext", "text", "text/plain"},
	}
	for _, c := range cases {
		kind, mime := classifyFileKind(c.name)
		if kind != c.wantKind || mime != c.wantMime {
			t.Errorf("classifyFileKind(%q) = (%q, %q), want (%q, %q)", c.name, kind, mime, c.wantKind, c.wantMime)
		}
	}
}

func TestIsBinaryContent(t *testing.T) {
	if isBinaryContent([]byte("hello world\n这是文本")) {
		t.Error("plain text detected as binary")
	}
	if !isBinaryContent([]byte{'P', 'K', 0, 1, 2}) {
		t.Error("NUL-containing content not detected as binary")
	}
	// 超过 8KB 后的 NUL 不影响判定（嗅探窗口外）
	long := make([]byte, 9000)
	for i := range long {
		long[i] = 'a'
	}
	long[8999] = 0
	if isBinaryContent(long) {
		t.Error("NUL beyond sniff window should not mark content as binary")
	}
}

func TestResolveSandboxedPath(t *testing.T) {
	root := t.TempDir()

	// 正常相对路径
	abs, err := resolveSandboxedPath("docs/spec.md", root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isPathWithinSandbox(abs, root) {
		t.Errorf("resolved path %q escaped sandbox %q", abs, root)
	}

	// 空路径
	if _, err := resolveSandboxedPath("", root); err == nil {
		t.Error("empty path should be rejected")
	}

	// 越狱路径
	escapes := []string{"../outside.txt", "../../etc/passwd", "sub/../../escape.txt"}
	for _, p := range escapes {
		if _, err := resolveSandboxedPath(p, root); err == nil {
			t.Errorf("escape path %q should be rejected", p)
		}
	}
}

// ── resolveSandboxedPathMulti (T5.x gap fix) ─────

func TestResolveSandboxedPathMulti_firstSandbox(t *testing.T) {
	root := t.TempDir()
	// 第一个沙箱内有文件时应返回
	abs, err := resolveSandboxedPathMulti("a.txt", root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isPathWithinSandbox(abs, root) {
		t.Errorf("abs %q not within sandbox %q", abs, root)
	}
}

func TestResolveSandboxedPathMulti_fallbackSandbox(t *testing.T) {
	// 绝对路径在 r1 沙箱外（被拒）但在 r2 沙箱内（通过）
	r1 := t.TempDir()
	r2 := t.TempDir()

	target := filepath.Join(r2, "file.md")
	abs, err := resolveSandboxedPathMulti(target, r1, r2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// r1 为随机目录名，target 是 r2 内的绝对路径，不可能是 r1 的子路径
	if !isPathWithinSandbox(abs, r2) {
		t.Errorf("abs %q should be within sandbox %q", abs, r2)
	}
	if filepath.Dir(abs) != r2 {
		t.Errorf("abs = %q, want parent dir %q", abs, r2)
	}
}

func TestResolveSandboxedPathMulti_allReject(t *testing.T) {
	// 所有沙箱都拒绝越狱
	_, err := resolveSandboxedPathMulti("../escape.txt", t.TempDir(), t.TempDir())
	if err == nil {
		t.Error("all sandboxes should reject escape path")
	}
}

func TestResolveSandboxedPathMulti_emptyPath(t *testing.T) {
	_, err := resolveSandboxedPathMulti("", t.TempDir())
	if err == nil {
		t.Error("empty path should be rejected")
	}
}

// ── isPathWithinSandbox ───────────────────────────

func TestIsPathWithinSandbox(t *testing.T) {
	root := t.TempDir()

	// 在沙箱内
	if !isPathWithinSandbox(filepath.Join(root, "a/b.txt"), root) {
		t.Error("path within sandbox should be allowed")
	}

	// 在沙箱外
	outside := "/tmp/some-file.txt"
	if isPathWithinSandbox(outside, root) {
		t.Error("path outside sandbox should be rejected")
	}

	// 同级目录
	peer := t.TempDir()
	if isPathWithinSandbox(peer, root) {
		t.Error("peer directory should not be within sandbox")
	}
}

// ── classifyFileKind edge cases ──────────────────

func TestClassifyFileKind_EdgeCase(t *testing.T) {
	cases := []struct {
		name     string
		wantKind string
		wantMime string
	}{
		{"README.MD", "md", "text/markdown"}, // 大写 .MD
		{"IMG.JPG", "image", "image/jpeg"},   // 大写 .JPG
		{"IMG.JPEG", "image", "image/jpeg"},
		{"IMG.PNG", "image", "image/png"},
		{"IMG.GIF", "image", "image/gif"},
		{"IMG.WEBP", "image", "image/webp"},
		{"IMG.SVG", "image", "image/svg+xml"},
		{"IMG.BMP", "image", "image/bmp"},
		{"FAVICON.ICO", "image", "image/x-icon"},
		{"noext", "text", "text/plain"},
		{"Makefile", "text", "text/plain"},
		{"unknown.xyz", "text", "text/plain"},
		{"SPEC.PDF", "pdf", "application/pdf"},
		{"doc.PDF", "pdf", "application/pdf"},
	}
	for _, c := range cases {
		kind, mime := classifyFileKind(c.name)
		if kind != c.wantKind || mime != c.wantMime {
			t.Errorf("classifyFileKind(%q) = (%q, %q), want (%q, %q)", c.name, kind, mime, c.wantKind, c.wantMime)
		}
	}
}

// ── isBinaryContent edge cases ───────────────────

func TestIsBinaryContent_Empty(t *testing.T) {
	if isBinaryContent([]byte{}) {
		t.Error("empty content should not be binary")
	}
}

func TestIsBinaryContent_ExactlySniffWindow(t *testing.T) {
	// 刚好 8192 字节，第 8191 处为 NUL → 二进制
	buf := make([]byte, 8192)
	for i := range buf {
		buf[i] = 'a'
	}
	buf[8191] = 0
	if !isBinaryContent(buf) {
		t.Error("NUL at end of sniff window should be detected")
	}
}

func TestIsBinaryContent_JustAfterSniffWindow(t *testing.T) {
	// 9000 字节，NUL 在第 8200 字节 → 嗅探窗口外，不被判定为二进制
	buf := make([]byte, 9000)
	for i := range buf {
		buf[i] = 'a'
	}
	buf[8200] = 0
	if isBinaryContent(buf) {
		t.Error("NUL beyond sniff window should not mark content as binary")
	}
}

func TestIsBinaryContent_UTF8Text(t *testing.T) {
	if isBinaryContent([]byte("hello world\n你好世界\n")) {
		t.Error("UTF-8 text should not be detected as binary")
	}
}

// ── Sandbox integration via helper chain (covers ReadFile/WriteFile/DeleteEntry path) ──

func TestSandboxChain_ValidFile(t *testing.T) {
	root := t.TempDir()
	// 创建测试文件
	testFile := filepath.Join(root, "test.md")
	if err := os.WriteFile(testFile, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}

	// 模拟 ReadFile 的路径解析链路
	abs, err := resolveSandboxedPath("test.md", root)
	if err != nil {
		t.Fatalf("resolveSandboxedPath failed: %v", err)
	}
	if !isPathWithinSandbox(abs, root) {
		t.Errorf("path %q should be within sandbox", abs)
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(raw) != "hello" {
		t.Errorf("content = %q, want %q", string(raw), "hello")
	}
}

func TestSandboxChain_EscapeRejected(t *testing.T) {
	root := t.TempDir()

	// 所有三种越狱模式都在沙箱层被拒绝
	escapes := []string{"../outside.txt", "../../etc/passwd", "sub/../../escape.txt"}
	for _, p := range escapes {
		abs, err := resolveSandboxedPath(p, root)
		if err == nil {
			t.Errorf("escape path %q: resolveSandboxedPath should reject, got %q", p, abs)
		}
		if isPathWithinSandbox(filepath.Join(root, p), root) {
			t.Errorf("escape path %q should not be within sandbox", p)
		}
	}
}

func TestSandboxChain_WriteFileAtomPattern(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "notes.md")

	// 模拟 WriteFile 的原子写模式
	tmp, err := os.CreateTemp(root, ".mady-write-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.WriteString("new content"); err != nil {
		_ = tmp.Close()
		t.Fatal(err)
	}
	_ = tmp.Close()
	if err := os.Rename(tmpName, target); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "new content" {
		t.Errorf("content = %q, want %q", string(raw), "new content")
	}
}

func TestSandboxChain_DeleteEntryPattern(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "delete-me.txt")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	// 模拟 DeleteEntry 的 sandbox 检查 + os.Remove
	abs, err := resolveSandboxedPath("delete-me.txt", root)
	if err != nil {
		t.Fatal(err)
	}
	if !isPathWithinSandbox(abs, root) {
		t.Fatal("path not within sandbox")
	}
	if err := os.Remove(abs); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}
}

func TestSandboxChain_NonEmptyDirRejected(t *testing.T) {
	// 真实调用 App.DeleteEntry：非空目录必须被拒绝且目录保留
	root := t.TempDir()
	dir := filepath.Join(root, "mydir")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	app := &App{
		server: madyserver.New(agentcore.Config{}),
		fc:     &bootstrap.Context{BaseConfig: agentcore.Config{ProjectDir: root}, WorkspaceDir: root},
	}
	if err := app.DeleteEntry("mydir"); err == nil {
		t.Fatal("DeleteEntry(non-empty dir) should be rejected")
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Errorf("non-empty dir should not be deleted: %v", statErr)
	}
}

func TestSandboxChain_FileTooLarge(t *testing.T) {
	// 真实调用 App.ReadFile：超限文件必须被拒绝
	root := t.TempDir()
	big := make([]byte, maxReadFileSize+1)
	if err := os.WriteFile(filepath.Join(root, "big.md"), big, 0600); err != nil {
		t.Fatal(err)
	}

	app := &App{
		server: madyserver.New(agentcore.Config{}),
		fc:     &bootstrap.Context{BaseConfig: agentcore.Config{ProjectDir: root}, WorkspaceDir: root},
	}
	if _, err := app.ReadFile("big.md"); err == nil {
		t.Error("ReadFile(too large) should fail")
	}
}

// TestSandbox_SymlinkReadEscape 验证符号链接读逃逸被拒绝（G-B1）。
// 沙箱内 symlink -> 沙箱外文件：isPathWithinSandbox / resolveSandboxedPath 必须拒绝。
func TestSandbox_SymlinkReadEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("top-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "evil-link.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	if isPathWithinSandbox(link, root) {
		t.Error("symlink to outside file should NOT be within sandbox")
	}
	if _, err := resolveSandboxedPath("evil-link.txt", root); err == nil {
		t.Error("resolveSandboxedPath(symlink escape) should fail")
	}
}

// TestSandbox_SymlinkDirEscape 验证经符号链接目录的写逃逸被拒绝（G-B1）。
// 沙箱内 symlink dir -> 沙箱外目录：写入目标必须被拒绝。
func TestSandbox_SymlinkDirEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "evil-dir")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	target := filepath.Join(root, "evil-dir", "x.md")
	if isPathWithinSandbox(target, root) {
		t.Error("write through symlink dir should NOT be within sandbox")
	}
	if _, err := resolveSandboxedPath(filepath.Join("evil-dir", "x.md"), root); err == nil {
		t.Error("resolveSandboxedPath(symlink dir escape) should fail")
	}
}

// TestSandbox_SymlinkWithinRoot 验证沙箱内正常 symlink（指向沙箱内）不被误伤。
func TestSandbox_SymlinkWithinRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real.txt"), filepath.Join(root, "alias.txt")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	if !isPathWithinSandbox(filepath.Join(root, "alias.txt"), root) {
		t.Error("symlink pointing inside sandbox should be allowed")
	}
}

// --- AI 服务设置（Q9） ---

func TestAISettingsPersistence_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := aiSettingsPath(dir)

	want := AISettings{Provider: "kimi", Model: "kimi-k2.6"}
	if err := saveAISettingsTo(path, want); err != nil {
		t.Fatalf("saveAISettingsTo: %v", err)
	}
	got := loadAISettingsFrom(path)
	if got != want {
		t.Errorf("round trip mismatch: got %+v, want %+v", got, want)
	}
	// 原子写不应残留 tmp 文件
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("tmp file should not exist after atomic rename")
	}
}

func TestLoadAISettingsFrom_MissingOrInvalid(t *testing.T) {
	dir := t.TempDir()

	// 文件不存在 → 零值，不视为错误
	if got := loadAISettingsFrom(aiSettingsPath(dir)); got != (AISettings{}) {
		t.Errorf("missing file should yield zero AISettings, got %+v", got)
	}

	// 非法 JSON → 零值，不视为错误
	bad := aiSettingsPath(dir)
	if err := os.WriteFile(bad, []byte("{oops"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := loadAISettingsFrom(bad); got != (AISettings{}) {
		t.Errorf("invalid JSON should yield zero AISettings, got %+v", got)
	}
}

func TestSetAISettings_RequiresInput(t *testing.T) {
	app := &App{}
	if err := app.SetAISettings(AISettings{}); err == nil {
		t.Error("empty AISettings should be rejected")
	}
}

func TestSetAISettings_ModelOnly(t *testing.T) {
	dir := t.TempDir()
	app := &App{
		fc:         &bootstrap.Context{MadyHome: dir},
		aiProvider: "deepseek",
		aiModel:    "deepseek-v4-flash",
	}

	if err := app.SetAISettings(AISettings{Model: "deepseek-v4-pro"}); err != nil {
		t.Fatalf("SetAISettings: %v", err)
	}

	// 运行时状态更新
	if app.aiModel != "deepseek-v4-pro" {
		t.Errorf("aiModel = %q, want deepseek-v4-pro", app.aiModel)
	}
	if app.aiProvider != "deepseek" {
		t.Errorf("aiProvider = %q, want deepseek (unchanged)", app.aiProvider)
	}
	// framework 上下文同步更新
	if app.fc.BaseConfig.ModelConfig.Model != "deepseek-v4-pro" {
		t.Errorf("BaseConfig model = %q, want deepseek-v4-pro", app.fc.BaseConfig.ModelConfig.Model)
	}
	// 持久化生效
	got := loadAISettingsFrom(aiSettingsPath(dir))
	if got.Provider != "deepseek" || got.Model != "deepseek-v4-pro" {
		t.Errorf("persisted settings = %+v, want {deepseek deepseek-v4-pro}", got)
	}
}

func TestSetAISettings_ProviderWithoutKey(t *testing.T) {
	// 清空所有相关 API Key，确保 BuildProvider 必然失败
	t.Setenv("PROVIDER", "deepseek")
	t.Setenv("API_KEY", "")
	t.Setenv("ZHIPU_API_KEY", "")

	dir := t.TempDir()
	app := &App{
		fc:         &bootstrap.Context{MadyHome: dir},
		aiProvider: "deepseek",
		aiModel:    "deepseek-v4-flash",
	}

	err := app.SetAISettings(AISettings{Provider: "zhipu"})
	if err == nil {
		t.Fatal("provider switch without API key should fail")
	}

	// 失败时状态完全不变
	if app.aiProvider != "deepseek" || app.aiModel != "deepseek-v4-flash" {
		t.Errorf("state mutated on failure: provider=%q model=%q", app.aiProvider, app.aiModel)
	}
	// 环境变量已回滚
	if got := os.Getenv("PROVIDER"); got == "zhipu" {
		t.Error("PROVIDER env should be rolled back on failure")
	}
	// 未持久化
	if got := loadAISettingsFrom(aiSettingsPath(dir)); got != (AISettings{}) {
		t.Errorf("nothing should be persisted on failure, got %+v", got)
	}
}
