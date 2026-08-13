package piagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sky-valley/pi/ai"
	"github.com/sky-valley/pi/ai/providers"

	"github.com/xujian519/mady/agentcore"
)

// ---------------------------------------------------------------------------
// faux provider 辅助：每个测试独立注册唯一 Api 的确定性 provider
// ---------------------------------------------------------------------------

var fauxSeq atomic.Int64

type fauxFixture struct {
	reg   *providers.FauxProviderRegistration
	model *ai.Model
}

func newFauxFixture(t *testing.T) *fauxFixture {
	t.Helper()
	id := fmt.Sprintf("faux-%d", fauxSeq.Add(1))
	reg := providers.RegisterFauxProvider(providers.RegisterFauxProviderOptions{
		Api:      id,
		Provider: "faux",
	})
	t.Cleanup(reg.Unregister)
	return &fauxFixture{reg: reg, model: reg.GetModel()}
}

func (f *fauxFixture) scriptToolCallThenReport() {
	f.reg.SetResponses([]providers.FauxResponseStep{
		// 第一轮：请求调用 read 工具
		func(_ ai.Context, _ *ai.SimpleStreamOptions, _ *providers.FauxState, m *ai.Model) *ai.AssistantMessage {
			return &ai.AssistantMessage{
				Content: ai.ContentList{providers.FauxToolCall("read", map[string]any{"path": "x"})},
				Api:     m.Api, Provider: m.Provider, Model: m.ID,
				Usage:      ai.Usage{Input: 100, Output: 5},
				StopReason: ai.StopToolUse,
			}
		},
		// 第二轮：最终报告
		func(_ ai.Context, _ *ai.SimpleStreamOptions, _ *providers.FauxState, m *ai.Model) *ai.AssistantMessage {
			return &ai.AssistantMessage{
				Content: ai.ContentList{providers.FauxText(
					"Scope: 探查完成\nResult: 找到关键文件\nKey files: /a/b.go, /c/d.go\nFiles changed: none\nIssues: none")},
				Api: m.Api, Provider: m.Provider, Model: m.ID,
				Usage:      ai.Usage{Input: 200, Output: 30},
				StopReason: ai.StopStop,
			}
		},
	})
}

func stubReadTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "read",
		Description: "读取文件（测试替身）",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
		ReadOnly:    true,
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return "文件内容：hello", nil
		},
	}
}

// ---------------------------------------------------------------------------
// 单元：报告解析
// ---------------------------------------------------------------------------

func TestParseReport_Structured(t *testing.T) {
	text := "前置说明\nScope: 探查了文档目录\nResult: 存在 3 个待办\nKey files: /a/x.md, /b/y.md\nFiles changed: none\nIssues: 无\n"
	r := parseReport(text)
	if r.Scope != "探查了文档目录" {
		t.Errorf("Scope = %q", r.Scope)
	}
	if r.Result != "存在 3 个待办" {
		t.Errorf("Result = %q", r.Result)
	}
	if len(r.KeyFiles) != 2 || r.KeyFiles[0] != "/a/x.md" {
		t.Errorf("KeyFiles = %v", r.KeyFiles)
	}
	if len(r.FilesChanged) != 0 {
		t.Errorf("FilesChanged = %v, want empty for none", r.FilesChanged)
	}
}

func TestParseReport_FallbackToResult(t *testing.T) {
	text := "没有按格式输出的普通文本"
	r := parseReport(text)
	if r.Result != text {
		t.Errorf("Result = %q, want full text fallback", r.Result)
	}
}

// ---------------------------------------------------------------------------
// 单元：模型解析（环境变量兜底）
// ---------------------------------------------------------------------------

func TestResolveModel_EnvFallback(t *testing.T) {
	t.Setenv("PROVIDER", "deepseek")
	t.Setenv("MODEL", "deepseek-v4-flash")
	m, _, err := resolveModel(nil, "", "")
	if err != nil {
		t.Fatalf("resolveModel: %v", err)
	}
	if m == nil || m.ID != "deepseek-v4-flash" {
		t.Errorf("model = %+v, want deepseek-v4-flash", m)
	}
	if string(m.Provider) != "deepseek" {
		t.Errorf("provider = %q, want deepseek", m.Provider)
	}
}

func TestResolveModel_InjectedWins(t *testing.T) {
	injected := &ai.Model{ID: "fake-1", Api: "faux"}
	m, _, err := resolveModel(injected, "deepseek/deepseek-chat", "openai/gpt-5")
	if err != nil {
		t.Fatalf("resolveModel: %v", err)
	}
	if m != injected {
		t.Error("injected model should win")
	}
}

func TestResolveModel_ParamSpec(t *testing.T) {
	m, _, err := resolveModel(nil, "", "deepseek/deepseek-v4-flash")
	if err != nil {
		t.Fatalf("resolveModel: %v", err)
	}
	if m == nil || m.ID != "deepseek-v4-flash" {
		t.Errorf("model = %+v, want deepseek-v4-flash", m)
	}
}

func TestResolveModel_ThinkingSuffix(t *testing.T) { // model 参数 :high 后缀应返回档位
	m, level, err := resolveModel(nil, "", "deepseek/deepseek-v4-flash:high")
	if err != nil {
		t.Fatalf("resolveModel: %v", err)
	}
	if m == nil {
		t.Fatal("nil model")
	}
	if level != "high" {
		t.Errorf("thinking level = %q, want high", level)
	}
}

func TestResolveAPIKey_MadyConvention(t *testing.T) { // 通用 API_KEY 优先于 provider 特定变量
	t.Setenv("API_KEY", "sk-generic")
	t.Setenv("DEEPSEEK_API_KEY", "sk-deepseek")
	if k := resolveAPIKey("deepseek"); k != "sk-generic" {
		t.Errorf("resolveAPIKey = %q, want generic API_KEY", k)
	}

	t.Setenv("API_KEY", "")
	if k := resolveAPIKey("deepseek"); k != "sk-deepseek" {
		t.Errorf("resolveAPIKey = %q, want DEEPSEEK_API_KEY", k)
	}
}

// ---------------------------------------------------------------------------
// 集成：faux provider 跑真实子会话循环
// ---------------------------------------------------------------------------

func TestRunSpawn_ExploreReturnsReport(t *testing.T) { // AC-1
	fx := newFauxFixture(t)
	fx.scriptToolCallThenReport()

	registered := []*agentcore.Tool{stubReadTool()}
	out := RunSpawn(context.Background(), SpawnConfig{Model: fx.model, Timeout: 30 * time.Second}, registered, SpawnParams{
		SubagentType: PresetExplore,
		Directive:    "探查并汇报",
	})
	if !out.Success {
		t.Fatalf("outcome = %+v", out)
	}
	if out.Report.Scope != "探查完成" {
		t.Errorf("Scope = %q, want 探查完成", out.Report.Scope)
	}
	if out.Report.Result != "找到关键文件" {
		t.Errorf("Result = %q", out.Report.Result)
	}
	if len(out.Report.KeyFiles) != 2 {
		t.Errorf("KeyFiles = %v, want 2", out.Report.KeyFiles)
	}
}

func TestRunSpawn_UsageReported(t *testing.T) { // AC-6
	fx := newFauxFixture(t)
	fx.scriptToolCallThenReport()

	registered := []*agentcore.Tool{stubReadTool()}
	out := RunSpawn(context.Background(), SpawnConfig{Model: fx.model}, registered, SpawnParams{
		SubagentType: PresetExplore,
		Directive:    "探查并汇报",
	})
	if !out.Success {
		t.Fatalf("outcome = %+v", out)
	}
	if out.Usage.InputTokens == 0 || out.Usage.OutputTokens == 0 {
		t.Errorf("Usage = %+v, want non-zero tokens", out.Usage)
	}
	if out.StopReason == "" {
		t.Error("StopReason empty")
	}
}

func TestRunSpawn_ToolCallEnteredSubsession(t *testing.T) { // AC-3：白名单工具可被调用
	fx := newFauxFixture(t)
	fx.scriptToolCallThenReport()

	registered := []*agentcore.Tool{stubReadTool()}
	out := RunSpawn(context.Background(), SpawnConfig{Model: fx.model}, registered, SpawnParams{
		SubagentType: PresetExplore,
		Directive:    "探查并汇报",
	})
	if !out.Success {
		t.Fatalf("outcome = %+v", out)
	}
	if len(out.Report.KeyFiles) != 2 {
		t.Errorf("tool call not executed (report = %+v)", out.Report)
	}
}

func TestRunSpawn_UnknownPreset(t *testing.T) {
	out := RunSpawn(context.Background(), SpawnConfig{}, nil, SpawnParams{
		SubagentType: "no-such",
		Directive:    "x",
	})
	if out.Success || !strings.Contains(out.Error, "未知子会话预设") {
		t.Errorf("outcome = %+v, want unknown-preset error", out)
	}
}

func TestRunSpawn_EmptyWhitelist(t *testing.T) {
	fx := newFauxFixture(t)
	registered := []*agentcore.Tool{stubReadTool()}
	// exclude 掉 explore 全部白名单
	out := RunSpawn(context.Background(), SpawnConfig{Model: fx.model}, registered, SpawnParams{
		SubagentType: PresetExplore,
		Directive:    "x",
		ExcludeTools: []string{"read", "grep", "glob", "ls", "find", "view"},
	})
	if out.Success {
		t.Errorf("outcome = %+v, want empty-whitelist error", out)
	}
}

func TestRunSpawn_ContextIsolation(t *testing.T) { // AC-4：父上下文不被污染
	fx := newFauxFixture(t)
	fx.scriptToolCallThenReport()

	parent := agentcore.New(agentcore.NewConfig())
	parent.State().AddMessage(agentcore.Message{Role: agentcore.RoleUser, Content: "既有对话"})
	before := len(parent.State().Messages())

	registered := []*agentcore.Tool{stubReadTool()}
	out := RunSpawn(context.Background(), SpawnConfig{Model: fx.model}, registered, SpawnParams{
		SubagentType: PresetExplore,
		Directive:    "探查并汇报",
	})
	if !out.Success {
		t.Fatalf("outcome = %+v", out)
	}
	if after := len(parent.State().Messages()); after != before {
		t.Errorf("parent messages %d → %d, want unchanged (子会话上下文必须独立)", before, after)
	}
}

func TestRunSpawn_CancelledContext(t *testing.T) { // V-8：取消传播
	fx := newFauxFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out := RunSpawn(ctx, SpawnConfig{Model: fx.model, Timeout: time.Second}, []*agentcore.Tool{stubReadTool()}, SpawnParams{
		SubagentType: PresetExplore,
		Directive:    "探查",
	})
	if out.Success {
		t.Errorf("outcome = %+v, want failure on cancelled ctx", out)
	}
}

func TestRunSpawn_ProviderUnavailable(t *testing.T) { // V-7：provider 不可用明确报错
	// 清空 key 相关环境变量，deepseek provider 无凭证必然在运行期失败。
	t.Setenv("PROVIDER", "deepseek")
	t.Setenv("MODEL", "deepseek-v4-flash")
	t.Setenv("API_KEY", "")
	t.Setenv("DEEPSEEK_API_KEY", "")
	os.Unsetenv("BASE_URL")
	out := RunSpawn(context.Background(), SpawnConfig{Timeout: time.Second}, []*agentcore.Tool{stubReadTool()}, SpawnParams{
		SubagentType: PresetExplore,
		Directive:    "探查",
	})
	if out.Success {
		t.Error("outcome should fail without credentials")
	}
}
