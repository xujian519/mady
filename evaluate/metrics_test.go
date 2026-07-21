package evaluate

import (
	"sync"
	"testing"

	"github.com/xujian519/mady/guardrails"
)

// adapter 将 guardrails.CitationReport 映射到本包 CitationValidityReport，
// 用于在测试中复用线上引用核验 Gate。
func adapter(r guardrails.CitationReport) CitationValidityReport {
	return CitationValidityReport{
		Total:        r.Total,
		Valid:        r.Valid,
		Unknown:      r.Unknown,
		Unverifiable: r.Unverifiable,
		Suspect:      r.Suspect,
		Invalid:      r.Invalid,
	}
}

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

func TestCitationValidity(t *testing.T) {
	// 注入线上引用核验 Gate 作为核验源，与生产装配（cmd/mady eval）一致。
	// 通过公开 API SetCitationVerifier 注入，t.Cleanup 恢复默认值。
	// SetCitationVerifier 内部用 atomic.Pointer 保证并发安全。
	//
	// 不可加 t.Parallel() — 依赖全局 currentCitationVerifier 的串行隔离。
	prev := getCitationVerifier()
	t.Cleanup(func() { SetCitationVerifier(prev) })
	SetCitationVerifier(func(text string) CitationValidityReport {
		return adapter(guardrails.VerifyCitations(text))
	})

	m := CitationValidity{}
	check := func(p string, want float64) {
		t.Helper()
		if got := m.Compute(p, ""); !approxEqual(got, want) {
			t.Errorf("CitationValidity(%q)=%.3f want %.3f", p, got, want)
		}
	}
	// 合法引用：第 22 条用途声明命中注册主题（创造性）。
	check("根据专利法第22条第3款的规定，权利要求1具备创造性。", 1.0)
	// 无法条引用 / 静态表未覆盖（Unknown）/ 无用途声明（Unverifiable）：不计入分母，得 1。
	check("该权利要求具备创造性。", 1.0)
	check("依据专利法第64条，保护范围以权利要求的内容为准。", 1.0)
	check("修改符合专利法第33条。", 1.0)
	// 张冠李戴（Suspect）与幻觉编号（Invalid）：计入分母且非 Valid，得 0。
	check("分析如下：专利法第47条（分案申请）允许申请人提出分案。", 0.0)
	check("依据专利法第二百零八条，该申请应予驳回。", 0.0)
	// 对错参半：Valid 1 / 可核验 2 = 0.5。
	check("根据专利法第22条第3款，具备创造性。专利法第47条（分案申请）允许分案。", 0.5)
}

// TestSetCitationVerifierConcurrent 验证 SetCitationVerifier 与 Compute 并发无 data race。
//
// 不可加 t.Parallel() — 依赖全局 currentCitationVerifier 的串行隔离。
// 必须 `go test -race` 跑此用例才能真正验证原子性。
//
// 设计要点：断言"两个 verifier 都被观察到"不能靠运气（goroutine 调度可能导致
// writer 在 readers 启动前就跑完）。这里采用确定性协调：
//   - writer 每轮切换 verifier 后通过 barrier 等所有 readers 完成一次 Compute，
//     再进入下一轮。这样每轮 readers 看到的 verifier 是确定的。
//   - 我们仍用 atomic.Pointer 的 Load/Store（而非锁）让 race detector 能检测到
//     任何非原子访问尝试。
func TestSetCitationVerifierConcurrent(t *testing.T) {
	prev := getCitationVerifier()
	t.Cleanup(func() { SetCitationVerifier(prev) })

	// 两个 verifier 产生可区分的分数：
	//   verifierA: Total=2, Valid=1 → verifiable=2, score=0.5
	//   verifierB: Total=2, Valid=0 → verifiable=2, score=0.0
	verifierA := func(_ string) CitationValidityReport { return CitationValidityReport{Total: 2, Valid: 1} }
	verifierB := func(_ string) CitationValidityReport { return CitationValidityReport{Total: 2, Valid: 0} }
	SetCitationVerifier(verifierA)

	const readers = 8
	const rounds = 50 // 每轮切换一次 verifier；rounds 足够让 race detector 观察到竞争

	var observed sync.Map

	// barrier：每轮所有 readers 各 Compute 一次后释放，writer 才进入下一轮。
	// writer 单独跑一个 goroutine，与 readers 通过两个 channel 握手。
	readerDone := make(chan struct{}, readers)
	writerRelease := make(chan struct{})

	// writer goroutine：交替切换 A/B，每轮放行 readers 后等他们完成
	go func() {
		for r := 0; r < rounds; r++ {
			if r%2 == 0 {
				SetCitationVerifier(verifierB)
			} else {
				SetCitationVerifier(verifierA)
			}
			// 放行所有 readers
			for i := 0; i < readers; i++ {
				writerRelease <- struct{}{}
			}
			// 等所有 readers 完成本轮 Compute
			for i := 0; i < readers; i++ {
				<-readerDone
			}
		}
		close(writerRelease)
	}()

	// reader goroutines：每轮先等 writer 放行，再 Compute 一次，通知完成
	var wg sync.WaitGroup
	wg.Add(readers)
	for range readers {
		go func() {
			defer wg.Done()
			m := CitationValidity{}
			for {
				_, ok := <-writerRelease
				if !ok {
					return
				}
				score := m.Compute("测试文本", "")
				if score < 0 || score > 1 {
					t.Errorf("Compute 返回越界值: %f", score)
					return
				}
				observed.Store(score, true)
				readerDone <- struct{}{}
			}
		}()
	}
	wg.Wait()

	// 确定性断言：barrier 协调保证每一轮 verifier 都被所有 readers 观察到，
	// 因此 rounds ≥ 2 时 A 和 B 的分数都必须出现在 observed 中。
	gotA, gotB := false, false
	observed.Range(func(k, _ any) bool {
		s := k.(float64)
		if s == 0.5 {
			gotA = true
		}
		if s == 0.0 {
			gotB = true
		}
		return true
	})
	if !gotA || !gotB {
		t.Errorf("期望观察到两个 verifier 的分数 (0.0 和 0.5)，gotA=%v gotB=%v", gotA, gotB)
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

func TestEvidenceGroundedness(t *testing.T) {
	m := EvidenceGroundedness{ValidEvidence: []string{"CN001", "CN002", "guide-p2-c3"}}
	check := func(p string, want float64) {
		t.Helper()
		if got := m.Compute(p, ""); !approxEqual(got, want) {
			t.Errorf("EvidenceGroundedness(%q)=%.3f want %.3f", p, got, want)
		}
	}
	// Both citations valid → 1.0.
	check("引用了 doc_id: CN001 和 [CN002]", 1.0)
	// One valid, one hallucinated → 0.5.
	check("doc_id: CN001 和 doc_id: FAKE999", 0.5)
	// No citations → 0 (ungrounded).
	check("无任何证据引用", 0)
	// All hallucinated → 0.
	check("[FAKE001] [FAKE002]", 0)
}

func TestEvidenceGroundedness_EmptyValidSet(t *testing.T) {
	m := EvidenceGroundedness{ValidEvidence: nil}
	// No valid evidence set → cannot ground → 0 even if prediction cites IDs.
	if got := m.Compute("doc_id: CN001", ""); got != 0 {
		t.Errorf("empty ValidEvidence should return 0, got %.3f", got)
	}
}

func TestEvidenceGroundedness_WithCitations(t *testing.T) {
	base := EvidenceGroundedness{}
	adapted := base.WithCitations([]string{"A1", "A2"}).(EvidenceGroundedness)
	if len(adapted.ValidEvidence) != 2 {
		t.Errorf("WithCitations should set ValidEvidence, got %d", len(adapted.ValidEvidence))
	}
}

func TestRuleComplianceCompleteness(t *testing.T) {
	m := RuleComplianceCompleteness{Required: []string{"NOV-001", "NOV-002", "A22.3"}}
	check := func(p string, want float64) {
		t.Helper()
		if got := m.Compute(p, ""); !approxEqual(got, want) {
			t.Errorf("RuleComplianceCompleteness(%q)=%.3f want %.3f", p, got, want)
		}
	}
	// All three rules referenced → 1.0.
	check("已检查 NOV-001、NOV-002 和 A22.3", 1.0)
	// Two of three → 0.667.
	check("适用 NOV-001 与 A22.3", 2.0/3.0)
	// None → 0.
	check("未提及任何规则", 0)
}

func TestRuleComplianceCompleteness_EmptyRequired(t *testing.T) {
	m := RuleComplianceCompleteness{}
	if got := m.Compute("anything", ""); got != 1 {
		t.Errorf("empty Required should return 1, got %.3f", got)
	}
}

func TestRuleComplianceCompleteness_CaseInsensitive(t *testing.T) {
	m := RuleComplianceCompleteness{Required: []string{"NOV-001"}}
	// Lowercase prediction should still match uppercase rule ID.
	if got := m.Compute("checked nov-001", ""); !approxEqual(got, 1) {
		t.Errorf("case-insensitive match failed: %.3f", got)
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
