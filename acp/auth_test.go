package acp_test

// C1/C3 安全修复回归测试：
//   - C1：配置 AuthProvider 后，authenticate 之前的会话方法必须被拒绝；
//     令牌正确时认证放行；未配置 provider 时保持本地开发体验（放行）。
//   - C3：客户端声明的 FS 能力必须在服务端白名单内，否则 initialize 拒绝。

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xujian519/mady/acp"
)

// newAuthTestServer 构造带指定 AuthProvider 的测试服务器，input 为模拟的
// stdio JSON-RPC 输入流，返回服务器与输出缓冲。
func newAuthTestServer(t *testing.T, input string, prov acp.AuthProvider) (*acp.Server, *bytes.Buffer) {
	t.Helper()
	sm := newTestManager(t, &stubAgentFactory{}, nil)
	output := &bytes.Buffer{}
	srv := acp.NewServer(acp.ServerConfig{
		SessionManager: sm,
		AgentInfo:      acp.AgentInfo{Name: "test", Version: "1.0"},
		AuthProvider:   prov,
		Reader:         bytes.NewReader([]byte(input)),
		Writer:         output,
		Logger:         testLogger(t),
	})
	return srv, output
}

func runServerToEOF(t *testing.T, srv *acp.Server) {
	t.Helper()
	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("server run failed: %v", err)
	}
}

func TestACPAuthGateRejectsSessionBeforeAuthenticate(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/tmp"}}` + "\n"
	srv, output := newAuthTestServer(t, input, acp.NewTokenAuthProvider("secret"))
	runServerToEOF(t, srv)

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(resps))
	}
	// initialize 应成功并声明 token 认证方式。
	if resps[0].Error != nil {
		t.Fatalf("initialize failed: %v", resps[0].Error)
	}
	var initResult struct {
		AuthMethods []map[string]any `json:"authMethods"`
	}
	if err := json.Unmarshal(resps[0].Result, &initResult); err != nil {
		t.Fatalf("parse initialize result: %v", err)
	}
	if len(initResult.AuthMethods) == 0 {
		t.Fatal("expected authMethods advertised when token auth is configured")
	}
	// session/new 在认证前必须被拒绝。
	if resps[1].Error == nil {
		t.Fatal("expected session/new to be rejected before authenticate")
	}
	if resps[1].Error.Code != -32000 {
		t.Fatalf("expected -32000 Authentication required, got %d: %s",
			resps[1].Error.Code, resps[1].Error.Message)
	}
}

func TestACPAuthenticateTokenFlow(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"authenticate","params":{"methodId":"token","token":"wrong"}}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"session/new","params":{"cwd":"/tmp"}}` + "\n" +
		`{"jsonrpc":"2.0","id":4,"method":"authenticate","params":{"methodId":"token","token":"secret"}}` + "\n" +
		`{"jsonrpc":"2.0","id":5,"method":"session/new","params":{"cwd":"/tmp"}}` + "\n"
	srv, output := newAuthTestServer(t, input, acp.NewTokenAuthProvider("secret"))
	runServerToEOF(t, srv)

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 5 {
		t.Fatalf("expected 5 responses, got %d", len(resps))
	}
	if resps[1].Error == nil {
		t.Fatal("expected wrong token to be rejected")
	}
	if resps[2].Error == nil || resps[2].Error.Code != -32000 {
		t.Fatal("expected session/new still rejected after failed authenticate")
	}
	if resps[3].Error != nil {
		t.Fatalf("expected correct token to authenticate, got %v", resps[3].Error)
	}
	if resps[4].Error != nil {
		t.Fatalf("expected session/new to succeed after authenticate, got %v", resps[4].Error)
	}
}

func TestACPAuthenticateRejectsUnknownMethod(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"authenticate","params":{"methodId":"oauth","token":"secret"}}` + "\n"
	srv, output := newAuthTestServer(t, input, acp.NewTokenAuthProvider("secret"))
	runServerToEOF(t, srv)

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 1 || resps[0].Error == nil {
		t.Fatal("expected unknown auth method to be rejected")
	}
}

func TestACPNoopAuthAllowsLocalDevelopment(t *testing.T) {
	// 未配置 AuthProvider（nil → noop）时保持本地开发体验：会话方法直接可用。
	input := `{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/tmp"}}` + "\n"
	srv, output := newAuthTestServer(t, input, nil)
	runServerToEOF(t, srv)

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Fatalf("expected session/new allowed without auth in local dev, got %v", resps[0].Error)
	}
}

func TestACPClientCapabilitiesFailClosed(t *testing.T) {
	// C3：未配置 FS 白名单时，客户端声明 FS 能力必须被拒绝。
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,` +
		`"clientCapabilities":{"fs":{"readTextFile":true,"writeTextFile":true}}}}` + "\n"
	srv, output := newAuthTestServer(t, input, nil)
	runServerToEOF(t, srv)

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 1 || resps[0].Error == nil {
		t.Fatal("expected initialize with FS capabilities to be rejected without allowlist")
	}
	if !strings.Contains(resps[0].Error.Message, "capabilities") {
		t.Fatalf("unexpected error message: %s", resps[0].Error.Message)
	}
}

func TestACPClientCapabilitiesAllowlist(t *testing.T) {
	// 白名单内的能力放行，白名单外的能力拒绝。
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,` +
		`"clientCapabilities":{"fs":{"readTextFile":true,"writeTextFile":true}}}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":1,` +
		`"clientCapabilities":{"fs":{"readTextFile":true}}}}` + "\n"
	sm := newTestManager(t, &stubAgentFactory{}, nil)
	output := &bytes.Buffer{}
	srv := acp.NewServer(acp.ServerConfig{
		SessionManager:        sm,
		AgentInfo:             acp.AgentInfo{Name: "test", Version: "1.0"},
		Reader:                bytes.NewReader([]byte(input)),
		Writer:                output,
		Logger:                testLogger(t),
		AllowedFSCapabilities: map[string]bool{"FS.ReadTextFile": true},
	})
	runServerToEOF(t, srv)

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(resps))
	}
	if resps[0].Error == nil {
		t.Fatal("expected writeTextFile capability to be rejected")
	}
	if resps[1].Error != nil {
		t.Fatalf("expected readTextFile-only capabilities to be accepted, got %v", resps[1].Error)
	}
}
