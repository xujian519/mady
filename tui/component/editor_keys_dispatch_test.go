package component

// editor_keys_dispatch_test.go 补充键盘分发层测试。
//
// 背景：既有 editor 测试大多直接调用底层编辑方法（moveCursor/insertRune 等），
// 绕过 processKeys 的 keybinding 匹配层，导致 processKeys 语句覆盖率仅 27.7%。
// 本文件通过 Update(core.KeyMsg{}) 驱动真实按键序列，覆盖：
//   - 光标移动（Ctrl+A/E、Home/End、←/→）
//   - 删除家族（Backspace/Delete/Ctrl+W/Ctrl+U/Ctrl+K）
//   - 撤销/重做、kill-ring yank
//   - Enter 提交 / Alt+Enter / Shift+Enter 换行
//   - 控制字符忽略、Ctrl+P 无绑定键被静默吞掉（锁定现状，防意外劫持）
//   - 输入历史回填（↑/↓）

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

// newDispatchEditor 创建绑定了全局 keybindings 的编辑器并聚焦。
func newDispatchEditor() *Editor {
	e := NewEditor(nil) // nil → 全局默认 keybindings
	e.SetFocused(true)
	return e
}

// press 通过 Update 注入一次按键（与真实终端一致：KeyMsg.Data 是原始字节）。
func press(e *Editor, data string) {
	e.Update(core.KeyMsg{Data: data})
}

func TestEditorDispatchTypingMixed(t *testing.T) {
	e := newDispatchEditor()
	press(e, "hi中😀")
	if got := e.GetValue(); got != "hi中😀" {
		t.Fatalf("typed value = %q, want %q", got, "hi中😀")
	}
}

func TestEditorDispatchCursorMotion(t *testing.T) {
	e := newDispatchEditor()
	press(e, "hello world")
	// Ctrl+A → 行首；再输入前缀。
	press(e, "\x01") // ctrl+a
	press(e, "X")
	if got := e.GetValue(); got != "Xhello world" {
		t.Fatalf("ctrl+a + type: got %q", got)
	}
	// Ctrl+E → 行尾；再输入后缀。
	press(e, "\x05") // ctrl+e
	press(e, "Y")
	if got := e.GetValue(); got != "Xhello worldY" {
		t.Fatalf("ctrl+e + type: got %q", got)
	}
	// ← 左移一字符后输入。
	press(e, "\x1b[D") // left
	press(e, "Z")
	if got := e.GetValue(); got != "Xhello worldZY" {
		t.Fatalf("left + type: got %q", got)
	}
	// → 右移回末尾。
	press(e, "\x1b[C") // right
	press(e, "W")
	if got := e.GetValue(); got != "Xhello worldZYW" {
		t.Fatalf("right + type: got %q", got)
	}
	// Home/End。
	press(e, "\x1b[H") // home
	press(e, "A")
	press(e, "\x1b[F") // end
	press(e, "B")
	if got := e.GetValue(); got != "AXhello worldZYWB" {
		t.Fatalf("home/end: got %q, want %q", got, "AXhello worldZYWB")
	}
}

func TestEditorDispatchDeleteFamily(t *testing.T) {
	e := newDispatchEditor()
	press(e, "one two three")
	// Ctrl+W 删除前一词（"three" → 连同前导空格）。
	press(e, "\x17") // ctrl+w
	if got := e.GetValue(); got != "one two " {
		t.Fatalf("ctrl+w: got %q, want %q", got, "one two ")
	}
	// Ctrl+U 删除至行首。
	press(e, "\x15") // ctrl+u
	if got := e.GetValue(); got != "" {
		t.Fatalf("ctrl+u: got %q, want empty", got)
	}
	// 重新输入，Ctrl+K 删除至行尾。
	press(e, "abc")
	press(e, "\x01") // ctrl+a 行首
	press(e, "\x0b") // ctrl+k 删至行尾
	if got := e.GetValue(); got != "" {
		t.Fatalf("ctrl+k: got %q, want empty", got)
	}
	// Backspace 删除。
	press(e, "xy")
	press(e, "\x7f") // backspace
	if got := e.GetValue(); got != "x" {
		t.Fatalf("backspace: got %q, want %q", got, "x")
	}
	// Delete 删除光标后字符。
	press(e, "\x01")    // ctrl+a
	press(e, "\x1b[3~") // delete
	if got := e.GetValue(); got != "" {
		t.Fatalf("delete: got %q, want empty", got)
	}
}

func TestEditorDispatchUndoRedo(t *testing.T) {
	e := newDispatchEditor()
	press(e, "abc")
	// 每个字符插入是独立快照：按 3 次 Ctrl+Z 全部撤销。
	press(e, "\x1a") // ctrl+z
	press(e, "\x1a")
	press(e, "\x1a")
	if got := e.GetValue(); got != "" {
		t.Fatalf("undo x3: got %q, want empty", got)
	}
	// Kitty CSI-u: ctrl+shift+z (z=122, mods=6 → ctrl+shift)。redo 增量式：
	// 一次恢复一个撤销快照，按 3 次全部重做。
	press(e, "\x1b[122;6u")
	if got := e.GetValue(); got != "a" {
		t.Fatalf("redo: got %q, want %q", got, "a")
	}
	press(e, "\x1b[122;6u")
	press(e, "\x1b[122;6u")
	if got := e.GetValue(); got != "abc" {
		t.Fatalf("redo x3: got %q, want %q", got, "abc")
	}
	// 纯 ctrl+z 不被 redo 分支劫持：redo 显式要求 Shift。
	press(e, "\x1a") // ctrl+z → undo（不是 redo）
	if got := e.GetValue(); got != "ab" {
		t.Fatalf("ctrl+z after redo: got %q, want %q", got, "ab")
	}
}

func TestEditorDispatchKillRingYank(t *testing.T) {
	e := newDispatchEditor()
	press(e, "hello world")
	press(e, "\x01") // ctrl+a 行首
	press(e, "\x0b") // ctrl+k 删至行尾（进入 kill-ring）
	if got := e.GetValue(); got != "" {
		t.Fatalf("ctrl+k: got %q, want empty", got)
	}
	press(e, "X")
	press(e, "\x19") // ctrl+y yank
	if got := e.GetValue(); got != "Xhello world" {
		t.Fatalf("yank: got %q, want %q", got, "Xhello world")
	}
}

func TestEditorDispatchSubmitAndNewline(t *testing.T) {
	var submitted string
	e := NewEditor(nil)
	e.SetFocused(true)
	e.OnSubmit(func(v string) { submitted = v })
	press(e, "hello")
	press(e, "\r") // enter → submit
	if submitted != "hello" {
		t.Fatalf("submit = %q, want %q", submitted, "hello")
	}

	// Alt+Enter（ESC+CR）→ 插入换行而不是提交（独立实例）。
	e2 := NewEditor(nil)
	e2.SetFocused(true)
	press(e2, "a")
	press(e2, "\x1b\r") // alt+enter
	press(e2, "b")
	if got := e2.GetValue(); got != "a\nb" {
		t.Fatalf("alt+enter: got %q, want %q", got, "a\nb")
	}
}

func TestEditorDispatchKittyShiftEnter(t *testing.T) {
	e := newDispatchEditor()
	press(e, "x")
	press(e, "\x1b[13;2u") // kitty CSI-u: enter + shift → newline
	press(e, "y")
	if got := e.GetValue(); got != "x\ny" {
		t.Fatalf("kitty shift+enter: got %q, want %q", got, "x\ny")
	}
}

// TestEditorDispatchUnboundKeysIgnored 锁定未绑定键（如 Ctrl+P）被静默吞掉：
// 不产生字符、不移动光标、不提交。若未来为 Ctrl+P 接入命令面板，本测试需同步更新。
func TestEditorDispatchUnboundKeysIgnored(t *testing.T) {
	e := newDispatchEditor()
	press(e, "ab")
	press(e, "\x10") // ctrl+p —— 无 editor 绑定
	if got := e.GetValue(); got != "ab" {
		t.Fatalf("ctrl+p should be ignored by editor, got %q", got)
	}
	press(e, "\x00") // NUL 控制字符
	if got := e.GetValue(); got != "ab" {
		t.Fatalf("NUL should be ignored, got %q", got)
	}
}

func TestEditorDispatchHistoryRecall(t *testing.T) {
	e := newDispatchEditor()
	e.PushInputHistory("first input")
	e.PushInputHistory("second input")
	press(e, "draft text")
	// ↑ 回填最新历史。
	press(e, "\x1b[A") // up
	if got := e.GetValue(); got != "second input" {
		t.Fatalf("up recall: got %q, want %q", got, "second input")
	}
	// 再 ↑ 回更旧一条。
	press(e, "\x1b[A")
	if got := e.GetValue(); got != "first input" {
		t.Fatalf("up recall 2: got %q, want %q", got, "first input")
	}
	// ↓ 回到较新条目。
	press(e, "\x1b[B") // down
	if got := e.GetValue(); got != "second input" {
		t.Fatalf("down recall: got %q, want %q", got, "second input")
	}
	// 再 ↓ 退出历史模式，恢复草稿。
	press(e, "\x1b[B")
	if got := e.GetValue(); got != "draft text" {
		t.Fatalf("down exit history: got %q, want %q", got, "draft text")
	}
}

// TestEditorDispatchTabNoNULWhenNoAutocomplete 验证无 autocomplete 时 Tab 被忽略，
// 不向 buffer 插入 NUL 控制字符（回归：Tab 解析出的 Rune 为 \x00，旧代码会
// insertRune 导致不可见字符污染提交文本）。
func TestEditorDispatchTabNoNULWhenNoAutocomplete(t *testing.T) {
	e := newDispatchEditor()
	press(e, "a")
	press(e, "\t") // tab 无 autocomplete 时忽略
	press(e, "b")
	if got := e.GetValue(); got != "ab" {
		t.Fatalf("tab without autocomplete: got %q, want %q", got, "ab")
	}
	if strings.ContainsRune(e.GetValue(), '\x00') {
		t.Fatal("buffer must not contain NUL after tab")
	}
}
