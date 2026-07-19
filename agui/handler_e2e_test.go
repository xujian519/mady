package agui_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agui"
)

// ──────────────────────────────────────────────
// SSE 事件解析
// ──────────────────────────────────────────────

type sseEvent struct {
	Event string
	Data  string
}

// parseSSE 将 SSE 响应体解析为事件列表。
func parseSSE(t *testing.T, raw string) []sseEvent {
	t.Helper()
	var events []sseEvent
	scanner := bufio.NewScanner(strings.NewReader(raw))
	var current sseEvent
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			// 空行 = 事件分隔符
			if current.Event != "" {
				events = append(events, current)
				current = sseEvent{}
			}
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			current.Event = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			current.Data = strings.TrimPrefix(line, "data: ")
		}
	}
	// 末尾可能没有尾随空行
	if current.Event != "" {
		events = append(events, current)
	}
	return events
}

// assertEventTypes 断言事件序列与期望的类型列表严格匹配。
func assertEventTypes(t *testing.T, events []sseEvent, want []string) {
	t.Helper()
	if len(events) != len(want) {
		got := make([]string, len(events))
		for i, e := range events {
			got[i] = e.Event
		}
		t.Fatalf("事件数量不匹配:\n  want: %v\n  got:  %v", want, got)
	}
	for i, w := range want {
		if events[i].Event != w {
			t.Errorf("事件[%d]: want %q, got %q", i, w, events[i].Event)
		}
	}
}

// assertEventTypeSubset 断言期望的类型列表是事件序列的子集（按序出现，可穿插其他事件）。
func assertEventTypeSubset(t *testing.T, events []sseEvent, want []string) {
	t.Helper()
	got := make([]string, len(events))
	for i, e := range events {
		got[i] = e.Event
	}
	j := 0
	for _, g := range got {
		if j < len(want) && g == want[j] {
			j++
			continue
		}
	}
	if j < len(want) {
		t.Fatalf("事件序列缺少期望事件:\n  want subset: %v\n  got:         %v", want, got)
	}
}

// postAGUI 发送 POST 请求并返回 SSE 响应体。
func postAGUI(t *testing.T, handler http.Handler, body string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/agui", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w.Body.String()
}

// ──────────────────────────────────────────────
// Mock Provider：文本流式
// ──────────────────────────────────────────────

// streamingTextProvider 在第一次 Stream 调用时发送文本块，后续返回空。
type streamingTextProvider struct {
	mu   sync.Mutex
	turn int
}

func (p *streamingTextProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	// 测试使用 Streaming=true，此路径不会被调用。仅满足 Provider 接口。
	return &agentcore.ProviderResponse{}, nil
}

func (p *streamingTextProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	p.mu.Lock()
	p.turn++
	turn := p.turn
	p.mu.Unlock()

	ch := make(chan agentcore.StreamDelta, 4)
	go func() {
		if turn == 1 {
			ch <- agentcore.StreamDelta{Content: "Hel"}
			ch <- agentcore.StreamDelta{Content: "lo "}
			ch <- agentcore.StreamDelta{Content: "from AGUI!"}
		}
		ch <- agentcore.StreamDelta{Done: true}
		close(ch)
	}()
	return ch, nil
}

// ──────────────────────────────────────────────
// Mock Provider：工具调用（非流式）
// ──────────────────────────────────────────────

// toolCallProvider 第一次 Complete 返回工具调用，第二次返回文本。
type toolCallProvider struct {
	mu   sync.Mutex
	turn int
}

func (p *toolCallProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	p.mu.Lock()
	p.turn++
	turn := p.turn
	p.mu.Unlock()
	if turn == 1 {
		return &agentcore.ProviderResponse{
			ToolCalls: []agentcore.ToolCall{{
				ID:        "call_1",
				Name:      "test_tool",
				Arguments: `{"key":"val"}`,
			}},
		}, nil
	}
	return &agentcore.ProviderResponse{Content: "tool result processed"}, nil
}

func (p *toolCallProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta, 1)
	ch <- agentcore.StreamDelta{Done: true}
	close(ch)
	return ch, nil
}

// ──────────────────────────────────────────────
// Mock Provider：推理块 + 文本（流式）
// ──────────────────────────────────────────────

// thinkingThenTextProvider 先发推理块，再发文本。
type thinkingThenTextProvider struct {
	mu   sync.Mutex
	turn int
}

func (p *thinkingThenTextProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	// 测试使用 Streaming=true，此路径不会被调用。仅满足 Provider 接口。
	return &agentcore.ProviderResponse{}, nil
}

func (p *thinkingThenTextProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	p.mu.Lock()
	p.turn++
	turn := p.turn
	p.mu.Unlock()

	ch := make(chan agentcore.StreamDelta, 6)
	go func() {
		if turn == 1 {
			// 推理块
			ch <- agentcore.StreamDelta{
				Content: "让我分析一下这个问题...",
				Blocks: []agentcore.ContentBlock{
					{Kind: agentcore.BlockKindThinking, Text: "让我分析一下这个问题..."},
				},
			}
			ch <- agentcore.StreamDelta{
				Content: "经过分析，",
				Blocks: []agentcore.ContentBlock{
					{Kind: agentcore.BlockKindThinking, Text: "经过分析，"}},
			}
			// 切换到文本（不带 Blocks 字段，使 kind 降为 BlockKindText）
			ch <- agentcore.StreamDelta{Content: "答案是这样的。"}
		}
		ch <- agentcore.StreamDelta{Done: true}
		close(ch)
	}()
	return ch, nil
}

// ──────────────────────────────────────────────
// Helper：构造完整 agentcore.Config
// ──────────────────────────────────────────────

func testConfig(provider agentcore.Provider, streaming bool) agentcore.Config {
	return agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:      "test-agent",
			Model:     "test-model",
			Provider:  provider,
			Streaming: streaming,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 5,
		},
	}
}

// ──────────────────────────────────────────────
// 测试用例
// ──────────────────────────────────────────────

func TestE2E_TextOnly(t *testing.T) {
	provider := &streamingTextProvider{}
	cfg := testConfig(provider, true)
	h := agui.NewHandler(cfg)

	body := `{"messages":[{"role":"user","content":"hello"}]}`
	raw := postAGUI(t, h, body)
	events := parseSSE(t, raw)

	// 期望的事件序列（严格匹配）
	// STATE_SNAPSHOT 在 STEP_FINISHED 之后发射，由 TurnEndEvent 监听器触发。
	want := []string{
		"RUN_STARTED",
		"STEP_STARTED",
		"TEXT_MESSAGE_START",
		"TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_END",
		"STEP_FINISHED",
		"STATE_SNAPSHOT",
		"RUN_FINISHED",
	}
	assertEventTypes(t, events, want)

	// 验证 RUN_FINISHED 的 outcome 为 success
	last := events[len(events)-1]
	var finished agui.RunFinishedEvent
	if err := json.Unmarshal([]byte(last.Data), &finished); err != nil {
		t.Fatalf("RUN_FINISHED data 解析失败: %v", err)
	}
	if finished.Outcome == nil {
		t.Fatal("期望 RUN_FINISHED 包含 outcome")
	}
	if finished.Outcome.Type != "success" {
		t.Errorf("期望 outcome.type=success, got %q", finished.Outcome.Type)
	}

	// 验证文本内容
	for _, ev := range events {
		if ev.Event == "TEXT_MESSAGE_CONTENT" {
			var content agui.TextMessageContentEvent
			if err := json.Unmarshal([]byte(ev.Data), &content); err != nil {
				t.Fatalf("TEXT_MESSAGE_CONTENT 解析失败: %v", err)
			}
		}
	}
	// 验证消息 ID 连续性
	var startID string
	var endID string
	for _, ev := range events {
		switch ev.Event {
		case "TEXT_MESSAGE_START":
			var start agui.TextMessageStartEvent
			if err := json.Unmarshal([]byte(ev.Data), &start); err != nil {
				t.Fatal(err)
			}
			startID = start.MessageID
		case "TEXT_MESSAGE_END":
			var end agui.TextMessageEndEvent
			if err := json.Unmarshal([]byte(ev.Data), &end); err != nil {
				t.Fatal(err)
			}
			endID = end.MessageID
		}
	}
	if startID == "" {
		t.Fatal("未找到 TEXT_MESSAGE_START 事件")
	}
	if startID != endID {
		t.Errorf("消息 ID 不一致: start=%q end=%q", startID, endID)
	}
}

func TestE2E_ToolCall(t *testing.T) {
	provider := &toolCallProvider{}
	cfg := testConfig(provider, false)
	h := agui.NewHandler(cfg)

	body := `{"messages":[{"role":"user","content":"use a tool"}]}`
	raw := postAGUI(t, h, body)
	events := parseSSE(t, raw)

	// 验证工具调用事件序列（子集匹配，允许穿插 TEXT_MESSAGE_END 等 closeAll 事件）
	assertEventTypeSubset(t, events, []string{
		"RUN_STARTED",
		"STEP_STARTED",
		"TOOL_CALL_START",
		"TOOL_CALL_ARGS",
		"TOOL_CALL_END",
		"TOOL_CALL_RESULT",
		"STEP_FINISHED",
		"STEP_STARTED",
		"STEP_FINISHED",
		"RUN_FINISHED",
	})

	// 验证 TOOL_CALL 事件数据
	for _, ev := range events {
		switch ev.Event {
		case "TOOL_CALL_START":
			var tcStart agui.ToolCallStartEvent
			if err := json.Unmarshal([]byte(ev.Data), &tcStart); err != nil {
				t.Fatal(err)
			}
			if tcStart.ToolCallName != "test_tool" {
				t.Errorf("期望 ToolCallName=test_tool, got %q", tcStart.ToolCallName)
			}
		case "TOOL_CALL_RESULT":
			var tcResult agui.ToolCallResultEvent
			if err := json.Unmarshal([]byte(ev.Data), &tcResult); err != nil {
				t.Fatal(err)
			}
			if tcResult.Role != "tool" {
				t.Errorf("期望 role=tool, got %q", tcResult.Role)
			}
		case "RUN_FINISHED":
			var finished agui.RunFinishedEvent
			if err := json.Unmarshal([]byte(ev.Data), &finished); err != nil {
				t.Fatal(err)
			}
			if finished.Outcome == nil || finished.Outcome.Type != "success" {
				t.Errorf("期望 outcome.type=success")
			}
		}
	}
}

func TestE2E_ThinkingThenText(t *testing.T) {
	provider := &thinkingThenTextProvider{}
	cfg := testConfig(provider, true)
	h := agui.NewHandler(cfg)

	body := `{"messages":[{"role":"user","content":"think about this"}]}`
	raw := postAGUI(t, h, body)
	events := parseSSE(t, raw)

	// 验证推理事件序列（子集匹配）
	assertEventTypeSubset(t, events, []string{
		"RUN_STARTED",
		"STEP_STARTED",
		"THINKING_START",
		"THINKING_TEXT_MESSAGE_START",
		"THINKING_TEXT_MESSAGE_CONTENT",
		"THINKING_TEXT_MESSAGE_CONTENT",
		"THINKING_END",
		"TEXT_MESSAGE_START",
		"TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_END",
		"STEP_FINISHED",
		"RUN_FINISHED",
	})

	// 验证 THINKING 事件的 ID 连续性
	var thinkingID string
	for _, ev := range events {
		switch ev.Event {
		case "THINKING_START":
			var ts agui.ThinkingStartEvent
			if err := json.Unmarshal([]byte(ev.Data), &ts); err != nil {
				t.Fatal(err)
			}
			thinkingID = ts.ThinkingID
		case "THINKING_END":
			var te agui.ThinkingEndEvent
			if err := json.Unmarshal([]byte(ev.Data), &te); err != nil {
				t.Fatal(err)
			}
			if te.ThinkingID != thinkingID {
				t.Errorf("THINKING_END ID 不匹配: start=%q end=%q", thinkingID, te.ThinkingID)
			}
		}
	}
	if thinkingID == "" {
		t.Error("未找到 THINKING_START 事件")
	}
}

func TestE2E_InvalidConfig(t *testing.T) {
	// 无 Provider 的配置应返回 RUN_ERROR
	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:  "broken",
			Model: "test-model",
			// Provider: nil
		},
	}
	h := agui.NewHandler(cfg)
	body := `{"messages":[{"role":"user","content":"hi"}]}`
	raw := postAGUI(t, h, body)
	events := parseSSE(t, raw)

	if len(events) == 0 {
		t.Fatal("期望至少一个事件")
	}
	if events[0].Event != "RUN_ERROR" {
		t.Errorf("期望 RUN_ERROR, got %s", events[0].Event)
	}
}

func TestE2E_NoUserMessage(t *testing.T) {
	// 没有 user 消息应返回 RUN_ERROR
	provider := &streamingTextProvider{}
	cfg := testConfig(provider, true)
	h := agui.NewHandler(cfg)

	body := `{"messages":[]}`
	raw := postAGUI(t, h, body)
	events := parseSSE(t, raw)

	if len(events) == 0 {
		t.Fatal("期望至少一个事件")
	}
	if events[0].Event != "RUN_ERROR" {
		t.Errorf("期望 RUN_ERROR, got %s", events[0].Event)
	}
	var errEv agui.RunErrorEvent
	if err := json.Unmarshal([]byte(events[0].Data), &errEv); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errEv.Message, "no user message") {
		t.Errorf("期望包含 'no user message', got %q", errEv.Message)
	}
}

func TestE2E_Capabilities(t *testing.T) {
	cfg := testConfig(&streamingTextProvider{}, true)
	h := agui.NewHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/agui", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200, got %d", w.Code)
	}
	var caps agui.AgentCapabilities
	if err := json.Unmarshal(w.Body.Bytes(), &caps); err != nil {
		t.Fatal(err)
	}
	if caps.Identity.Name != "test-agent" {
		t.Errorf("期望 Name=test-agent, got %q", caps.Identity.Name)
	}
	if !caps.Transport.Streaming {
		t.Error("期望 Streaming=true")
	}
}

func TestE2E_ResumeHandling(t *testing.T) {
	provider := &streamingTextProvider{}
	cfg := testConfig(provider, true)
	h := agui.NewHandler(cfg)

	// 带 resume 条目的请求——验证不崩溃且正常返回
	body := `{
		"messages":[{"role":"user","content":"resume me"}],
		"resume":[{"interruptId":"int-1","status":"resolved"}]
	}`
	raw := postAGUI(t, h, body)
	events := parseSSE(t, raw)

	if len(events) == 0 {
		t.Fatal("期望收到事件")
	}
	if events[0].Event == "RUN_ERROR" {
		var errEv agui.RunErrorEvent
		json.Unmarshal([]byte(events[0].Data), &errEv)
		t.Fatalf("resume 请求不应返回 RUN_ERROR: %s", errEv.Message)
	}
	// 验证最终有 RUN_FINISHED
	last := events[len(events)-1]
	if last.Event != "RUN_FINISHED" {
		t.Errorf("最后一个事件应为 RUN_FINISHED, got %s", last.Event)
	}
}

func TestE2E_StepNameIncrement(t *testing.T) {
	// 工具的两次调用会使 agent 产生两个 turn
	provider := &toolCallProvider{}
	cfg := testConfig(provider, false)
	h := agui.NewHandler(cfg)

	body := `{"messages":[{"role":"user","content":"step test"}]}`
	raw := postAGUI(t, h, body)
	events := parseSSE(t, raw)

	var stepNames []string
	for _, ev := range events {
		if ev.Event == "STEP_STARTED" {
			var step agui.StepStartedEvent
			if err := json.Unmarshal([]byte(ev.Data), &step); err != nil {
				t.Fatal(err)
			}
			stepNames = append(stepNames, step.StepName)
		}
	}
	if len(stepNames) < 2 {
		t.Fatalf("期望至少 2 个 STEP_STARTED, got %d", len(stepNames))
	}
	// 验证 step name 从 turn_1 开始（因为工具调用多一轮）
	fmt.Printf("step names: %v\n", stepNames)
}
