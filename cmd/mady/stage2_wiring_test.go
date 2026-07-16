package main

import (
	"testing"

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
