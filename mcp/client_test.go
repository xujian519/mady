package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
)

func TestStdioClient_ListToolsAndCall(t *testing.T) {
	client := newTestClient(t)
	defer func() { _ = client.Close() }()

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %#v", tools)
	}

	result, err := client.CallTool(context.Background(), "echo", map[string]any{"text": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if got := formatToolResult(result); got != "echo: hello" {
		t.Fatalf("result = %q", got)
	}
}

func TestStdioClient_ParsesCapabilities(t *testing.T) {
	client := newTestClient(t)
	defer func() { _ = client.Close() }()

	if !client.SupportsToolListChanged() {
		t.Fatal("expected tools.listChanged support")
	}
	if !client.SupportsResourceSubscribe() {
		t.Fatal("expected resources.subscribe support")
	}
	if !client.SupportsResourceListChanged() {
		t.Fatal("expected resources.listChanged support")
	}
	if !client.SupportsPromptListChanged() {
		t.Fatal("expected prompts.listChanged support")
	}
}

func TestStdioClient_ListsResourcesAndPrompts(t *testing.T) {
	client := newTestClient(t)
	defer func() { _ = client.Close() }()

	resources, err := client.ListResources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].URI != "file:///notes/readme.txt" {
		t.Fatalf("resources = %#v", resources)
	}

	readResult, err := client.ReadResource(context.Background(), "file:///notes/readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(readResult.Contents) != 1 || readResult.Contents[0].Text != "hello from resource" {
		t.Fatalf("readResult = %#v", readResult)
	}

	templates, err := client.ListResourceTemplates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 1 || templates[0].URITemplate != "file:///{path}" {
		t.Fatalf("templates = %#v", templates)
	}

	prompts, err := client.ListPrompts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 1 || prompts[0].Name != "review" {
		t.Fatalf("prompts = %#v", prompts)
	}

	prompt, err := client.GetPrompt(context.Background(), "review", map[string]any{"topic": "MCP"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prompt.Messages) != 1 || prompt.Messages[0].Content.Text != "Review MCP" {
		t.Fatalf("prompt = %#v", prompt)
	}
}

func TestStdioClient_CaptureStderrCapsBuffer(t *testing.T) {
	oldPrefix := strings.Repeat("old-prefix-", 600)
	client := &Client{
		stderr: io.NopCloser(strings.NewReader(oldPrefix + "\ntrailer\n")),
	}

	client.captureStderr(client.stderr)

	client.mu.Lock()
	defer client.mu.Unlock()
	if client.errBuf.Len() > stderrContextMaxBytes {
		t.Fatalf("stderr buffer len = %d", client.errBuf.Len())
	}
	got := client.errBuf.String()
	if !strings.Contains(got, "trailer") {
		t.Fatalf("stderr buffer missing tail: %q", got)
	}
	if got == oldPrefix+"\ntrailer" {
		t.Fatalf("stderr buffer was not truncated")
	}
}

func TestScheduleRefresh_CoalescesBurst(t *testing.T) {
	var mu sync.Mutex
	var inFlight bool
	var pending bool
	var runCount atomic.Int32
	started := make(chan struct{})
	unblock := make(chan struct{})

	run := func(context.Context) error {
		n := runCount.Add(1)
		if n == 1 {
			close(started)
			<-unblock
		}
		return nil
	}

	scheduleRefresh(context.Background(), &mu, &inFlight, &pending, run, nil, nil, "test", "stdio")
	<-started
	scheduleRefresh(context.Background(), &mu, &inFlight, &pending, run, nil, nil, "test", "stdio")
	scheduleRefresh(context.Background(), &mu, &inFlight, &pending, run, nil, nil, "test", "stdio")
	close(unblock)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runCount.Load() == 2 {
			time.Sleep(20 * time.Millisecond)
			if got := runCount.Load(); got != 2 {
				t.Fatalf("runCount = %d", got)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for coalesced refreshes; runCount=%d", runCount.Load())
}

func TestStdioClient_ReportAsyncErrorEmitsTransportEvent(t *testing.T) {
	client := &Client{cfg: StdioConfig{Name: "stdio-test"}}
	eventCh := make(chan agentcore.Event, 1)
	client.SetEventSink(func(event agentcore.Event) {
		eventCh <- event
	})

	client.reportAsyncError("notification", "server_message", fmt.Errorf("boom"), true)

	select {
	case event := <-eventCh:
		transportEvent, ok := event.(TransportErrorEvent)
		if !ok {
			t.Fatalf("event type = %T", event)
		}
		if transportEvent.Extension != "stdio-test" || transportEvent.Transport != "stdio" || transportEvent.Operation != "notification" || transportEvent.Reason != "server_message" {
			t.Fatalf("transport event = %#v", transportEvent)
		}
		if transportEvent.Message != "boom" || !transportEvent.Recoverable {
			t.Fatalf("transport event payload = %#v", transportEvent)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for transport event")
	}
}

func TestStdioClient_DiscoveryNotificationsRefreshCaches(t *testing.T) {
	updatedCh := make(chan *ReadResourceResult, 1)
	resourceListChangedCh := make(chan struct{}, 1)
	promptListChangedCh := make(chan struct{}, 1)

	client, err := NewStdioClient(context.Background(), StdioConfig{
		Command:       os.Args[0],
		Args:          []string{"-test.run=TestMCPHelperProcess", "--"},
		Env:           []string{"GO_WANT_MCP_HELPER_PROCESS=1"},
		ClientName:    "mady-test",
		ClientVersion: "0.0.0",
		Discovery: DiscoveryConfig{
			ResourceUpdatedHandler: func(ctx context.Context, uri string, result *ReadResourceResult) {
				updatedCh <- result
			},
			ResourcesListChangedHandler: func(ctx context.Context) {
				resourceListChangedCh <- struct{}{}
			},
			PromptsListChangedHandler: func(ctx context.Context) {
				promptListChangedCh <- struct{}{}
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	readResult, err := client.ReadResource(context.Background(), "file:///notes/readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := readResult.Contents[0].Text; got != "hello from resource" {
		t.Fatalf("initial read text = %q", got)
	}
	if _, err := client.ListResources(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListPrompts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.SubscribeResource(context.Background(), "file:///notes/readme.txt"); err != nil {
		t.Fatal(err)
	}

	select {
	case updated := <-updatedCh:
		if updated == nil || len(updated.Contents) != 1 || updated.Contents[0].Text != "hello refreshed" {
			t.Fatalf("updated = %#v", updated)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for resource update")
	}

	select {
	case <-resourceListChangedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for resource list changed")
	}
	select {
	case <-promptListChangedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for prompt list changed")
	}

	resources, err := client.ListResources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].Name != "readme-v2.txt" {
		t.Fatalf("resources = %#v", resources)
	}
	prompts, err := client.ListPrompts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 1 || prompts[0].Name != "review-v2" {
		t.Fatalf("prompts = %#v", prompts)
	}
	if err := client.UnsubscribeResource(context.Background(), "file:///notes/readme.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestStdioExtension_BuildsAgentTools(t *testing.T) {
	ext := newTestExtension(t)
	defer func() { _ = ext.Dispose() }()

	tools := ext.Tools()
	if len(tools) != 1 {
		t.Fatalf("tools len = %d", len(tools))
	}
	if tools[0].Name != "mcp.echo" {
		t.Fatalf("tool name = %q", tools[0].Name)
	}

	out, err := tools[0].Func(context.Background(), json.RawMessage(`{"text":"world"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out != "echo: world" {
		t.Fatalf("out = %#v", out)
	}
}

func TestStdioExtension_FormatsToolExecutionError(t *testing.T) {
	ext := newTestExtension(t)
	defer func() { _ = ext.Dispose() }()

	out, err := ext.Tools()[0].Func(context.Background(), json.RawMessage(`{"text":"boom"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out != "Error: echo failed" {
		t.Fatalf("out = %#v", out)
	}
}

func TestStdioExtension_WorksWithAgent(t *testing.T) {
	ext := newTestExtension(t)
	defer func() { _ = ext.Dispose() }()

	provider := &toolCallingProvider{}
	agent := agentcore.New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "mcp-agent",
			Model:    "stub",
			Provider: provider,
		},
		Extensions: []agentcore.Extension{ext},
	})

	out, err := agent.Run(context.Background(), "say hello")
	if err != nil {
		t.Fatal(err)
	}
	if out != "done" {
		t.Fatalf("out = %q", out)
	}
	if !provider.seenTool {
		t.Fatal("expected MCP tool to be exposed to provider")
	}

	msgs := agent.State().Messages()
	if len(msgs) < 3 {
		t.Fatalf("messages len = %d", len(msgs))
	}
	toolMsg := msgs[len(msgs)-2]
	if toolMsg.Role != agentcore.RoleTool || toolMsg.Content != "echo: hello from agent" {
		t.Fatalf("tool message = %#v", toolMsg)
	}
}

func TestStdioExtension_HotReloadsToolsOnListChanged(t *testing.T) {
	ext := newTestExtension(t)
	defer func() { _ = ext.Dispose() }()

	agent := agentcore.New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "mcp-agent-hot-reload",
			Model:    "stub",
			Provider: &toolCallingProvider{},
		},
		Extensions: []agentcore.Extension{ext},
	})
	defer agent.Close()

	if _, err := ext.Client().CallTool(context.Background(), "echo", map[string]any{"text": "refresh-tools"}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		names := agent.ToolNames()
		hasEcho := false
		hasReverse := false
		for _, name := range names {
			if name == "mcp.echo" {
				hasEcho = true
			}
			if name == "mcp.reverse" {
				hasReverse = true
			}
		}
		if hasReverse && !hasEcho {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("tool names after hot reload = %#v", agent.ToolNames())
}

func TestStdioExtension_EmitsToolRefreshEvent(t *testing.T) {
	ext := newTestExtension(t)
	defer func() { _ = ext.Dispose() }()

	agent := agentcore.New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "mcp-agent-event",
			Model:    "stub",
			Provider: &toolCallingProvider{},
		},
		Extensions: []agentcore.Extension{ext},
	})
	defer agent.Close()

	eventCh := make(chan ToolsRefreshedEvent, 1)
	agent.On(EventMCPToolsRefreshed, func(ev agentcore.Event) {
		if e, ok := ev.(ToolsRefreshedEvent); ok {
			eventCh <- e
		}
	})

	if _, err := ext.Client().CallTool(context.Background(), "echo", map[string]any{"text": "refresh-tools"}); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-eventCh:
		if ev.Transport != "stdio" || ev.Extension == "" {
			t.Fatalf("event = %#v", ev)
		}
		if len(ev.OldTools) != 1 || ev.OldTools[0] != "mcp.echo" {
			t.Fatalf("old tools = %#v", ev.OldTools)
		}
		if len(ev.NewTools) != 1 || ev.NewTools[0] != "mcp.reverse" {
			t.Fatalf("new tools = %#v", ev.NewTools)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tool refresh event")
	}
}

func TestStdioExtension_EmitsCapabilitiesEvent(t *testing.T) {
	ext := newTestExtension(t)
	defer func() { _ = ext.Dispose() }()

	agent := agentcore.New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "mcp-agent-capabilities-event",
			Model:    "stub",
			Provider: &toolCallingProvider{},
		},
		Extensions: []agentcore.Extension{ext},
	})
	defer agent.Close()

	eventCh := make(chan CapabilitiesUpdatedEvent, 1)
	agent.On(EventMCPCapabilitiesUpdated, func(ev agentcore.Event) {
		if e, ok := ev.(CapabilitiesUpdatedEvent); ok {
			eventCh <- e
		}
	})

	ext.emitCapabilitiesEvent(ext.Client().Capabilities())

	select {
	case ev := <-eventCh:
		if ev.Transport != "stdio" || !ev.Capabilities.Tools.ListChanged || !ev.Capabilities.Resources.Subscribe {
			t.Fatalf("event = %#v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for capabilities event")
	}
}

func TestDiscoveryExtension_BuildsDiscoveryTools(t *testing.T) {
	client := newTestClient(t)
	defer func() { _ = client.Close() }()

	ext, err := NewDiscoveryExtension(client, DiscoveryToolConfig{
		Name:             "mcp-discovery-test",
		ToolPrefix:       "mcp.",
		IncludeResources: true,
		IncludePrompts:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	tools := ext.Tools()
	if len(tools) != 7 {
		t.Fatalf("tools len = %d", len(tools))
	}

	var seen = map[string]*agentcore.Tool{}
	for _, tool := range tools {
		seen[tool.Name] = tool
	}
	if _, ok := seen["mcp.resources.list"]; !ok {
		t.Fatalf("missing resources.list tool: %#v", seen)
	}
	if _, ok := seen["mcp.prompts.get"]; !ok {
		t.Fatalf("missing prompts.get tool: %#v", seen)
	}

	readOut, err := seen["mcp.resources.read"].Func(context.Background(), json.RawMessage(`{"uri":"file:///notes/readme.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	readJSON, _ := json.Marshal(readOut)
	if string(readJSON) == "" || !json.Valid(readJSON) {
		t.Fatalf("read tool output = %q", string(readJSON))
	}

	promptOut, err := seen["mcp.prompts.get"].Func(context.Background(), json.RawMessage(`{"name":"review","arguments":{"topic":"MCP"}}`))
	if err != nil {
		t.Fatal(err)
	}
	promptJSON, _ := json.Marshal(promptOut)
	if string(promptJSON) == "" || !json.Valid(promptJSON) {
		t.Fatalf("prompt tool output = %q", string(promptJSON))
	}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_HELPER_PROCESS") != "1" {
		return
	}
	runHelperServer()
	os.Exit(0)
}

func runHelperServer() {
	setHelperToolSetVersion(1)
	setHelperDiscoveryState("readme.txt", "hello from resource", "review")
	scanner := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()

	for scanner.Scan() {
		line := scanner.Bytes()
		var msg map[string]any
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		method, _ := msg["method"].(string)
		id := msg["id"]
		switch method {
		case "initialize":
			writeMCPResponse(writer, id, map[string]any{
				"protocolVersion": protocolVersion,
				"capabilities": map[string]any{
					"tools": map[string]any{"listChanged": true},
					"resources": map[string]any{
						"subscribe":   true,
						"listChanged": true,
					},
					"prompts": map[string]any{"listChanged": true},
				},
				"serverInfo": map[string]any{
					"name":    "fake-mcp",
					"version": "1.0.0",
				},
			})
		case "notifications/initialized":
			// Notification: no response.
		case "tools/list":
			writeMCPResponse(writer, id, map[string]any{
				"tools": helperTools(),
			})
		case "tools/call":
			params, _ := msg["params"].(map[string]any)
			args, _ := params["arguments"].(map[string]any)
			text, _ := args["text"].(string)
			name, _ := params["name"].(string)
			if name == "reverse" {
				writeMCPResponse(writer, id, map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": reverseString(text)},
					},
				})
				continue
			}
			if text == "refresh-tools" {
				writeMCPResponse(writer, id, map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "tool list updated"},
					},
				})
				setHelperToolSetVersion(2)
				writeMCPNotification(writer, "notifications/tools/list_changed", nil)
				continue
			}
			if text == "boom" {
				writeMCPResponse(writer, id, map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "echo failed"},
					},
					"isError": true,
				})
				continue
			}
			writeMCPResponse(writer, id, map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "echo: " + text},
				},
			})
		case "resources/list":
			writeMCPResponse(writer, id, map[string]any{
				"resources": []map[string]any{
					{
						"uri":         "file:///notes/readme.txt",
						"name":        readHelperResourceName(),
						"description": "Test resource",
						"mimeType":    "text/plain",
					},
				},
			})
		case "resources/read":
			writeMCPResponse(writer, id, map[string]any{
				"contents": []map[string]any{
					{
						"uri":      "file:///notes/readme.txt",
						"mimeType": "text/plain",
						"text":     readHelperResourceText(),
					},
				},
			})
		case "resources/templates/list":
			writeMCPResponse(writer, id, map[string]any{
				"resourceTemplates": []map[string]any{
					{
						"uriTemplate": "file:///{path}",
						"name":        "files",
						"description": "File template",
					},
				},
			})
		case "prompts/list":
			writeMCPResponse(writer, id, map[string]any{
				"prompts": []map[string]any{
					{
						"name":        readHelperPromptName(),
						"description": "Review a topic",
						"arguments": []map[string]any{
							{"name": "topic", "required": true},
						},
					},
				},
			})
		case "resources/subscribe":
			writeMCPResponse(writer, id, map[string]any{})
			setHelperDiscoveryState("readme-v2.txt", "hello refreshed", "review-v2")
			writeMCPNotification(writer, "notifications/resources/updated", map[string]any{
				"uri": "file:///notes/readme.txt",
			})
			writeMCPNotification(writer, "notifications/resources/list_changed", nil)
			writeMCPNotification(writer, "notifications/prompts/list_changed", nil)
		case "resources/unsubscribe":
			writeMCPResponse(writer, id, map[string]any{})
		case "prompts/get":
			params, _ := msg["params"].(map[string]any)
			args, _ := params["arguments"].(map[string]any)
			topic, _ := args["topic"].(string)
			writeMCPResponse(writer, id, map[string]any{
				"description": "Prompt result",
				"messages": []map[string]any{
					{
						"role": "user",
						"content": map[string]any{
							"type": "text",
							"text": "Review " + topic,
						},
					},
				},
			})
		default:
			writeMCPError(writer, id, -32601, fmt.Sprintf("method not found: %s", method))
		}
	}
}

func writeMCPResponse(w *bufio.Writer, id any, result any) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
	_ = w.Flush()
}

func writeMCPError(w *bufio.Writer, id any, code int64, message string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
	_ = w.Flush()
}

func writeMCPNotification(w *bufio.Writer, method string, params any) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	_ = json.NewEncoder(w).Encode(msg)
	_ = w.Flush()
}

var helperResourceName = "readme.txt"
var helperResourceText = "hello from resource"
var helperPromptName = "review"
var helperToolSetVersion = 1

func setHelperDiscoveryState(resourceName, resourceText, promptName string) {
	helperResourceName = resourceName
	helperResourceText = resourceText
	helperPromptName = promptName
}

func setHelperToolSetVersion(v int) { helperToolSetVersion = v }

func readHelperResourceName() string { return helperResourceName }
func readHelperResourceText() string { return helperResourceText }
func readHelperPromptName() string   { return helperPromptName }

func helperTools() []map[string]any {
	if helperToolSetVersion == 2 {
		return []map[string]any{
			{
				"name":        "reverse",
				"description": "Reverse a string",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"text": map[string]any{"type": "string"},
					},
					"required": []string{"text"},
				},
			},
		}
	}
	return []map[string]any{
		{
			"name":        "echo",
			"description": "Echo back a string",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string"},
				},
				"required": []string{"text"},
			},
		},
	}
}

func reverseString(v string) string {
	runes := []rune(v)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	client, err := NewStdioClient(context.Background(), StdioConfig{
		Command:       os.Args[0],
		Args:          []string{"-test.run=TestMCPHelperProcess", "--"},
		Env:           []string{"GO_WANT_MCP_HELPER_PROCESS=1"},
		ToolPrefix:    "mcp.",
		ClientName:    "mady-test",
		ClientVersion: "0.0.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func newTestExtension(t *testing.T) *StdioExtension {
	t.Helper()
	ext, err := NewStdioExtension(context.Background(), StdioConfig{
		Name:          "mcp-test",
		Command:       os.Args[0],
		Args:          []string{"-test.run=TestMCPHelperProcess", "--"},
		Env:           []string{"GO_WANT_MCP_HELPER_PROCESS=1"},
		ToolPrefix:    "mcp.",
		ClientName:    "mady-test",
		ClientVersion: "0.0.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ext
}

var _ agentcore.Extension = (*StdioExtension)(nil)
var _ agentcore.ToolProvider = (*StdioExtension)(nil)

type toolCallingProvider struct {
	seenTool bool
	turn     int
}

func (p *toolCallingProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	p.turn++
	for _, tool := range req.Tools {
		if tool.Name == "mcp.echo" {
			p.seenTool = true
		}
	}
	if p.turn == 1 {
		return &agentcore.ProviderResponse{
			ToolCalls: []agentcore.ToolCall{{
				ID:        "call_1",
				Name:      "mcp.echo",
				Arguments: `{"text":"hello from agent"}`,
			}},
		}, nil
	}
	return &agentcore.ProviderResponse{Content: "done"}, nil
}

func (p *toolCallingProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta)
	close(ch)
	return ch, nil
}

// TestStdioClient_ReconnectAfterCrashKeepsClientUsable 验证重连成功后
// 客户端状态正常（未被永久关闭），后续调用持续可用。
// 注：此测试验证的是成功重连路径；失败路径（initialize 失败）因测试
// 辅助进程总是可启动，难以在集成测试中模拟。
func TestStdioClient_ReconnectAfterCrashKeepsClientUsable(t *testing.T) {
	client := newTestClient(t)
	defer func() { _ = client.Close() }()

	// 先验证客户端正常工作
	result, err := client.CallTool(context.Background(), "echo", map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("initial call failed: %v", err)
	}
	if !strings.Contains(result.Content[0].Text, "echo: hello") {
		t.Fatalf("unexpected result: %#v", result)
	}

	// 杀死 MCP 辅助进程模拟服务器崩溃，触发 readLoop 检测到 EOF
	if client.cmd != nil && client.cmd.Process != nil {
		_ = client.cmd.Process.Kill()
	}

	// 等待 readLoop 检测到进程退出
	time.Sleep(300 * time.Millisecond)

	// 调用工具 → 触发 tryReconnect。测试辅助进程会被重新启动，重连应成功。
	result, err = client.CallTool(context.Background(), "echo", map[string]any{"text": "after-crash"})
	if err != nil {
		t.Fatalf("call after crash failed: %v", err)
	}
	if !strings.Contains(result.Content[0].Text, "echo: after-crash") {
		t.Fatalf("unexpected result after reconnect: %#v", result)
	}

	// 验证客户端没有被永久关闭
	client.mu.Lock()
	closed := client.closed
	client.mu.Unlock()
	if closed {
		t.Fatal("client should not be permanently closed after a successful reconnect")
	}

	// 再调用另一个工具确保客户端持续可用
	result, err = client.CallTool(context.Background(), "echo", map[string]any{"text": "still-works"})
	if err != nil {
		t.Fatalf("subsequent call failed: %v", err)
	}
	if !strings.Contains(result.Content[0].Text, "echo: still-works") {
		t.Fatalf("unexpected result after second call: %#v", result)
	}
}

// TestStdioClient_ReconnectEmitsEvents 验证重连过程中会发出 ReconnectEvent 事件。
func TestStdioClient_ReconnectEmitsEvents(t *testing.T) {
	ext := newTestExtension(t)
	defer func() { _ = ext.Dispose() }()

	agent := agentcore.New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "mcp-reconnect-event-test",
			Model:    "stub",
			Provider: &toolCallingProvider{},
		},
		Extensions: []agentcore.Extension{ext},
	})
	defer agent.Close()

	var started, succeeded bool
	eventCh := make(chan ReconnectEvent, 4)
	agent.On(EventMCPReconnect, func(ev agentcore.Event) {
		if e, ok := ev.(ReconnectEvent); ok {
			eventCh <- e
		}
	})

	client := ext.Client()

	// 杀死 MCP 辅助进程模拟服务器崩溃
	if client.cmd != nil && client.cmd.Process != nil {
		_ = client.cmd.Process.Kill()
	}

	// 等待 readLoop 检测到进程退出
	time.Sleep(300 * time.Millisecond)

	// 调用工具 → 触发 tryReconnect
	_, err := client.CallTool(context.Background(), "echo", map[string]any{"text": "event-test"})
	if err != nil {
		t.Fatalf("call after crash failed: %v", err)
	}

	// 收集所有重连事件
	timeout := time.After(2 * time.Second)
loop:
	for {
		select {
		case ev := <-eventCh:
			if ev.Phase == ReconnectPhaseStarted {
				started = true
			}
			if ev.Phase == ReconnectPhaseSucceeded {
				succeeded = true
			}
			if started && succeeded {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	if !started {
		t.Error("expected ReconnectPhaseStarted event, but none received")
	}
	if !succeeded {
		t.Error("expected ReconnectPhaseSucceeded event, but none received")
	}
}

// TestStdioClient_ReconnectBackoffReset 验证连续重连失败超过
// maxReconnectAttempts 后，退避间隔会被重置为初始值。
func TestStdioClient_ReconnectBackoffReset(t *testing.T) {
	client := newTestClient(t)
	defer func() { _ = client.Close() }()

	// 模拟连续重连失败：设置 reconnectAttempts 超过阈值
	client.reconnectMu.Lock()
	client.reconnectAttempts = maxReconnectAttempts + 1
	client.reconnectBackoff = maxReconnectBackoff // 模拟已达最大退避
	client.reconnectMu.Unlock()

	if client.reconnectBackoff != maxReconnectBackoff {
		t.Fatalf("expected backoff=%v before reconnect, got %v", maxReconnectBackoff, client.reconnectBackoff)
	}

	// 通过关闭管道来触发 isConnectionDeadLocked() 返回 true
	client.mu.Lock()
	client.readErr = io.EOF
	client.mu.Unlock()

	// 调用 tryReconnect 应检测到超过阈值并重置退避和计数
	// 使用带超时的 context 防止测试阻塞
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// tryReconnect 会启动新的辅助进程并完成 initialize
	_ = client.tryReconnect(ctx)

	client.reconnectMu.Lock()
	attempts := client.reconnectAttempts
	backoff := client.reconnectBackoff
	client.reconnectMu.Unlock()

	// 重连成功后 reconnectAttempts 应重置为 0
	if attempts != 0 {
		t.Errorf("expected reconnectAttempts=0 after successful reconnect, got %d", attempts)
	}
	// 重连成功后 backoff 应重置为初始值
	if backoff != initialReconnectDelay {
		t.Errorf("expected backoff=%v after successful reconnect, got %v", initialReconnectDelay, backoff)
	}
}
