package agentcore

import (
	"strings"
	"testing"
)

func TestExtractEntities_PatentNumber(t *testing.T) {
	text := "请帮我分析专利 CN109690000A 的新颖性"
	entities := extractEntities(text)

	patentNo, ok := entities["patent_no"]
	if !ok {
		t.Fatal("expected patent_no to be extracted")
	}
	if patentNo != "CN109690000A" {
		t.Fatalf("patent_no = %q, want %q", patentNo, "CN109690000A")
	}
}

func TestExtractEntities_AppNumber(t *testing.T) {
	text := "申请号 2024101234567 的相关信息"
	entities := extractEntities(text)

	if _, ok := entities["app_no"]; !ok {
		t.Fatal("expected app_no to be extracted")
	}
}

func TestExtractEntities_PCTAppNumber(t *testing.T) {
	text := "PCT/CN2024/123456 的国际阶段检索报告"
	entities := extractEntities(text)

	pctAppNo, ok := entities["pct_app_no"]
	if !ok {
		t.Fatal("expected pct_app_no to be extracted")
	}
	if pctAppNo != "PCT/CN2024/123456" {
		t.Fatalf("pct_app_no = %q, want %q", pctAppNo, "PCT/CN2024/123456")
	}
}

func TestExtractEntities_CaseID(t *testing.T) {
	text := "案件 AB2024-0001 的最新进展"
	entities := extractEntities(text)

	caseID, ok := entities["case_id"]
	if !ok {
		t.Fatal("expected case_id to be extracted")
	}
	if caseID != "AB2024-0001" {
		t.Fatalf("case_id = %q, want %q", caseID, "AB2024-0001")
	}
}

func TestExtractEntities_Multiple(t *testing.T) {
	text := "分析专利 CN109690000A 和案件 AB2024-0001 的关联性"
	entities := extractEntities(text)

	if len(entities) < 2 {
		t.Fatalf("expected at least 2 entities, got %d: %v", len(entities), entities)
	}
	if entities["patent_no"] != "CN109690000A" {
		t.Errorf("patent_no mismatch: %q", entities["patent_no"])
	}
	if entities["case_id"] != "AB2024-0001" {
		t.Errorf("case_id mismatch: %q", entities["case_id"])
	}
}

func TestExtractEntities_EmptyText(t *testing.T) {
	entities := extractEntities("")
	if entities != nil {
		t.Fatalf("expected nil entities for empty text, got %v", entities)
	}
}

func TestExtractEntities_NoMatch(t *testing.T) {
	entities := extractEntities("你好，今天天气怎么样")
	if entities != nil {
		t.Fatalf("expected nil entities for text without matches, got %v", entities)
	}
}

func TestLastUserMessage(t *testing.T) {
	msgs := []Message{
		{Role: RoleSystem, Content: "你是助手"},
		{Role: RoleUser, Content: "第一个问题"},
		{Role: RoleAssistant, Content: "第一个回答"},
		{Role: RoleUser, Content: "第二个问题"},
	}

	got := LastUserMessage(msgs)
	if got != "第二个问题" {
		t.Fatalf("LastUserMessage = %q, want %q", got, "第二个问题")
	}
}

func TestLastUserMessage_NoUser(t *testing.T) {
	msgs := []Message{
		{Role: RoleSystem, Content: "你是助手"},
		{Role: RoleAssistant, Content: "你好"},
	}

	got := LastUserMessage(msgs)
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestLastUserMessage_Empty(t *testing.T) {
	got := LastUserMessage(nil)
	if got != "" {
		t.Fatalf("expected empty string for nil, got %q", got)
	}
}

func TestLastN(t *testing.T) {
	msgs := make([]Message, 10)
	for i := range msgs {
		msgs[i] = Message{Role: RoleUser, Content: string(rune('A' + i))}
	}

	got := lastN(msgs, 3)
	if len(got) != 3 {
		t.Fatalf("lastN len = %d, want 3", len(got))
	}
	if got[0].Content != "H" || got[2].Content != "J" {
		t.Fatalf("lastN returned wrong slice: %v", got)
	}
}

func TestLastN_NLargerThanLen(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "A"},
		{Role: RoleUser, Content: "B"},
	}

	got := lastN(msgs, 10)
	if len(got) != 2 {
		t.Fatalf("lastN len = %d, want 2", len(got))
	}
}

func TestLastN_ZeroN(t *testing.T) {
	msgs := []Message{{Role: RoleUser, Content: "A"}}
	got := lastN(msgs, 0)
	if got != nil {
		t.Fatal("expected nil for n=0")
	}
}

func TestExtractHandoffContext_Basic(t *testing.T) {
	// 创建一个带状态的 Agent
	agent := New(StubConfig(&stubProvider{}, WithName("test-agent")))
	defer agent.Close()

	// 手动填充消息历史
	agent.state.AddMessage(Message{Role: RoleSystem, Content: "你是测试助手"})
	agent.state.AddMessage(Message{Role: RoleUser, Content: "分析专利 CN109690000A"})
	agent.state.AddMessage(Message{Role: RoleAssistant, Content: "好的，正在分析"})

	ctx := agent.ExtractHandoffContext("patent-agent", 2)

	if ctx.FromAgent != "test-agent" {
		t.Fatalf("FromAgent = %q, want %q", ctx.FromAgent, "test-agent")
	}
	if ctx.ToAgent != "patent-agent" {
		t.Fatalf("ToAgent = %q, want %q", ctx.ToAgent, "patent-agent")
	}
	if ctx.UserIntent != "分析专利 CN109690000A" {
		t.Fatalf("UserIntent = %q, want %q", ctx.UserIntent, "分析专利 CN109690000A")
	}
	if _, ok := ctx.ExtractedEntities["patent_no"]; !ok {
		t.Error("expected patent_no in extracted entities")
	}
	if len(ctx.RecentMessages) != 2 {
		t.Fatalf("RecentMessages len = %d, want 2", len(ctx.RecentMessages))
	}
	if ctx.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestExtractHandoffContext_RecentNDefault(t *testing.T) {
	agent := New(StubConfig(&stubProvider{}, WithName("test-agent")))
	defer agent.Close()

	agent.state.AddMessage(Message{Role: RoleUser, Content: "你好"})

	ctx := agent.ExtractHandoffContext("chat-agent", 0) // 0 → 使用默认值 6

	// recentN=0 时使用默认 6，只有 1 条消息所以返回 1 条
	if len(ctx.RecentMessages) != 1 {
		t.Fatalf("RecentMessages len = %d, want 1", len(ctx.RecentMessages))
	}
}

// TestJoinMessageText 测试消息文本拼接
func TestJoinMessageText(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "你好"},
		{Role: RoleAssistant, Content: "你好，有什么可以帮助的？"},
	}
	text := joinMessageText(msgs)
	if !strings.Contains(text, "你好") {
		t.Fatal("expected joined text to contain message content")
	}
}

// TestExtractHandoffContext_WithCaseID 测试法律案件编号抽取
func TestExtractHandoffContext_WithCaseID(t *testing.T) {
	agent := New(StubConfig(&stubProvider{}, WithName("chat-agent")))
	defer agent.Close()

	agent.state.AddMessage(Message{Role: RoleSystem, Content: "你是测试助手"})
	agent.state.AddMessage(Message{Role: RoleUser, Content: "查询案件 AB2024-0001 的判例"})

	ctx := agent.ExtractHandoffContext("legal-advisor", 3)

	if caseID, ok := ctx.ExtractedEntities["case_id"]; !ok || caseID != "AB2024-0001" {
		t.Fatalf("expected case_id=AB2024-0001, got %q (ok=%v)", caseID, ok)
	}
}
