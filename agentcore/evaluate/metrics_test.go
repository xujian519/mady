package evaluate

import (
	"testing"
)

func TestExactMatch(t *testing.T) {
	m := ExactMatch{CaseSensitive: false}
	check := func(p, r string, want float64) {
		t.Helper()
		if got := m.Compute(p, r); got != want {
			t.Errorf("ExactMatch(%q,%q)=%.2f want %.2f", p, r, got, want)
		}
	}
	check("hello world", "Hello World", 1)
	check("hello", "world", 0)
	check("  trim  ", "trim", 1)
	cs := ExactMatch{CaseSensitive: true}
	if cs.Compute("Hello", "hello") != 0 {
		t.Error("case sensitive should differ")
	}
}

func TestF1Score(t *testing.T) {
	var m F1Score
	check := func(p, r string, want float64) {
		t.Helper()
		if got := m.Compute(p, r); !approxEqual(got, want) {
			t.Errorf("F1(%q,%q)=%.3f want %.3f", p, r, got, want)
		}
	}
	check("", "", 1)
	check("abc", "", 0)
	check("", "abc", 0)
	check("abc", "abc", 1)
	// "ab" vs "ac": overlap=1 (a), precision=0.5, recall=0.5, F1=0.5
	check("ab", "ac", 0.5)
	// Chinese: 专利 vs 专利权 → overlap=2 (专,利), p=2/3, r=2/3... wait
	// 专利 → {专,利}, 专利权 → {专,利,权}, overlap=2, p=2/2=1, r=2/3, F1=2*1*(2/3)/(1+2/3)=0.8
	check("专利", "专利权", 0.8)
}

func TestKeywordRecall(t *testing.T) {
	m := KeywordRecall{Keywords: []string{"新颖性", "创造性", "实用性"}}
	check := func(p string, want float64) {
		t.Helper()
		if got := m.Compute(p, ""); !approxEqual(got, want) {
			t.Errorf("KeywordRecall(%q)=%.3f want %.3f", p, got, want)
		}
	}
	check("该发明具有新颖性和创造性", 2.0/3.0)
	check("新颖性 创造性 实用性 都具备", 1)
	check("完全无关的内容", 0)

	// Auto-extract from reference
	auto := KeywordRecall{}
	ref := "新颖性 创造性 实用性"
	if got := auto.Compute("新颖性满足", ref); !approxEqual(got, 1.0/3.0) {
		t.Errorf("auto keyword recall = %.3f", got)
	}
}

func TestCitationCompleteness(t *testing.T) {
	m := CitationCompleteness{Required: []string{"CN123", "专利法第22条"}}
	check := func(p string, want float64) {
		t.Helper()
		if got := m.Compute(p, ""); !approxEqual(got, want) {
			t.Errorf("CitationCompleteness(%q)=%.3f want %.3f", p, got, want)
		}
	}
	check("引用了CN123和专利法第22条", 1)
	check("仅引用了CN123", 0.5)
	check("无引用", 0)

	empty := CitationCompleteness{}
	if empty.Compute("anything", "") != 1 {
		t.Error("empty required should return 1")
	}
}

func TestLengthScore(t *testing.T) {
	m := LengthScore{Min: 10, Ideal: 100, Max: 200}
	check := func(p string, wantMin, wantMax float64) {
		t.Helper()
		got := m.Compute(p, "")
		if got < wantMin || got > wantMax {
			t.Errorf("LengthScore(len=%d)=%.3f outside [%.3f,%.3f]", len([]rune(p)), got, wantMin, wantMax)
		}
	}
	check("短", 0, 0.15)                    // very short
	check(repeatRune('A', 100), 0.95, 1.0) // ideal
	check(repeatRune('A', 50), 0.4, 0.6)   // mid ramp
	check(repeatRune('A', 400), 0, 0.1)    // well over max → near 0
}

func TestTokenize(t *testing.T) {
	tokens := tokenize("Hello, 世界!")
	if len(tokens) != 7 { // h,e,l,l,o,世,界
		t.Errorf("tokenize got %d tokens: %v", len(tokens), tokens)
	}
}

func TestExtractKeywords(t *testing.T) {
	kw := ExtractKeywords("专利 新颖性，创造性；实用性")
	if len(kw) != 4 {
		t.Errorf("expected 4 keywords, got %d: %v", len(kw), kw)
	}
}

func TestRuneLen(t *testing.T) {
	if runeLen("abc") != 3 {
		t.Error("ascii len")
	}
	if runeLen("中文") != 2 {
		t.Error("chinese len")
	}
}

func approxEqual(a, b float64) bool {
	return a-b > -0.001 && a-b < 0.001
}

func repeatRune(r rune, n int) string {
	out := make([]rune, n)
	for i := range out {
		out[i] = r
	}
	return string(out)
}
