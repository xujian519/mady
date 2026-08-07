package intent

import (
	"strings"
	"testing"
)

func TestKeywordClassifier_DomainRouting(t *testing.T) {
	kc := NewKeywordClassifier()

	tests := []struct {
		input  string
		domain Domain
	}{
		{"帮我分析这个专利的新颖性", DomainPatent},
		{"检索专利 CN112345678A", DomainPatent},
		{"invention claim analysis", DomainPatent},
		// "专利法" contains "专利", so it's patent domain (patent has higher priority than legal)
		{"查一下专利法第26条第3款", DomainPatent},
		{"这个合同条款是否有效", DomainLegal},
		{"帮我写一个python脚本", DomainAssistant},
		{"搜索最新的AI论文", DomainAssistant},
		{"今天天气真好", DomainChat},
		{"hello, how are you?", DomainChat},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := kc.Classify(tt.input)
			if result.Domain != tt.domain {
				t.Errorf("Classify(%q).Domain = %q, want %q", tt.input, result.Domain, tt.domain)
			}
		})
	}
}

func TestKeywordClassifier_SubIntent(t *testing.T) {
	kc := NewKeywordClassifier()

	tests := []struct {
		input     string
		subIntent SubIntent
		runMode   RunMode
	}{
		{"分析这个专利的无效宣告理由", SubIntentInvalidation, ModeFlexiblePlan},
		{"判断这个专利是否构成侵权", SubIntentInfringement, ModeFlexiblePlan},
		{"评估这个发明专利的新颖性", SubIntentNovelty, ModeJudgment},
		{"用三步法分析创造性", SubIntentInventiveness, ModeJudgment},
		{"帮我撰写一份专利申请", SubIntentDrafting, ModeFlexiblePlan},
		// "答复" requires patent context — need patent keywords present
		{"答复这份专利的审查意见通知书", SubIntentOAResponse, ModeFlexiblePlan},
		// "驳回" + "复审" should trigger reexamination
		{"这个专利的驳回决定是否可以复审", SubIntentReexamination, ModeFlexiblePlan},
		{"做一份FTO自由实施分析，评估专利的侵权风险", SubIntentFTO, ModeFlexiblePlan},
		{"评估这个专利的26.3充分公开要求", SubIntentEnablement, ModeJudgment},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := kc.Classify(tt.input)
			if result.SubIntent != tt.subIntent {
				t.Errorf("Classify(%q).SubIntent = %q, want %q", tt.input, result.SubIntent, tt.subIntent)
			}
			if result.RunMode != tt.runMode {
				t.Errorf("Classify(%q).RunMode = %q, want %q", tt.input, result.RunMode, tt.runMode)
			}
		})
	}
}

func TestKeywordClassifier_Complexity(t *testing.T) {
	kc := NewKeywordClassifier()

	tests := []struct {
		input      string
		complexity Complexity
	}{
		{"hi", ComplexityLow},
		{"帮我搜索一下相关专利", ComplexityHigh},                            // 含"专利"
		{"分析这个技术方案的创造性", ComplexityHigh},                          // 含"分析"+"创造性"
		{strings.Repeat("这是一个很长的输入文本内容用于测试", 50), ComplexityHigh}, // ~750 chars
	}

	for _, tt := range tests {
		name := tt.input
		if len([]rune(name)) > 20 {
			name = string([]rune(name)[:20])
		}
		t.Run(name, func(t *testing.T) {
			result := kc.Classify(tt.input)
			if result.Complexity != tt.complexity {
				short := tt.input
				if len([]rune(short)) > 30 {
					short = string([]rune(short)[:30])
				}
				t.Errorf("Classify(%q).Complexity = %v, want %v",
					short, result.Complexity, tt.complexity)
			}
		})
	}
}

func TestKeywordClassifier_EmptyInput(t *testing.T) {
	kc := NewKeywordClassifier()
	result := kc.Classify("")
	if result.Domain != DomainChat {
		t.Errorf("empty input should be chat, got %q", result.Domain)
	}
	if result.Confidence == 0 {
		t.Error("empty input should have non-zero confidence")
	}
}

func TestUnifiedRouter_KeywordPriority(t *testing.T) {
	kc := NewKeywordClassifier()
	router := NewUnifiedRouter(kc)

	// Keyword should work even without preferences
	result := router.Classify("分析专利的新颖性")
	if result.Domain != DomainPatent {
		t.Errorf("expected patent, got %q", result.Domain)
	}
	if result.SubIntent != SubIntentNovelty {
		t.Errorf("expected novelty, got %q", result.SubIntent)
	}
}

func TestUnifiedRouter_FallbackToChat(t *testing.T) {
	router := NewUnifiedRouter(NewKeywordClassifier())

	result := router.Classify("hello world")
	if result.Domain != DomainChat {
		t.Errorf("expected chat fallback, got %q", result.Domain)
	}
}
