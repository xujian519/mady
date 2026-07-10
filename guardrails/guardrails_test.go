package guardrails

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestNew_DefaultLevelIsLight(t *testing.T) {
	hook := New()
	gr, ok := hook.(*guardrail)
	if !ok {
		t.Fatalf("expected *guardrail, got %T", hook)
	}
	if gr.config.Level != LevelLight {
		t.Errorf("default level = %v, want LevelLight", gr.config.Level)
	}
}

func TestNew_CustomLevel(t *testing.T) {
	hook := New(WithLevel(LevelStrict))
	gr := hook.(*guardrail)
	if gr.config.Level != LevelStrict {
		t.Errorf("level = %v, want LevelStrict", gr.config.Level)
	}
}

func TestNew_CustomDisclaimer(t *testing.T) {
	hook := New(
		WithLevel(LevelStandard),
		WithDisclaimer("custom disclaimer text"),
	)
	gr := hook.(*guardrail)
	if gr.config.Disclaimer != "custom disclaimer text" {
		t.Errorf("disclaimer = %q", gr.config.Disclaimer)
	}
}

func TestGuardrail_BlockedPhrases(t *testing.T) {
	tests := []struct {
		name    string
		content string
		config  Config
		wantErr bool
	}{
		{
			name:    "blocks malicious code",
			content: "这是恶意代码的示例",
			config:  Config{Level: LevelLight, BlockedPhrases: []string{"恶意代码"}},
			wantErr: true,
		},
		{
			name:    "passes normal content",
			content: "这是一份正常的专利分析报告",
			config:  Config{Level: LevelLight, BlockedPhrases: []string{"恶意代码"}},
			wantErr: false,
		},
		{
			name:    "blocks attack method",
			content: "攻击方法如下所述",
			config:  Config{Level: LevelLight, BlockedPhrases: []string{"攻击方法"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gr := &guardrail{config: tt.config}
			mcc := &agentcore.ModelCallContext{
				Response: &agentcore.ProviderResponse{
					Content: tt.content,
				},
			}
			gr.AfterModelCall(context.TODO(), nil, mcc)
			if tt.wantErr && mcc.Err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && mcc.Err != nil {
				t.Errorf("unexpected error: %v", mcc.Err)
			}
		})
	}
}

func TestGuardrail_DisclaimerInjection(t *testing.T) {
	tests := []struct {
		name             string
		level            Level
		disclaimer       string
		riskKeywords     []string
		content          string
		shouldInject     bool
		shouldNotContain string
	}{
		{
			name:         "LevelLight does not inject disclaimer",
			level:        LevelLight,
			disclaimer:   "免责声明",
			riskKeywords: []string{"风险"},
			content:      "有风险的内容",
			shouldInject: false,
		},
		{
			name:         "LevelStandard injects on risk keyword",
			level:        LevelStandard,
			disclaimer:   "本回复仅供参考。",
			riskKeywords: []string{"侵权"},
			content:      "本文涉及侵权分析",
			shouldInject: true,
		},
		{
			name:         "LevelStandard does not inject without keyword",
			level:        LevelStandard,
			disclaimer:   "免责声明",
			riskKeywords: []string{"侵权"},
			content:      "普通内容没有风险",
			shouldInject: false,
		},
		{
			name:         "disclaimer not duplicated when content already has it",
			level:        LevelStandard,
			disclaimer:   "免责声明文本",
			riskKeywords: []string{"侵权"},
			content:      "侵权分析内容\n---\n不构成正式法律意见",
			shouldInject: false, // "不构成正式" prevents re-injection
		},
		{
			name:         "LevelStrict injects on keyword",
			level:        LevelStrict,
			disclaimer:   "强烈免责声明。",
			riskKeywords: []string{"无效"},
			content:      "该专利可能无效",
			shouldInject: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gr := &guardrail{config: Config{
				Level:        tt.level,
				Disclaimer:   tt.disclaimer,
				RiskKeywords: tt.riskKeywords,
			}}
			mcc := &agentcore.ModelCallContext{
				Response: &agentcore.ProviderResponse{
					Content: tt.content,
				},
			}
			gr.AfterModelCall(context.TODO(), nil, mcc)

			hasDisclaimer := strings.Contains(mcc.Response.Content, tt.disclaimer)
			if tt.shouldInject && !hasDisclaimer {
				t.Errorf("disclaimer not injected. content: %s", mcc.Response.Content)
			}
			if !tt.shouldInject && hasDisclaimer {
				t.Errorf("disclaimer unexpectedly injected. content: %s", mcc.Response.Content)
			}
			if tt.shouldNotContain != "" && strings.Contains(mcc.Response.Content, tt.shouldNotContain) {
				t.Errorf("content should not contain %q: %s", tt.shouldNotContain, mcc.Response.Content)
			}
		})
	}
}

func TestGuardrail_ApprovalKeywords(t *testing.T) {
	t.Run("LevelStrict sets SuppressPersist on approval keyword", func(t *testing.T) {
		gr := &guardrail{config: Config{
			Level:            LevelStrict,
			ApprovalKeywords: []string{"专利结论"},
		}}
		mcc := &agentcore.ModelCallContext{
			Response: &agentcore.ProviderResponse{
				Content: "专利结论：该发明具有新颖性。",
			},
		}
		gr.AfterModelCall(context.TODO(), nil, mcc)

		if !mcc.Response.SuppressPersist {
			t.Error("expected SuppressPersist to be set at LevelStrict with approval keyword")
		}
	})

	t.Run("LevelStandard does not set SuppressPersist", func(t *testing.T) {
		gr := &guardrail{config: Config{
			Level:            LevelStandard,
			ApprovalKeywords: []string{"专利结论"},
		}}
		mcc := &agentcore.ModelCallContext{
			Response: &agentcore.ProviderResponse{
				Content: "专利结论：该发明具有新颖性。",
			},
		}
		gr.AfterModelCall(context.TODO(), nil, mcc)

		if mcc.Response.SuppressPersist {
			t.Error("LevelStandard should not set SuppressPersist")
		}
	})
}

func TestGuardrail_NilResponseIsSafe(t *testing.T) {
	gr := &guardrail{config: Config{
		Level:        LevelStrict,
		RiskKeywords: []string{"风险"},
	}}
	// Should not panic with nil response.
	gr.AfterModelCall(context.TODO(), nil, &agentcore.ModelCallContext{
		Response: nil,
		Err:      nil,
	})
}

func TestGuardrail_ErrorResponseIsSkipped(t *testing.T) {
	gr := &guardrail{config: Config{
		Level:        LevelStrict,
		RiskKeywords: []string{"风险"},
	}}
	mcc := &agentcore.ModelCallContext{
		Response: &agentcore.ProviderResponse{
			Content: "有风险的内容",
		},
		Err: agentcore.NewNodeError("provider error", nil, "test", "test"),
	}
	original := mcc.Response.Content
	gr.AfterModelCall(context.TODO(), nil, mcc)
	// Should skip on error.
	if mcc.Response.Content != original {
		t.Errorf("content was modified on error")
	}
}
