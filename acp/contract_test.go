package acp_test

// 本文件是协议面契约护栏（M67-2）。契约清单见
// docs/code-health/protocol-contract-baseline.md §3.4。
//
// ACP 的方法名、protocolVersion、错误码、stdio 行帧格式、session/update 通知
// 都是与编辑器客户端（Zed 等）互操作的线协议。断言写成逐字比对，改动须走
// 协议变更流程，不能在重构卡里顺手改。

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/xujian519/mady/acp"
	"github.com/xujian519/mady/agentcore"
)

// jsonrpcMethodNotFound 是 JSON-RPC 标准码 -32601。ACP 包内以字面量使用，
// 此处在契约测试中单独命名，便于失败信息可读。
const jsonrpcMethodNotFound = -32601

// TestContractACPProtocolVersion 锁定 initialize 结果的 protocolVersion 与帧格式。
func TestContractACPProtocolVersion(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}` + "\n"
	srv, output := newTestServer(t, input, nil)
	runServerToEOF(t, srv)

	frames := contractFrames(t, output.Bytes())
	if len(frames) != 1 {
		t.Fatalf("期望 1 帧响应，实际 %d 帧", len(frames))
	}
	if got := frames[0]["jsonrpc"]; got != "2.0" {
		t.Fatalf("jsonrpc = %v，契约要求 2.0", got)
	}
	if frames[0]["error"] != nil {
		t.Fatalf("initialize 失败：%v", frames[0]["error"])
	}
	result, ok := frames[0]["result"].(map[string]any)
	if !ok {
		t.Fatalf("result 不是对象：%#v", frames[0]["result"])
	}
	if got := result["protocolVersion"]; got != float64(acp.ProtocolVersion) {
		t.Fatalf("protocolVersion = %v，契约要求 %d", got, acp.ProtocolVersion)
	}
}

// TestContractACPMethodRegistry 锁定 11 个 JSON-RPC 方法名。
// 用空参数触发业务层错误，只要不是 -32601 即说明方法已登记。
func TestContractACPMethodRegistry(t *testing.T) {
	methods := []string{
		"initialize",
		"authenticate",
		"session/new",
		"session/load",
		"session/resume",
		"session/fork",
		"session/list",
		"session/prompt",
		"session/cancel",
		"session/set_mode",
		"session/set_model",
	}
	if len(methods) != 11 {
		t.Fatalf("ACP 应有 11 个方法，当前 fixture 为 %d", len(methods))
	}

	var input bytes.Buffer
	for i, method := range methods {
		line := map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": method, "params": map[string]any{}}
		data, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		input.Write(data)
		input.WriteByte('\n')
	}

	srv, output := newTestServer(t, input.String(), nil)
	runServerToEOF(t, srv)

	frames := contractFrames(t, output.Bytes())
	if len(frames) != len(methods) {
		t.Fatalf("期望 %d 帧响应，实际 %d 帧", len(methods), len(frames))
	}
	for i, method := range methods {
		if code, ok := contractErrorCode(frames[i]); ok && code == jsonrpcMethodNotFound {
			t.Fatalf("方法 %q 被判为不存在（-32601），契约要求已登记", method)
		}
	}
}

// TestContractACPErrorCodes 锁定 ACP 错误码与消息文案。
func TestContractACPErrorCodes(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		wantCode    int
		wantMessage string
	}{
		{
			name:        "方法不存在",
			input:       `{"jsonrpc":"2.0","id":1,"method":"unknown/method","params":{}}`,
			wantCode:    -32601,
			wantMessage: "Method not found",
		},
		{
			name:        "参数非法",
			input:       `{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":"bad"}`,
			wantCode:    -32602,
			wantMessage: "Invalid params",
		},
		{
			// NewServer 在未配置真实 AuthProvider 时注入 noop 实现（认证必然
			// 失败，fail-closed），因此可观察到的文案是 Authentication failed。
			name:        "认证失败",
			input:       `{"jsonrpc":"2.0","id":1,"method":"authenticate","params":{}}`,
			wantCode:    -32001,
			wantMessage: "Authentication failed",
		},
		{
			name:        "会话不存在",
			input:       `{"jsonrpc":"2.0","id":1,"method":"session/load","params":{"sessionId":"missing","cwd":"/tmp"}}`,
			wantCode:    -32002,
			wantMessage: "Session not found",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, output := newTestServer(t, c.input+"\n", nil)
			runServerToEOF(t, srv)

			frames := contractFrames(t, output.Bytes())
			if len(frames) != 1 {
				t.Fatalf("期望 1 帧响应，实际 %d 帧", len(frames))
			}
			rpcErr, ok := frames[0]["error"].(map[string]any)
			if !ok {
				t.Fatalf("期望错误帧，实际 %#v", frames[0])
			}
			if got := rpcErr["code"]; got != float64(c.wantCode) {
				t.Fatalf("错误码 = %v，契约要求 %d", got, c.wantCode)
			}
			if got := rpcErr["message"]; got != c.wantMessage {
				t.Fatalf("错误消息 = %v，契约要求 %q", got, c.wantMessage)
			}
		})
	}

	t.Run("认证门禁", func(t *testing.T) {
		input := `{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/tmp"}}` + "\n"
		srv, output := newAuthTestServer(t, input, acp.NewTokenAuthProvider("secret"))
		runServerToEOF(t, srv)

		frames := contractFrames(t, output.Bytes())
		if len(frames) != 1 {
			t.Fatalf("期望 1 帧响应，实际 %d 帧", len(frames))
		}
		rpcErr, ok := frames[0]["error"].(map[string]any)
		if !ok {
			t.Fatalf("期望错误帧，实际 %#v", frames[0])
		}
		if got := rpcErr["code"]; got != float64(-32000) {
			t.Fatalf("错误码 = %v，契约要求 -32000", got)
		}
		if got := rpcErr["message"]; got != "Authentication required" {
			t.Fatalf("错误消息 = %v，契约要求 %q", got, "Authentication required")
		}
	})
}

// TestContractACPSessionUpdateNotification 锁定 session/update 通知的帧形态：
// 通知帧无 id 字段，params 为 SessionNotification（sessionId + update.sessionUpdate）。
func TestContractACPSessionUpdateNotification(t *testing.T) {
	agent := agentcore.New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "acp-contract",
			Model:    "stub",
			Provider: lengthProvider{},
		},
		ExecutionConfig: agentcore.ExecutionConfig{MaxTurns: 10},
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	sm := acp.NewSessionManager(acp.SessionManagerConfig{
		AgentFactory: &truncationAgentFactory{agent: agent},
		Logger:       logger,
	})
	if _, err := sm.CreateSession(context.Background(), "/tmp", "s1"); err != nil {
		t.Fatalf("CreateSession 失败：%v", err)
	}

	output := &safeBuffer{}
	input := `{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":{"sessionId":"s1","prompt":[{"type":"text","text":"hi"}]}}` + "\n"
	srv := acp.NewServer(acp.ServerConfig{
		SessionManager: sm,
		AgentInfo:      acp.AgentInfo{Name: "test", Version: "1.0"},
		Reader:         bytes.NewReader([]byte(input)),
		Writer:         output,
		Logger:         logger,
	})
	go func() { _ = srv.Run(context.Background()) }()

	if resp := waitForJSONRPCResponse(t, output, "1"); resp == nil {
		t.Fatal("等待 prompt 响应超时")
	} else if resp.Error != nil {
		t.Fatalf("prompt 失败：%s", resp.Error.Message)
	}

	frames := contractFrames(t, output.Bytes())
	var kinds []string
	sawNotification := false
	for _, frame := range frames {
		if got := frame["jsonrpc"]; got != "2.0" {
			t.Fatalf("帧的 jsonrpc = %v，契约要求 2.0", got)
		}
		if frame["method"] != "session/update" {
			continue
		}
		sawNotification = true
		if _, hasID := frame["id"]; hasID {
			t.Fatalf("通知帧不应带 id 字段：%#v", frame)
		}
		rawParams, err := json.Marshal(frame["params"])
		if err != nil {
			t.Fatal(err)
		}
		var notif acp.SessionNotification
		if err := json.Unmarshal(rawParams, &notif); err != nil {
			t.Fatalf("解析 SessionNotification 失败：%v", err)
		}
		if notif.SessionID != "s1" {
			t.Fatalf("通知 sessionId = %q，契约要求 s1", notif.SessionID)
		}
		kinds = append(kinds, notif.Update.SessionUpdate)
	}
	if !sawNotification {
		t.Fatal("未观察到 session/update 通知帧")
	}
	for _, want := range []string{"user_message_chunk", "agent_message_chunk"} {
		found := false
		for _, kind := range kinds {
			if kind == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("缺少 sessionUpdate=%q 通知，实际序列 %v", want, kinds)
		}
	}
}

// contractFrames 把 stdio 输出按行解码为原始 JSON 帧，同时校验「每帧一行」。
func contractFrames(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	frames := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var frame map[string]any
		if err := json.Unmarshal(line, &frame); err != nil {
			t.Fatalf("帧不是独立 JSON 行：%v（原始：%s）", err, line)
		}
		frames = append(frames, frame)
	}
	return frames
}

// contractErrorCode 提取帧中的 JSON-RPC 错误码。
func contractErrorCode(frame map[string]any) (int, bool) {
	rpcErr, ok := frame["error"].(map[string]any)
	if !ok {
		return 0, false
	}
	code, ok := rpcErr["code"].(float64)
	if !ok {
		return 0, false
	}
	return int(code), true
}
