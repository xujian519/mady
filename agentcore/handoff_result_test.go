package agentcore

import (
	"testing"
)

func TestParseHandoffResult_PureJSON(t *testing.T) {
	output := `{"action":"检索专利信息","result":"找到了3篇相关专利","success":true}`

	hr, ok := ParseHandoffResult(output)
	if !ok {
		t.Fatal("expected to parse pure JSON HandoffResult")
	}
	if hr.Action != "检索专利信息" {
		t.Errorf("Action = %q", hr.Action)
	}
	if hr.Result != "找到了3篇相关专利" {
		t.Errorf("Result = %q", hr.Result)
	}
	if !hr.Success {
		t.Error("expected Success=true")
	}
}

func TestParseHandoffResult_MarkdownCodeBlock(t *testing.T) {
	output := "```json\n{\"action\":\"法律分析\",\"result\":\"根据民法典第584条...\",\"success\":true}\n```"

	hr, ok := ParseHandoffResult(output)
	if !ok {
		t.Fatal("expected to parse markdown-wrapped HandoffResult")
	}
	if hr.Action != "法律分析" {
		t.Errorf("Action = %q", hr.Action)
	}
}

func TestParseHandoffResult_WithNeedsFollowup(t *testing.T) {
	output := `{"action":"起草合同","result":"需要确认合同金额","success":true,"needs_followup":true}`

	hr, ok := ParseHandoffResult(output)
	if !ok {
		t.Fatal("expected to parse")
	}
	if !hr.NeedsFollowup {
		t.Error("expected NeedsFollowup=true")
	}
}

func TestParseHandoffResult_Failure(t *testing.T) {
	output := `{"action":"执行失败","result":"处理遇到问题，请稍后重试","success":false,"needs_followup":true}`

	hr, ok := ParseHandoffResult(output)
	if !ok {
		t.Fatal("expected to parse failure result")
	}
	if hr.Success {
		t.Error("expected Success=false")
	}
	if !hr.IsFailure() {
		t.Error("IsFailure() should return true")
	}
}

func TestParseHandoffResult_PlainText(t *testing.T) {
	// 普通文本不应该被解析为 HandoffResult
	output := "根据您的需求，分析结果如下：专利 CN109690000A 具有新颖性。"

	_, ok := ParseHandoffResult(output)
	if ok {
		t.Fatal("plain text should not be parsed as HandoffResult")
	}
}

func TestParseHandoffResult_EmptyJSON(t *testing.T) {
	output := `{"action":"","result":"","success":false}`

	_, ok := ParseHandoffResult(output)
	if ok {
		t.Fatal("empty action and result should not be parsed as valid HandoffResult")
	}
}

func TestNewHandoffResult(t *testing.T) {
	hr := NewHandoffResult("检索完成", "找到5条匹配结果")
	if !hr.Success {
		t.Error("NewHandoffResult should set Success=true")
	}
	if hr.Action != "检索完成" {
		t.Errorf("Action = %q", hr.Action)
	}
}

func TestNewFailureResult(t *testing.T) {
	hr := NewFailureResult("搜索失败", "请稍后重试")
	if hr.Success {
		t.Error("NewFailureResult should set Success=false")
	}
	if !hr.NeedsFollowup {
		t.Error("NewFailureResult should set NeedsFollowup=true")
	}
}

func TestToHandoffResultJSON(t *testing.T) {
	hr := HandoffResult{
		Action:  "检索完成",
		Result:  "找到结果",
		Success: true,
	}

	jsonStr := hr.ToHandoffResultJSON()
	if jsonStr == "" {
		t.Fatal("expected non-empty JSON")
	}

	// 验证可以反序列化
	parsed, ok := ParseHandoffResult(jsonStr)
	if !ok {
		t.Fatal("expected to parse serialized HandoffResult")
	}
	if parsed.Action != "检索完成" {
		t.Errorf("roundtrip failed: Action = %q", parsed.Action)
	}
}

func TestParseHandoffResult_WhitespaceAroundJSON(t *testing.T) {
	output := "  \n{\"action\":\"测试\",\"result\":\"成功\",\"success\":true}\n  "

	hr, ok := ParseHandoffResult(output)
	if !ok {
		t.Fatal("expected to parse JSON with surrounding whitespace")
	}
	if hr.Action != "测试" {
		t.Errorf("Action = %q", hr.Action)
	}
}
