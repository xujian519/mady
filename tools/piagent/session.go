package piagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sky-valley/pi/agent"
	"github.com/sky-valley/pi/ai"
	"github.com/sky-valley/pi/coding"

	"github.com/xujian519/mady/agentcore"
)

// SpawnConfig 是 spawn_agent 工具的运行时配置（由 tools.BuildTools 注入）。
type SpawnConfig struct {
	// WorkingDir 是子会话的工作目录（父 Agent 的沙箱根）。
	WorkingDir string
	// DefaultModel 是子会话默认模型规格（pi 格式，如 deepseek/deepseek-chat）。
	// 空时按环境变量（PROVIDER/MODEL/BASE_URL/API_KEY）解析。
	DefaultModel string
	// Model 直接注入子会话模型（测试/调用方已解析时使用）。非空时优先于
	// DefaultModel 与环境变量解析。
	Model *ai.Model
	// Timeout 是子会话单次运行的超时（默认 120s，SPAWN_AGENT_TIMEOUT 可覆盖）。
	Timeout time.Duration
	// Policy 是工具执行前的额外门控（可空）。
	Policy ToolPolicy
}

// SpawnParams 对应 spawn_agent 工具参数。
type SpawnParams struct {
	SubagentType string   `json:"subagent_type"`
	Directive    string   `json:"directive"`
	Tools        []string `json:"tools,omitempty"`
	ExcludeTools []string `json:"exclude_tools,omitempty"`
	Model        string   `json:"model,omitempty"`
	Thinking     string   `json:"thinking,omitempty"`
	MaxTokens    *int     `json:"max_tokens,omitempty"`
}

// SpawnReport 是子会话返回的结构化报告（02-spec §2.2）。
type SpawnReport struct {
	Scope        string   `json:"scope"`
	Result       string   `json:"result"`
	KeyFiles     []string `json:"key_files"`
	FilesChanged []string `json:"files_changed"`
	Issues       []string `json:"issues"`
}

// SpawnOutcome 是 spawn_agent 的完整返回。
type SpawnOutcome struct {
	Success    bool         `json:"success"`
	Report     SpawnReport  `json:"report"`
	Usage      UsageSummary `json:"usage"`
	StopReason string       `json:"stop_reason"`
	Error      string       `json:"error"`
}

// UsageSummary 是 token 用量摘要（ai.Usage 的 JSON 友好子集）。
type UsageSummary struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CacheRead    int     `json:"cache_read"`
	CacheWrite   int     `json:"cache_write"`
	CostUSD      float64 `json:"cost_usd"`
}

// DefaultTimeout 是子会话默认超时。
const DefaultTimeout = 120 * time.Second

// RunSpawn 执行一次子会话派发：解析预设 → 白名单 → 构建 pi 会话 → 运行 → 聚合报告。
func RunSpawn(ctx context.Context, cfg SpawnConfig, registered []*agentcore.Tool, params SpawnParams) SpawnOutcome {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if v := os.Getenv("SPAWN_AGENT_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			timeout = d
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	preset := FindPreset(params.SubagentType)
	if preset == nil {
		return outcomeErr(fmt.Sprintf("未知子会话预设 %q，可选：%s", params.SubagentType, strings.Join(PresetNames(), ", ")))
	}

	allowed := ResolveAllowed(registered, preset, params.Tools, params.ExcludeTools)
	if len(allowed) == 0 {
		return outcomeErr("子会话工具白名单为空，无法派发")
	}

	bridged, skipped := ToAgentTools(allowed, BridgeConfig{
		ReadOnly: preset.IsReadOnly,
		Policy:   cfg.Policy,
	})

	model, thinking, err := resolveModel(cfg.Model, cfg.DefaultModel, params.Model)
	if err != nil {
		return outcomeErr("子会话模型解析失败: " + err.Error())
	}
	// model 参数携带 :<level> 后缀时优先于 thinking 参数（pi ResolveModelPattern 语义）。
	if params.Thinking == "" && thinking != "" {
		params.Thinking = thinking
	}

	systemPrompt := buildSubagentPrompt(preset, params.Directive, cfg.WorkingDir)
	sess := coding.NewSession(coding.SessionOptions{
		Model:         model,
		ToolNames:     nil, // 自定义工具集，不启用 pi 内置工具（安全边界）
		NoTools:       coding.NoToolsBuiltin,
		CustomTools:   bridged,
		ThinkingLevel: agent.ThinkingLevel(params.Thinking),
		APIKey:        resolveAPIKey(model.Provider),
		MaxTokens:     params.MaxTokens,
		TimeoutMs:     int(timeout.Milliseconds()),
		Compaction:    &coding.DefaultCompactionSettings,
		SystemPrompt:  systemPrompt,
	})

	result, err := sess.Run(runCtx, params.Directive)
	if err != nil {
		return outcomeErr("子会话运行失败: " + err.Error())
	}

	out := SpawnOutcome{Success: true, Report: parseReport(result.Text)}
	out.Usage = UsageSummary{
		InputTokens:  result.Usage.Input,
		OutputTokens: result.Usage.Output,
		CacheRead:    result.Usage.CacheRead,
		CacheWrite:   result.Usage.CacheWrite,
		CostUSD:      result.Usage.Cost.Total,
	}
	out.StopReason = string(result.StopReason)
	if len(skipped) > 0 {
		out.Report.Issues = append(out.Report.Issues,
			fmt.Sprintf("以下工具因 schema 不兼容被跳过：%s", strings.Join(skipped, ", ")))
	}
	return out
}

// resolveModel 解析子会话模型，返回模型与（可选的）:thinking 后缀档位：
//  1. injected 非空 → 直接使用（测试/调用方注入）
//  2. params.Model 非空 → 优先解析（支持 provider/model 与 :thinking 后缀）
//  3. cfg.DefaultModel 非空 → 解析
//  4. 兜底：环境变量 PROVIDER/MODEL/BASE_URL/API_KEY（Mady 既有约定）
//
// 未知模型 ID 走 pi 的 provider 级 fallback（synthetic custom-id），
// BaseURL 由环境变量显式覆盖（支持本地/兼容端点）。
func resolveModel(injected *ai.Model, defaultModel, paramModel string) (*ai.Model, string, error) {
	if injected != nil {
		return injected, "", nil
	}
	spec := paramModel
	if spec == "" {
		spec = defaultModel
	}
	if spec != "" {
		r, err := coding.ResolveModelPattern(spec)
		if err != nil {
			return nil, "", fmt.Errorf("ResolveModelPattern(%q): %w", spec, err)
		}
		if r.Model == nil {
			return nil, "", fmt.Errorf("ResolveModelPattern(%q): nil model", spec)
		}
		return applyEnvBaseURL(r.Model), r.ThinkingLevel, nil
	}

	// 环境变量兜底：PROVIDER（默认 deepseek）/ MODEL / BASE_URL / API_KEY
	provider := envOr("PROVIDER", "deepseek")
	modelID := envOr("MODEL", "deepseek-v4-flash")
	// 优先在指定 provider 目录内匹配（避免 "deepseek/deepseek-chat" 被
	// pi 的 OpenRouter 式全目录精确匹配劫持）；目录内不存在再走通用解析。
	if m := providerModel(provider, modelID); m != nil {
		return applyEnvBaseURL(m), "", nil
	}
	r, err := coding.ResolveModelPattern(provider + "/" + modelID)
	if err != nil || r.Model == nil {
		return nil, "", fmt.Errorf("环境模型解析 %s/%s: %v", provider, modelID, err)
	}
	return applyEnvBaseURL(r.Model), r.ThinkingLevel, nil
}

// resolveAPIKey 按 Mady 既有约定解析 provider 的 API Key：
// 通用 API_KEY 优先（主 Agent 同约定），其次 provider 特定环境变量
// （DEEPSEEK_API_KEY / OPENAI_API_KEY / GEMINI_API_KEY 等，pi 映射表）。
// 返回空串时 pi 运行时还会做一次自身 env 兜底，不阻塞派发。
func resolveAPIKey(provider string) string {
	if k := os.Getenv("API_KEY"); k != "" {
		return k
	}
	return ai.GetEnvApiKey(provider, nil)
}

// providerModel 在指定 provider 目录内按模型 ID 精确查找（大小写不敏感）。
func providerModel(provider, modelID string) *ai.Model {
	canonical := ""
	for _, p := range ai.GetProviders() {
		if strings.EqualFold(p, provider) {
			canonical = p
			break
		}
	}
	if canonical == "" {
		return nil
	}
	for _, m := range ai.GetModels(canonical) {
		if strings.EqualFold(m.ID, modelID) {
			return m
		}
	}
	return nil
}

// applyEnvBaseURL 用 BASE_URL 环境变量覆盖模型端点（为空时保持目录默认值）。
func applyEnvBaseURL(m *ai.Model) *ai.Model {
	if base := os.Getenv("BASE_URL"); base != "" {
		clone := *m
		clone.BaseURL = base
		return &clone
	}
	return m
}

// buildSubagentPrompt 组装子会话系统提示词（预设后缀 + 强制输出格式）。
func buildSubagentPrompt(preset *Preset, directive, cwd string) string {
	var b strings.Builder
	b.WriteString("你是 Mady 的辅助智能体（子会话），由父 Agent 派发完成一个定向任务。\n")
	b.WriteString("任务指令：\n")
	b.WriteString(directive)
	b.WriteString("\n\n")
	if cwd != "" {
		b.WriteString("工作目录：" + cwd + "\n\n")
	}
	b.WriteString("规则：\n")
	b.WriteString("1. 只执行指令范围内的任务；不要自行扩大范围。\n")
	b.WriteString("2. 除非指令明确要求，不要创建文件；不得主动创建文档/README。\n")
	b.WriteString("3. 只调用你被允许的工具，不得尝试受限工具。\n")
	b.WriteString("4. 文件写入是任务的一部分：若指令要求写文件，先写完再给最终报告。\n")
	b.WriteString("5. 最终助手消息必须严格按下方输出格式，缺少字段视为失败。\n")
	b.WriteString("6. 引用文件时使用绝对路径。\n")
	b.WriteString("7. 若指令在你的工具范围内无法完成，在报告中明确说明。\n\n")
	b.WriteString(preset.SystemPromptSuffix)
	b.WriteString("\n\n强制输出格式（必须逐字段给出）：\n")
	b.WriteString("Scope: <一句话说明本次做了什么>\n")
	b.WriteString("Result: <发现/结论，可用 Markdown>\n")
	b.WriteString("Key files: <逗号分隔的绝对路径，或 none>\n")
	b.WriteString("Files changed: <改动清单及理由，或 none>\n")
	b.WriteString("Issues: <注意事项/阻塞项，或 none>\n")
	return b.String()
}

// parseReport 从子会话最终文本提取结构化报告（按强制格式逐字段解析）。
// 解析失败时整体落入 Result，不阻塞返回。
func parseReport(text string) SpawnReport {
	report := SpawnReport{}
	var lists = map[string]*[]string{
		"Key files":     &report.KeyFiles,
		"Files changed": &report.FilesChanged,
		"Issues":        &report.Issues,
	}
	singles := map[string]*string{
		"Scope":  &report.Scope,
		"Result": &report.Result,
	}
	all := []string{"Scope", "Result", "Key files", "Files changed", "Issues"}

	currentKey := ""
	var currentBuf []string
	flush := func() {
		if currentKey == "" {
			return
		}
		joined := strings.TrimSpace(strings.Join(currentBuf, "\n"))
		if field, ok := singles[currentKey]; ok {
			*field = joined
		}
		if list, ok := lists[currentKey]; ok {
			*list = splitList(joined)
		}
		currentKey = ""
		currentBuf = nil
	}
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		matched := false
		for _, name := range all {
			prefix := name + ":"
			if strings.HasPrefix(trimmed, prefix) {
				flush()
				currentKey = name
				rest := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
				if rest != "" {
					currentBuf = append(currentBuf, rest)
				}
				matched = true
				break
			}
		}
		if !matched && currentKey != "" {
			currentBuf = append(currentBuf, line)
		}
	}
	flush()

	if report.Scope == "" && report.Result == "" && len(report.KeyFiles) == 0 {
		// 未按格式输出：整体作为 Result。
		report.Result = strings.TrimSpace(text)
	}
	return report
}

func splitList(s string) []string {
	if s == "" || s == "none" || s == "None" || s == "无" || s == "N/A" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '、'
	})
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func outcomeErr(msg string) SpawnOutcome {
	return SpawnOutcome{Success: false, Error: msg}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Marshal 返回 spawn_agent 工具结果（SpawnOutcome 的 JSON 序列化）。
func (o SpawnOutcome) Marshal() (string, error) {
	b, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return "", fmt.Errorf("spawn outcome marshal: %w", err)
	}
	return string(b), nil
}
