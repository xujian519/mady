package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServer_Resubscribe(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewClient(ts.URL)

	// Send a task first
	_, err := client.SendTask(context.Background(), SendTaskRequest{
		ID:      "resub-task",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Publish some events to history
	server.PublishTaskUpdate("resub-task", &TaskUpdateEvent{
		Result: &Task{ID: "resub-task", State: TaskStateWorking},
	})
	server.PublishTaskUpdate("resub-task", &TaskUpdateEvent{
		Result: &Task{ID: "resub-task", State: TaskStateCompleted},
		Final:  true,
	})

	// Resubscribe and replay
	stream, err := client.ResubscribeTask(context.Background(), "resub-task")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var events []*TaskUpdateEvent
	for {
		ev, ok := stream.Recv()
		if !ok {
			break
		}
		events = append(events, ev)
		if ev.Final {
			break
		}
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Result.State != TaskStateWorking {
		t.Fatalf("expected working (initial), got %s", events[0].Result.State)
	}
	if events[1].Result.State != TaskStateWorking {
		t.Fatalf("expected working, got %s", events[1].Result.State)
	}
	if events[2].Result.State != TaskStateCompleted {
		t.Fatalf("expected completed, got %s", events[2].Result.State)
	}
	if !events[2].Final {
		t.Fatalf("expected final event")
	}
}

func TestServer_AuthAPIKey(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler, WithAuth(AuthConfig{APIKey: "secret123"}))

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Request without key should fail
	resp, err := postJSON(ts.URL+"/", JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tasks/send", Params: mustJSON(SendTaskRequest{ID: "auth-test", Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}}})})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil || resp.Error.Message != "unauthorized" {
		t.Fatalf("expected unauthorized, got %v", resp.Error)
	}

	// Request with key should succeed
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/", strings.NewReader(string(mustJSON(JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "tasks/send", Params: mustJSON(SendTaskRequest{ID: "auth-test2", Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}}})}))))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "secret123")
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer httpResp.Body.Close()

	var resp2 JSONRPCResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp2); err != nil {
		t.Fatal(err)
	}
	if resp2.Error != nil {
		t.Fatalf("unexpected error: %v", resp2.Error)
	}
}

func TestServer_AuthBearer(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler, WithAuth(AuthConfig{BearerToken: "tok_abc"}))

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Request without token should fail
	resp, err := postJSON(ts.URL+"/", JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tasks/send", Params: mustJSON(SendTaskRequest{ID: "auth-test", Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}}})})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil || resp.Error.Message != "unauthorized" {
		t.Fatalf("expected unauthorized, got %v", resp.Error)
	}

	// Request with token should succeed
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/", strings.NewReader(string(mustJSON(JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "tasks/send", Params: mustJSON(SendTaskRequest{ID: "auth-test2", Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}}})}))))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok_abc")
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer httpResp.Body.Close()

	var resp2 JSONRPCResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp2); err != nil {
		t.Fatal(err)
	}
	if resp2.Error != nil {
		t.Fatalf("unexpected error: %v", resp2.Error)
	}
}

func TestPushNotifier(t *testing.T) {
	var received bool
	var receivedTaskID string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if id, ok := payload["taskId"].(string); ok {
			receivedTaskID = id
		}
		// Verify auth header
		if r.Header.Get("Authorization") != "Bearer webhook-token" {
			t.Errorf("expected Bearer webhook-token, got %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	notifier := NewPushNotifier().WithAllowPrivate(true)
	task := &Task{ID: "push-task-1", State: TaskStateCompleted}
	cfg := &PushNotificationConfig{
		URL:   ts.URL,
		Token: "webhook-token",
	}

	ctx := context.Background()
	if err := notifier.Notify(ctx, cfg, task); err != nil {
		t.Fatal(err)
	}

	if !received {
		t.Fatal("webhook was not called")
	}
	if receivedTaskID != "push-task-1" {
		t.Fatalf("expected task id push-task-1, got %s", receivedTaskID)
	}
}

func TestSSRFSafeDialer_RefusesPrivateIPLiteral(t *testing.T) {
	d := newSSRFSafeDialer(false, nil)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	conn, err := d.DialContext(context.Background(), "tcp", ln.Addr().String())
	if err == nil {
		conn.Close()
		t.Fatal("expected dial to private IP literal to be refused")
	}
}

func TestSSRFSafeDialer_AllowsPrivateWhenFlagged(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan struct{})
	go func() {
		c, _ := ln.Accept()
		if c != nil {
			c.Close()
		}
		close(done)
	}()
	d := newSSRFSafeDialer(true, nil)
	conn, err := d.DialContext(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("expected dial to succeed with allowPrivate=true, got %v", err)
	}
	conn.Close()
	<-done
}

// mockResolver 是 ipResolver 的测试替身，按预设的 addrs/err 响应 LookupIPAddr。
// calls 字段记录被调用次数，用于断言 dialer 真正走了 DNS 解析路径。
type mockResolver struct {
	addrs []net.IPAddr
	err   error
	calls int
}

func (m *mockResolver) LookupIPAddr(_ context.Context, _ string) ([]net.IPAddr, error) {
	m.calls++
	return m.addrs, m.err
}

// TestSSRFSafeDialer_RefusesWhenDNSResolvesOnlyToPrivate 覆盖 DNS rebinding 核心
// 攻击向量：域名解析返回的全是私网 IP（如 127.0.0.1/10.0.0.1/169.254.169.254
// 云元数据服务），dialer 必须拒绝。这是 happy-path IP 字面量测试无法覆盖的场景。
func TestSSRFSafeDialer_RefusesWhenDNSResolvesOnlyToPrivate(t *testing.T) {
	mock := &mockResolver{addrs: []net.IPAddr{
		{IP: net.ParseIP("127.0.0.1")},
		{IP: net.ParseIP("10.0.0.1")},
		{IP: net.ParseIP("169.254.169.254")}, // AWS 元数据服务
	}}
	d := newSSRFSafeDialer(false, mock)
	_, err := d.DialContext(context.Background(), "tcp", "evil-rebind.example:80")
	if err == nil {
		t.Fatal("expected dial to fail when DNS resolves only to private IPs")
	}
	if mock.calls != 1 {
		t.Errorf("expected resolver to be called once, got %d", mock.calls)
	}
	// 错误消息必须明确是 SSRF 防护触发，不是其它网络错误
	if !strings.Contains(err.Error(), "private") {
		t.Errorf("expected error to mention private IPs, got: %v", err)
	}
}

// TestSSRFSafeDialer_PrefersPublicIPFromMixedDNSResults 验证当 DNS 返回
// 私网+公网混合 IP 时，dialer 选公网 IP 直连（绕开私网陷阱）。
// 用 192.0.2.1（RFC 5737 TEST-NET-1，保证不可路由但不在私网列表）模拟公网 IP，
// dial 必然失败但错误消息应包含该 IP，证明 dialer 用它发起连接。
func TestSSRFSafeDialer_PrefersPublicIPFromMixedDNSResults(t *testing.T) {
	mock := &mockResolver{addrs: []net.IPAddr{
		{IP: net.ParseIP("127.0.0.1")}, // 私网，应被跳过
		{IP: net.ParseIP("192.0.2.1")}, // TEST-NET-1，视为公网
	}}
	d := newSSRFSafeDialer(false, mock)
	// 192.0.2.1 不可路由，短超时避免测试阻塞
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := d.DialContext(ctx, "tcp", "mixed.example:80")
	if err == nil {
		// 极罕见：若真有服务监听 192.0.2.1:80 也算 dialer 选对了 IP
		return
	}
	if mock.calls != 1 {
		t.Errorf("expected resolver to be called once, got %d", mock.calls)
	}
	// 错误消息必须包含 "192.0.2.1"，证明 dialer 选了公网 IP dial（而非被私网检查拒）
	if !strings.Contains(err.Error(), "192.0.2.1") {
		t.Errorf("expected error to contain public IP 192.0.2.1 (chosen for dial), got: %v", err)
	}
	// 且不应是 SSRF 私网拒绝（说明不是被防护层挡下）
	if strings.Contains(err.Error(), "private") {
		t.Errorf("dial should not be blocked by private-IP guard, got: %v", err)
	}
}

// TestSSRFSafeDialer_ResolverErrorPropagated 验证 DNS 解析失败时错误被正确包装。
func TestSSRFSafeDialer_ResolverErrorPropagated(t *testing.T) {
	mock := &mockResolver{err: fmt.Errorf("dns server down")}
	d := newSSRFSafeDialer(false, mock)
	_, err := d.DialContext(context.Background(), "tcp", "fail.example:80")
	if err == nil {
		t.Fatal("expected dial to fail when resolver errors")
	}
	if !strings.Contains(err.Error(), "dns server down") {
		t.Errorf("expected error to wrap resolver error, got: %v", err)
	}
}

// TestSSRFSafeDialer_SetAllowPrivateIsAtomic 验证 SetAllowPrivate 可在 DialContext
// 之外并发调用（验证 atomic.Bool 提供的线程安全契约）。
func TestSSRFSafeDialer_SetAllowPrivateIsAtomic(t *testing.T) {
	d := newSSRFSafeDialer(false, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			d.SetAllowPrivate(i%2 == 0)
		}
	}()
	// 并发读 + 写不应触发 race detector
	for i := 0; i < 100; i++ {
		_ = d.allowPrivate.Load()
	}
	<-done
}

func TestValidateWebhookURL_PrivateIPRejected(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1:8080/hook",
		"http://10.0.0.1/hook",
		"http://192.168.1.1/hook",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]/hook",
	} {
		if err := validateWebhookURL(raw); err == nil {
			t.Errorf("validateWebhookURL(%q) = nil, want error", raw)
		}
	}
}

func TestNotify_PrivateWebhookURLRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	notifier := NewPushNotifier()
	cfg := &PushNotificationConfig{URL: "http://" + ln.Addr().String() + "/hook"}
	task := &Task{ID: "ssrf-task", State: TaskStateSubmitted}
	err = notifier.Notify(context.Background(), cfg, task)
	if err == nil {
		t.Fatal("expected Notify to private webhook to fail, got nil")
	}
}
