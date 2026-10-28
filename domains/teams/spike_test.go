package teams

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/session"
)

// recordingProvider 记录每次请求的消息并回显最后一条用户消息。
type recordingProvider struct {
	mu       sync.Mutex
	requests [][]agentcore.Message
}

func (p *recordingProvider) Complete(_ context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	p.mu.Lock()
	p.requests = append(p.requests, append([]agentcore.Message(nil), req.Messages...))
	p.mu.Unlock()
	return &agentcore.ProviderResponse{Content: "答:" + lastUserContent(req.Messages)}, nil
}

func (p *recordingProvider) Stream(context.Context, *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta, 1)
	ch <- agentcore.StreamDelta{Done: true}
	close(ch)
	return ch, nil
}

func (p *recordingProvider) lastMessages() []agentcore.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.requests) == 0 {
		return nil
	}
	return p.requests[len(p.requests)-1]
}

func lastUserContent(messages []agentcore.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == agentcore.RoleUser {
			return messages[i].Content
		}
	}
	return ""
}

// TestSpikeAgentsRunConcurrentlyWithIsolatedBuses 验证 ADR 0008 前提 1：
// 多个 Agent 实例可并发运行，事件总线互不干扰。
func TestSpikeAgentsRunConcurrentlyWithIsolatedBuses(t *testing.T) {
	providers := []*recordingProvider{{}, {}}
	agents := []*agentcore.Agent{
		agentcore.New(agentcore.StubConfig(providers[0], agentcore.WithName("member-a"))),
		agentcore.New(agentcore.StubConfig(providers[1], agentcore.WithName("member-b"))),
	}
	defer agents[0].Close()
	defer agents[1].Close()

	var seen [2][]string
	for i, agent := range agents {
		index := i
		agent.On(agentcore.EventAgentStart, func(e agentcore.Event) {
			if start, ok := e.(*agentcore.AgentStartEvent); ok {
				seen[index] = append(seen[index], start.Input)
			}
		})
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	outputs := make([]string, 2)
	errs := make([]error, 2)
	for i, agent := range agents {
		index := i
		wg.Go(func() {
			<-start
			outputs[index], errs[index] = agent.Run(context.Background(), "任务"+string(rune('A'+index)))
		})
	}
	close(start)
	wg.Wait()

	for i := range agents {
		if errs[i] != nil {
			t.Fatalf("agent %d 运行失败：%v", i, errs[i])
		}
	}
	if !strings.HasSuffix(outputs[0], "任务A") || !strings.HasSuffix(outputs[1], "任务B") {
		t.Fatalf("输出串扰：%q / %q", outputs[0], outputs[1])
	}
	if len(seen[0]) != 1 || seen[0][0] != "任务A" {
		t.Fatalf("member-a 事件混入其他实例：%v", seen[0])
	}
	if len(seen[1]) != 1 || seen[1][0] != "任务B" {
		t.Fatalf("member-b 事件混入其他实例：%v", seen[1])
	}
}

// TestSpikeMemberWakeFromAgentStore 验证 ADR 0008 前提 2：
// 用 AgentStore 恢复的 Agent 能带着完整上下文跨轮继续。
func TestSpikeMemberWakeFromAgentStore(t *testing.T) {
	dir := t.TempDir()
	fileStore, err := session.NewFileStore(dir)
	if err != nil {
		t.Fatalf("创建会话存储失败：%v", err)
	}
	store := session.NewAgentStore(fileStore, dir)
	provider := &recordingProvider{}

	first := agentcore.New(agentcore.StubConfig(provider,
		agentcore.WithName("member-a"),
		func(cfg *agentcore.Config) { cfg.Store = store },
	))
	if _, err := first.Run(context.Background(), "第一轮问题"); err != nil {
		t.Fatalf("第一轮运行失败：%v", err)
	}
	if err := first.SaveState(context.Background(), "member-a"); err != nil {
		t.Fatalf("保存成员状态失败：%v", err)
	}
	first.Close()

	woken := agentcore.New(agentcore.StubConfig(provider,
		agentcore.WithName("member-a"),
		func(cfg *agentcore.Config) { cfg.Store = store },
	))
	defer woken.Close()
	if err := woken.LoadState(context.Background(), "member-a"); err != nil {
		t.Fatalf("恢复成员状态失败：%v", err)
	}
	if _, err := woken.Run(context.Background(), "第二轮问题"); err != nil {
		t.Fatalf("唤醒后运行失败：%v", err)
	}

	var history []string
	for _, message := range provider.lastMessages() {
		history = append(history, message.Content)
	}
	joined := strings.Join(history, "|")
	if !strings.Contains(joined, "第一轮问题") || !strings.Contains(joined, "第二轮问题") {
		t.Fatalf("唤醒后上下文不完整：%s", joined)
	}
}

// TestSpikeAgentStateDoesNotCarryToolRegistry 记录前提 2 的边界：
// 快照只含消息与运行状态，工具注册表由调用方用同一配置重建。
func TestSpikeAgentStateDoesNotCarryToolRegistry(t *testing.T) {
	dir := t.TempDir()
	fileStore, err := session.NewFileStore(dir)
	if err != nil {
		t.Fatalf("创建会话存储失败：%v", err)
	}
	store := session.NewAgentStore(fileStore, dir)
	var calls atomic.Int32
	tool := &agentcore.Tool{
		Name:       "noop",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(context.Context, json.RawMessage) (any, error) {
			calls.Add(1)
			return "ok", nil
		},
	}
	cfg := agentcore.StubConfig(&recordingProvider{},
		agentcore.WithName("member-tools"),
		agentcore.WithTools(tool),
		func(cfg *agentcore.Config) { cfg.Store = store },
	)
	first := agentcore.New(cfg)
	if err := first.SaveState(context.Background(), "member-tools"); err != nil {
		t.Fatalf("保存状态失败：%v", err)
	}
	first.Close()

	woken := agentcore.New(agentcore.StubConfig(&recordingProvider{},
		agentcore.WithName("member-tools"),
		func(cfg *agentcore.Config) { cfg.Store = store },
	))
	defer woken.Close()
	if err := woken.LoadState(context.Background(), "member-tools"); err != nil {
		t.Fatalf("恢复状态失败：%v", err)
	}
	if _, ok := woken.GetTool("noop"); ok {
		t.Fatal("快照不应携带工具注册表；调用方须以同一配置重建")
	}
}
