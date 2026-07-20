// computer_use_keys.go：键名映射与按键字符串规范化。
// 职责：各后端的键名/修饰键映射表（cliclick、osascript、cua-driver、Windows VK、
// xdotool、ydotool）及 Unicode 键符号别名（⌘⌥⌃⇧ 等）到 ASCII 的替换。

package desktop

import "strings"

var cliclickKeyNames = map[string]string{
	"return": "return", "enter": "return",
	"escape": "escape", "esc": "escape",
	"tab": "tab", "space": "space",
	"delete": "backspace", "backspace": "backspace",
	"up": "up", "down": "down", "left": "left", "right": "right",
	"home": "home", "end": "end",
	"pageup": "pageup", "pagedown": "pagedown",
	"pgup": "pageup", "pgdn": "pagedown",
	"f1": "f1", "f2": "f2", "f3": "f3", "f4": "f4",
	"f5": "f5", "f6": "f6", "f7": "f7", "f8": "f8",
	"f9": "f9", "f10": "f10", "f11": "f11", "f12": "f12",
}

var cliclickModMap = map[string]string{
	"cmd": "cmd", "command": "cmd",
	"ctrl": "ctrl", "control": "ctrl",
	"alt": "alt", "option": "alt",
	"shift": "shift",
}

var osaKeyNames = map[string]string{
	"return": "return", "enter": "return",
	"escape": "escape", "esc": "escape",
	"tab": "tab", "space": "space",
	"delete": "delete", "backspace": "delete",
	"up": "up", "down": "down", "left": "left", "right": "right",
	"home": "home", "end": "end",
	"pageup": "page up", "pagedown": "page down",
	"pgup": "page up", "pgdn": "page down",
}

var osaModMap = map[string]string{
	"cmd": "command down", "command": "command down",
	"ctrl": "control down", "control": "control down",
	"alt": "option down", "option": "option down",
	"shift": "shift down",
}

var unicodeKeyAliases = map[string]string{
	"⌘": "cmd",
	"⌥": "option",
	"⌃": "ctrl",
	"⇧": "shift",
	"␣": "space",
	"⏎": "return",
	"↩": "return",
	"⌤": "enter",
	"⌫": "backspace",
	"⌦": "delete",
	"⎋": "escape",
	"⇥": "tab",
	"⇞": "pageup",
	"⇟": "pagedown",
	"↖": "home",
	"↘": "end",
	"↑": "up",
	"↓": "down",
	"←": "left",
	"→": "right",
}

func normalizeKeyString(s string) string {
	for unicode, ascii := range unicodeKeyAliases {
		s = strings.ReplaceAll(s, unicode, ascii)
	}
	return s
}

var cuaModMap = map[string]string{
	"cmd": "cmd", "command": "cmd",
	"ctrl": "ctrl", "control": "ctrl",
	"alt": "alt", "option": "alt",
	"shift": "shift",
}

var winVK = map[string]byte{
	"return": 0x0D, "enter": 0x0D,
	"escape": 0x1B, "esc": 0x1B,
	"tab": 0x09, "space": 0x20,
	"backspace": 0x08, "delete": 0x08,
	"up": 0x26, "down": 0x28,
	"left": 0x25, "right": 0x27,
	"home": 0x24, "end": 0x23,
	"pageup": 0x21, "pagedown": 0x22,
	"pgup": 0x21, "pgdn": 0x22,
}

var xdoKeyMap = map[string]string{
	"return": "Return", "enter": "Return",
	"escape": "Escape", "esc": "Escape",
	"tab": "Tab", "space": "space",
	"backspace": "BackSpace", "delete": "BackSpace",
	"up": "Up", "down": "Down", "left": "Left", "right": "Right",
	"home": "Home", "end": "End",
	"pageup": "Page_Up", "pagedown": "Page_Down",
	"pgup": "Page_Up", "pgdn": "Page_Down",
	"f1": "F1", "f2": "F2", "f3": "F3", "f4": "F4",
	"f5": "F5", "f6": "F6", "f7": "F7", "f8": "F8",
	"f9": "F9", "f10": "F10", "f11": "F11", "f12": "F12",
}

var ydoKeyCodes = map[string]string{
	"return": "KEY_ENTER", "enter": "KEY_ENTER",
	"escape": "KEY_ESC", "esc": "KEY_ESC",
	"tab": "KEY_TAB", "space": "KEY_SPACE",
	"backspace": "KEY_BACKSPACE", "delete": "KEY_BACKSPACE",
	"up": "KEY_UP", "down": "KEY_DOWN", "left": "KEY_LEFT", "right": "KEY_RIGHT",
	"home": "KEY_HOME", "end": "KEY_END",
	"pageup": "KEY_PAGEUP", "pagedown": "KEY_PAGEDOWN",
	"pgup": "KEY_PAGEUP", "pgdn": "KEY_PAGEDOWN",
}
