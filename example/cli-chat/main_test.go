package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/tui/chat"
)

// 说明：本文件是 example/cli-chat 的特征测试（characterization tests）。
// 目的不是提高覆盖率数字，而是在拆分 main.go（970 行）前用断言固定纯函数的
// 当前行为——拆分只搬代码、不改语义，测试即护栏。

func TestParsePathListEnv(t *testing.T) {
	sep := string(filepath.ListSeparator)
	tests := []struct {
		name string
		env  string
		want []string
	}{
		{"未设置", "", nil},
		{"仅空白", "   ", nil},
		{"单路径", "/tmp/skills", []string{"/tmp/skills"}},
		{"多路径含空白", " /a " + sep + " b " + sep + "c/", []string{"/a", "b", "c/"}},
		{"空段被跳过", sep + sep + "/x" + sep, []string{"/x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SKILL_DIRS_TEST", tt.env)
			got := parsePathListEnv("SKILL_DIRS_TEST")
			if len(got) != len(tt.want) {
				t.Fatalf("parsePathListEnv(%q) = %v, want %v", tt.env, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parsePathListEnv(%q)[%d] = %q, want %q", tt.env, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseListEnv(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want []string
	}{
		{"未设置", "", nil},
		{"单值", "a", []string{"a"}},
		{"逗号分隔去空白", " a , b ,, c ", []string{"a", "b", "c"}},
		{"仅逗号", ",,,", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LIST_ENV_TEST", tt.env)
			got := parseListEnv("LIST_ENV_TEST")
			if len(got) != len(tt.want) {
				t.Fatalf("parseListEnv(%q) = %v, want %v", tt.env, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parseListEnv(%q)[%d] = %q, want %q", tt.env, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDefaultModel(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		want     string
	}{
		{"缺省 deepseek", "", "ignored", "deepseek-chat"},
		{"zhipu", "zhipu", "", "glm-5.2"},
		{"kimi", "kimi", "", "kimi-k2-0905-preview"},
		{"其他厂商回退 MODEL", "openai", "gpt-x", "gpt-x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PROVIDER", tt.provider)
			t.Setenv("MODEL", tt.model)
			if got := defaultModel(); got != tt.want {
				t.Fatalf("defaultModel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestThinkingHelpers(t *testing.T) {
	if got := cloneThinkingConfig(nil); got != nil {
		t.Fatalf("cloneThinkingConfig(nil) = %v, want nil", got)
	}
	orig := &agentcore.ThinkingConfig{IncludeThoughts: true, Budget: 100}
	cp := cloneThinkingConfig(orig)
	if cp == orig {
		t.Fatal("cloneThinkingConfig 应返回副本，不能是同一指针")
	}
	cp.Budget = 200
	if orig.Budget != 100 {
		t.Fatalf("修改副本影响了原配置: Budget = %d, want 100", orig.Budget)
	}

	if got := compactThinkingConfig(nil); got != nil {
		t.Fatalf("compactThinkingConfig(nil) = %v, want nil", got)
	}
	if got := compactThinkingConfig(&agentcore.ThinkingConfig{}); got != nil {
		t.Fatalf("全默认配置应压缩为 nil, got %+v", got)
	}
	nonDefault := &agentcore.ThinkingConfig{Effort: agentcore.ThinkingEffortHigh}
	if got := compactThinkingConfig(nonDefault); got != nonDefault {
		t.Fatal("非默认配置应原样返回")
	}

	if got := formatThinkingConfig(nil); got != "default" {
		t.Fatalf("formatThinkingConfig(nil) = %q, want \"default\"", got)
	}
	cfg := &agentcore.ThinkingConfig{
		IncludeThoughts: false,
		Display:         agentcore.ThinkingDisplayOmitted,
		Effort:          agentcore.ThinkingEffortHigh,
		Budget:          1024,
	}
	want := "display=omitted effort=high budget=1024 include_thoughts=false"
	if got := formatThinkingConfig(cfg); got != want {
		t.Fatalf("formatThinkingConfig() = %q, want %q", got, want)
	}
}

func TestParseThinkingCommand(t *testing.T) {
	high := agentcore.ThinkingEffortHigh
	tests := []struct {
		name        string
		input       string
		current     *agentcore.ThinkingConfig
		wantCfg     *agentcore.ThinkingConfig
		wantChanged bool
		wantErr     bool
	}{
		{
			name:    "无参数回显当前",
			input:   "/thinking",
			current: &agentcore.ThinkingConfig{Effort: high},
			wantCfg: &agentcore.ThinkingConfig{Effort: high},
		},
		{
			name:        "reset 清空",
			input:       "/thinking reset",
			current:     &agentcore.ThinkingConfig{Effort: high},
			wantCfg:     nil,
			wantChanged: true,
		},
		{
			name:        "on 打开思考",
			input:       "/thinking on",
			wantCfg:     &agentcore.ThinkingConfig{IncludeThoughts: true, Display: agentcore.ThinkingDisplaySummarized},
			wantChanged: true,
		},
		{
			name:        "off 关闭思考",
			input:       "/thinking off",
			wantCfg:     &agentcore.ThinkingConfig{Display: agentcore.ThinkingDisplayOmitted},
			wantChanged: true,
		},
		{
			name:        "effort 指定强度",
			input:       "/thinking effort high",
			wantCfg:     &agentcore.ThinkingConfig{Effort: high},
			wantChanged: true,
		},
		{
			name:        "effort 恢复默认等价于压缩为 nil",
			input:       "/thinking effort default",
			wantCfg:     nil,
			wantChanged: true,
		},
		{
			name:        "budget 指定预算",
			input:       "/thinking budget 1024",
			wantCfg:     &agentcore.ThinkingConfig{Budget: 1024},
			wantChanged: true,
		},
		{
			name:        "budget default 归零后压缩为 nil",
			input:       "/thinking budget default",
			current:     &agentcore.ThinkingConfig{Budget: 512},
			wantCfg:     nil,
			wantChanged: true,
		},
		{
			name:        "include true 联动 display",
			input:       "/thinking include true",
			wantCfg:     &agentcore.ThinkingConfig{IncludeThoughts: true, Display: agentcore.ThinkingDisplaySummarized},
			wantChanged: true,
		},
		{
			name:        "include false 联动 display",
			input:       "/thinking include false",
			wantCfg:     &agentcore.ThinkingConfig{Display: agentcore.ThinkingDisplayOmitted},
			wantChanged: true,
		},
		{name: "effort 缺参数", input: "/thinking effort", wantErr: true},
		{name: "effort 非法值", input: "/thinking effort bogus", wantErr: true},
		{name: "budget 非数值", input: "/thinking budget abc", wantErr: true},
		{name: "include 非布尔", input: "/thinking include maybe", wantErr: true},
		{name: "未知子命令", input: "/thinking foo", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed, err := parseThinkingCommand(tt.input, tt.current)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseThinkingCommand(%q) 期望报错，实际 err=nil", tt.input)
				}
				if changed {
					t.Fatalf("出错时不应标记为已变更")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseThinkingCommand(%q) 意外报错: %v", tt.input, err)
			}
			if changed != tt.wantChanged {
				t.Fatalf("changed = %v, want %v", changed, tt.wantChanged)
			}
			if (got == nil) != (tt.wantCfg == nil) {
				t.Fatalf("cfg = %+v, want %+v", got, tt.wantCfg)
			}
			if got != nil && *got != *tt.wantCfg {
				t.Fatalf("cfg = %+v, want %+v", *got, *tt.wantCfg)
			}
		})
	}
}

func TestChatMessageFromAgentMessage(t *testing.T) {
	tests := []struct {
		name     string
		msg      agentcore.Message
		wantRole chat.ChatRole
		wantText string
	}{
		{"用户消息", agentcore.Message{Role: agentcore.RoleUser, Content: "你好"}, chat.RoleUser, "你好"},
		{"助手消息", agentcore.Message{Role: agentcore.RoleAssistant, Content: "回复"}, chat.RoleAssistant, "回复"},
		{"工具消息", agentcore.Message{Role: agentcore.RoleTool, Content: "结果"}, chat.RoleTool, "结果"},
		{"未知角色回退 system", agentcore.Message{Role: "unknown", Content: "x"}, chat.RoleSystem, "x"},
		{"无正文时展示工具调用数", agentcore.Message{
			Role:      agentcore.RoleAssistant,
			ToolCalls: []agentcore.ToolCall{{}, {}},
		}, chat.RoleAssistant, "tool calls: 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chatMessageFromAgentMessage(tt.msg)
			if got.Role != tt.wantRole || got.Text != tt.wantText {
				t.Fatalf("chatMessageFromAgentMessage() = (%v, %q), want (%v, %q)",
					got.Role, got.Text, tt.wantRole, tt.wantText)
			}
		})
	}
}

func TestToolDefinitions(t *testing.T) {
	ctx := context.Background()

	weather := weatherTool()
	if weather.Name != "get_weather" {
		t.Fatalf("weatherTool().Name = %q", weather.Name)
	}
	out, err := weather.Func(ctx, json.RawMessage(`{"location":"Tokyo"}`))
	if err != nil {
		t.Fatalf("weatherTool.Func 报错: %v", err)
	}
	weatherMap, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("weatherTool.Func 返回类型 = %T, want map[string]any", out)
	}
	if weatherMap["location"] != "Tokyo" || weatherMap["condition"] != "sunny" {
		t.Fatalf("weatherTool.Func 返回内容异常: %v", weatherMap)
	}
	if _, err := weather.Func(ctx, json.RawMessage(`not-json`)); err == nil {
		t.Fatal("weatherTool.Func 对非法 JSON 应报错")
	}

	calc := calculatorTool()
	if calc.Name != "calculator" {
		t.Fatalf("calculatorTool().Name = %q", calc.Name)
	}
	out, err = calc.Func(ctx, json.RawMessage(`{"expression":"7*8"}`))
	if err != nil {
		t.Fatalf("calculatorTool.Func 报错: %v", err)
	}
	calcMap, ok := out.(map[string]string)
	if !ok {
		t.Fatalf("calculatorTool.Func 返回类型 = %T, want map[string]string", out)
	}
	if calcMap["result"] != "56" {
		t.Fatalf("calculatorTool.Func result = %q, want \"56\"", calcMap["result"])
	}
	if _, err := calc.Func(ctx, json.RawMessage(`{`)); err == nil {
		t.Fatal("calculatorTool.Func 对非法 JSON 应报错")
	}
}
