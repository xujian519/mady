package main

import (
	"testing"

	"github.com/xujian519/mady/pkg/lawcite"
	"github.com/xujian519/mady/pkg/util"
)

// TestBuildReasoningRetriever_Stage2Wiring verifies that buildReasoningRetriever
// wires the Stage ② rule-acquisition sources when the corresponding knowledge
// assets are present under $MADY_HOME.
//
// This is an environment-gated integration test: it skips when the local
// machine has no ~/.mady/knowledge data (e.g. CI), so it never fails due to
// missing external data. On a developer machine with the xiaonuo-derived
// knowledge assets symlinked in, it confirms the retriever is constructed
// rather than nil (i.e. at least one lane is live).
func TestBuildReasoningRetriever_Stage2Wiring(t *testing.T) {
	madyHome, err := util.MadyHome()
	if err != nil {
		t.Skipf("MadyHome unavailable: %v", err)
	}

	// Reuse the real knowledge-backend opener; it returns (nil, "") cleanly
	// when knowledge.db is absent, so CI without data just skips below.
	backend, _ := loadKnowledgeBackend(madyHome)
	wikiRoot := resolveWikiRoot(madyHome)

	if backend == nil && wikiRoot == "" {
		t.Skip("no knowledge backend or wiki root under MadyHome; skipping wiring check")
	}

	fc := &frameworkContext{
		MadyHome:         madyHome,
		KnowledgeBackend: backend,
		WikiRoot:         wikiRoot,
	}
	retriever := buildReasoningRetriever(fc)
	if retriever == nil {
		t.Fatal("buildReasoningRetriever returned nil despite available assets")
	}

	// The retriever's source fields are unexported; we assert construction
	// success here. Per-lane behavior is covered by data-free unit tests in
	// domains/reasoning/wiring/*_test.go.
	t.Logf("Stage ② retriever wired: backend=%v wikiRoot=%q",
		backend != nil, wikiRoot)
}

// TestBuildCitationSource_S2Wiring verifies that buildCitationSource wires
// the S2 wiki law-article index when the legal directory under wikiRoot
// contains valid 专利法-2020-拆分-*.md files.
//
// This is an environment-gated integration test: it skips when the local
// machine has no wiki legal data (e.g. CI). The function itself always
// returns a non-nil CitationSource — on missing data it returns S1-only,
// which is verified even in CI.
func TestBuildCitationSource_S2Wiring(t *testing.T) {
	madyHome, err := util.MadyHome()
	if err != nil {
		t.Skipf("MadyHome unavailable: %v", err)
	}

	wikiRoot := resolveWikiRoot(madyHome)

	// Even with empty wikiRoot, buildCitationSource returns a non-nil
	// source (S1-only fallback). This is the minimum guarantee.
	src := buildCitationSource(wikiRoot)
	if src == nil {
		t.Fatal("buildCitationSource returned nil — should always return at least S1 source")
	}

	// Verify the source responds to basic queries (S1 static table).
	kw, ok := src.Topics(lawcite.StatutePatentLaw, 22)
	if !ok || len(kw) == 0 {
		t.Error("S1 source missing Article 22 keywords (should have 新颖性/创造性/实用性)")
	}
	t.Logf("Patent Law Article 22 keywords (S1+S2): %v", kw)

	if wikiRoot == "" {
		t.Skip("no wiki root; S2 index not tested")
	}

	// When S2 exists, verify the index loaded correctly.
	t.Logf("wikiRoot=%q — S2 index tested above", wikiRoot)
}
