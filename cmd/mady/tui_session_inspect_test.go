package main

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xujian519/mady/agentcore/evidence"
	"github.com/xujian519/mady/agentcore/filecheckpoint"
	"github.com/xujian519/mady/memory"
	"github.com/xujian519/mady/tui/chat"
)

// lastSystemMessage 返回 ChatApp 历史中最后一条系统消息文本。
// 用于断言 inspect 命令的输出。
func lastSystemMessage(s *tuiSession) string {
	msgs := s.app.History().Messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == chat.RoleSystem {
			return msgs[i].Text
		}
	}
	return ""
}

// newInspectSession 构造一个带 evidence/filecheckpoint/memory 扩展的 tuiSession，
// 用于测试 /ledger /snapshots /undo /memory 四个命令。
func newInspectSession(t *testing.T) *tuiSession {
	t.Helper()
	app := testAppForSession(t)
	s := &tuiSession{
		ctx: context.Background(),
		fc:  &frameworkContext{},
		app: app,
	}
	// 注入真实的 evidence + filecheckpoint 扩展（root="" 禁用路径逃逸检查，便于测试）
	s.fc.EvidenceExt = evidence.NewExtension()
	s.fc.FileCheckpointExt = filecheckpoint.NewExtensionWithFS(filecheckpoint.OSFileSystem{}, "")
	return s
}

// TestHandleLedgerCommand_Empty 验证无工具调用时的提示。
func TestHandleLedgerCommand_Empty(t *testing.T) {
	s := newInspectSession(t)
	s.handleLedgerCommand()
	msg := lastSystemMessage(s)
	if !strings.Contains(msg, "暂无") {
		t.Fatalf("期望提示'暂无工具调用证据'，实际：%q", msg)
	}
}

// TestHandleLedgerCommand_WithData 验证有 Receipt 时正确渲染。
func TestHandleLedgerCommand_WithData(t *testing.T) {
	s := newInspectSession(t)
	ledger := s.fc.EvidenceExt.Ledger()
	ledger.Record(evidence.Receipt{
		ToolName:   "read",
		Success:    true,
		Paths:      []string{"/tmp/foo.go"},
		Read:       true,
		DurationMs: 12,
	})
	ledger.Record(evidence.Receipt{
		ToolName: "write_file",
		Success:  true,
		Paths:    []string{"/tmp/bar.go"},
		Write:    true,
	})
	s.handleLedgerCommand()
	msg := lastSystemMessage(s)
	for _, want := range []string{"共 2 条", "read", "write_file", "/tmp/foo.go", "/tmp/bar.go", "[write]"} {
		if !strings.Contains(msg, want) {
			t.Errorf("期望输出包含 %q，实际：%s", want, msg)
		}
	}
}

// TestHandleLedgerCommand_NotEnabled 验证扩展缺失时降级提示。
func TestHandleLedgerCommand_NotEnabled(t *testing.T) {
	app := testAppForSession(t)
	s := &tuiSession{ctx: context.Background(), fc: nil, app: app}
	s.handleLedgerCommand()
	msg := lastSystemMessage(s)
	if !strings.Contains(msg, "未启用") {
		t.Fatalf("期望降级提示'未启用'，实际：%q", msg)
	}
}

// TestHandleSnapshotsCommand_Empty 验证无快照时的提示。
func TestHandleSnapshotsCommand_Empty(t *testing.T) {
	s := newInspectSession(t)
	s.handleSnapshotsCommand()
	msg := lastSystemMessage(s)
	if !strings.Contains(msg, "暂无文件快照") {
		t.Fatalf("期望提示'暂无文件快照'，实际：%q", msg)
	}
}

// TestHandleSnapshotsCommand_WithData 验证有快照时正确列出。
func TestHandleSnapshotsCommand_WithData(t *testing.T) {
	s := newInspectSession(t)
	store := s.fc.FileCheckpointExt.Store()
	store.BeginTurn(1, "请帮我创建 foo.go", 0)
	store.EndTurn()
	store.BeginTurn(2, "修改 bar.go", 1)
	store.EndTurn()
	s.handleSnapshotsCommand()
	msg := lastSystemMessage(s)
	for _, want := range []string{"共 2 轮", "请帮我创建 foo.go", "修改 bar.go", "轮 1", "轮 2"} {
		if !strings.Contains(msg, want) {
			t.Errorf("期望输出包含 %q，实际：%s", want, msg)
		}
	}
}

// TestHandleUndoCommand_NoArg 验证无参数时显示用法。
func TestHandleUndoCommand_NoArg(t *testing.T) {
	s := newInspectSession(t)
	// 模拟 parseSlashSubcommand("/undo") 返回 ""
	s.handleUndoCommand("")
	msg := lastSystemMessage(s)
	if !strings.Contains(msg, "用法") || !strings.Contains(msg, "/undo <轮号>") {
		t.Fatalf("期望显示用法，实际：%q", msg)
	}
}

// TestHandleUndoCommand_InvalidTurn 验证无效轮号报错。
func TestHandleUndoCommand_InvalidTurn(t *testing.T) {
	s := newInspectSession(t)
	// 模拟 parseSlashSubcommand("/undo abc") 返回 "abc"
	s.handleUndoCommand("abc")
	msg := lastSystemMessage(s)
	if !strings.Contains(msg, "无效的轮号") {
		t.Fatalf("期望提示'无效的轮号'，实际：%q", msg)
	}
}

// TestHandleUndoCommand_NotFound 验证找不到快照时报错（RestoreAndTrim 返回 error）。
func TestHandleUndoCommand_NotFound(t *testing.T) {
	s := newInspectSession(t)
	s.handleUndoCommand("99")
	msg := lastSystemMessage(s)
	if !strings.Contains(msg, "回退失败") || !strings.Contains(msg, "turn 99") {
		t.Fatalf("期望提示'回退失败 ... turn 99'，实际：%q", msg)
	}
}

// TestHandleUndoCommand_Success 验证回退成功路径（覆盖新增的 Meta 返回值）。
func TestHandleUndoCommand_Success(t *testing.T) {
	s := newInspectSession(t)
	store := s.fc.FileCheckpointExt.Store()
	store.BeginTurn(1, "创建测试文件", 0)
	store.EndTurn()
	s.handleUndoCommand("1")
	msg := lastSystemMessage(s)
	for _, want := range []string{"已回退到轮 1", "创建测试文件"} {
		if !strings.Contains(msg, want) {
			t.Errorf("期望输出包含 %q，实际：%s", want, msg)
		}
	}
}

// TestHandleMemoryCommand_NotEnabled 验证 memory 未启用时降级。
func TestHandleMemoryCommand_NotEnabled(t *testing.T) {
	app := testAppForSession(t)
	s := &tuiSession{ctx: context.Background(), fc: &frameworkContext{}, app: app}
	s.handleMemoryCommand("")
	msg := lastSystemMessage(s)
	if !strings.Contains(msg, "未启用") {
		t.Fatalf("期望降级提示'未启用'，实际：%q", msg)
	}
}

// TestHandleMemoryCommand_WithData 验证有记忆条目时正确渲染，
// 同时覆盖 ListOptions.UserID 过滤（不应出现其他用户的记忆）。
func TestHandleMemoryCommand_WithData(t *testing.T) {
	app := testAppForSession(t)
	mgr := memory.NewManager(memory.NewInMemoryStore(), nil, nil, memory.ManagerConfig{})
	s := &tuiSession{
		ctx: context.Background(),
		fc:  &frameworkContext{MemoryManager: mgr},
		app: app,
	}
	// 写入当前用户的一条记忆
	userID := stableUserID()
	scope := memory.MemoryScope{UserID: userID, AgentID: "mady-agent"}
	otherScope := memory.MemoryScope{UserID: "other-user", AgentID: "mady-agent"}
	ctx := context.Background()
	if _, err := mgr.Store().Remember(ctx, "用户偏好使用深色主题", scope, memory.LayerUser, nil); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	// 写入另一个用户的记忆（不应在 /memory 输出中出现）
	if _, err := mgr.Store().Remember(ctx, "其他用户的隐私数据", otherScope, memory.LayerUser, nil); err != nil {
		t.Fatalf("Remember other: %v", err)
	}

	s.handleMemoryCommand("")
	msg := lastSystemMessage(s)
	for _, want := range []string{"跨三层", "用户偏好", "深色主题", "重要度="} {
		if !strings.Contains(msg, want) {
			t.Errorf("期望输出包含 %q，实际：%s", want, msg)
		}
	}
	// 关键回归：UserID scope 过滤应在存储层完成，其他用户的记忆绝不可见
	if strings.Contains(msg, "其他用户的隐私数据") {
		t.Errorf("跨用户数据泄漏：输出包含其他用户记忆")
	}
}

// TestHandleMemoryCommand_Search 验证带 query 时走语义检索路径。
func TestHandleMemoryCommand_Search(t *testing.T) {
	app := testAppForSession(t)
	mgr := memory.NewManager(memory.NewInMemoryStore(), nil, nil, memory.ManagerConfig{})
	s := &tuiSession{
		ctx: context.Background(),
		fc:  &frameworkContext{MemoryManager: mgr},
		app: app,
	}
	// 写入一条记忆供检索
	scope := memory.MemoryScope{UserID: stableUserID(), AgentID: "mady-agent"}
	ctx := context.Background()
	if _, err := mgr.Store().Remember(ctx, "用户喜欢深色主题界面", scope, memory.LayerUser, nil); err != nil {
		t.Fatalf("Remember: %v", err)
	}

	s.handleMemoryCommand("深色")
	msg := lastSystemMessage(s)
	if !strings.Contains(msg, "记忆检索") {
		t.Errorf("期望走检索路径（含'记忆检索'前缀），实际：%s", msg)
	}
}

// TestTruncateRunes_UTF8Safety 是 B1 的回归测试：验证截断不会产生无效 UTF-8。
// 这是 byte 切片 bug 的直接回归保护——之前 s[:N] 在中英混排时必然踩雷。
func TestTruncateRunes_UTF8Safety(t *testing.T) {
	cases := []struct {
		name  string
		input string
		max   int
	}{
		// 中英混排，max 取各种边界值（旧 bug 在这些值上必触发无效 UTF-8）
		{"mixed_57", "用户输入 mady tui 然后请求撰写权利要求书和说明书", 57},
		{"mixed_77", "执行 bash command: echo 'hello' > /tmp/output.txt 然后继续处理后续任务", 77},
		{"mixed_117", "专利申请文件 abcdefg 这是一段很长的描述用于测试截断逻辑是否会破坏 UTF-8 字符的完整性", 117},
		{"short", "短", 5},
		{"exact", "abcd", 4}, // 恰好等于 max，不应截断
		{"ascii_long", "abcdefghij", 5},
		{"zero", "anything", 0},
		{"one", "ab", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := truncateRunes(c.input, c.max)
			// 1. 必须是合法 UTF-8（B1 的核心保护点）
			if !utf8.ValidString(got) {
				t.Errorf("truncateRunes(%q, %d) = %q — 无效 UTF-8", c.input, c.max, got)
			}
			// 2. rune 数不超过 max（或为单字符省略号）
			runeCount := len([]rune(got))
			if runeCount > c.max {
				t.Errorf("truncateRunes(%q, %d) = %q — rune 数 %d 超过 max %d",
					c.input, c.max, got, runeCount, c.max)
			}
			// 3. 短输入原样返回
			if len([]rune(c.input)) <= c.max && got != c.input {
				t.Errorf("truncateRunes(%q, %d) = %q — 短输入不应改变", c.input, c.max, got)
			}
		})
	}
}

// TestParseSlashRest 验证 B2 新增的 helper：多词参数保留。
func TestParseSlashRest(t *testing.T) {
	cases := []struct {
		input string
		cmd   string
		want  string
	}{
		{"/memory 偏好深色", "memory", "偏好深色"},
		{"/memory 用户偏好 深色 主题", "memory", "用户偏好 深色 主题"}, // 多词保留
		{"/memory", "memory", ""},
		{"/memory ", "memory", ""},
		{"/undo 3", "undo", "3"},
		{"/ledger", "ledger", ""},
		{"not a command", "memory", ""},
	}
	for _, c := range cases {
		got := parseSlashRest(c.input, c.cmd)
		if got != c.want {
			t.Errorf("parseSlashRest(%q, %q) = %q, want %q", c.input, c.cmd, got, c.want)
		}
	}
}

// TestLayerDisplayName 验证层名显示。
func TestLayerDisplayName(t *testing.T) {
	cases := []struct {
		layer memory.MemoryLayer
		want  string
	}{
		{memory.LayerUser, "用户偏好"},
		{memory.LayerSession, "会话"},
		{memory.LayerLongTerm, "长期"},
	}
	for _, c := range cases {
		if got := layerDisplayName(c.layer); got != c.want {
			t.Errorf("layerDisplayName(%q) = %q, want %q", c.layer, got, c.want)
		}
	}
}
