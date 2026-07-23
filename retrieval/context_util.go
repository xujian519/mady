package retrieval

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// ShouldTrigger checks if retrieval should fire based on the trigger policy.
// For TriggerSmart, call ShouldTriggerSmart afterward to check the complexity
// classifier. turnCount starts from 0.
func ShouldTrigger(policy TriggerPolicy, turnCount int, firstNTurns int) bool {
	switch policy {
	case TriggerFirstN:
		return turnCount < firstNTurns
	case TriggerOnDemand:
		return false
	default: // TriggerAlways, TriggerSmart (needs additional check)
		return true
	}
}

// ShouldTriggerSmart refines TriggerSmart policy using ComplexityClassifier.
// Returns true when the query complexity is Medium or Higher.
func ShouldTriggerSmart(query string, messages []agentcore.Message, classifier ComplexityClassifier) bool {
	if classifier == nil {
		return true // fallback to always
	}
	if query == "" {
		return false
	}
	c := classifier.Classify(query, messages)
	return c >= agentcore.ComplexityMedium
}

// FormatContextBlock formats retrieved chunks into a single context string,
// respecting the MaxChars budget in config.
func FormatContextBlock(results []ScoredChunk, cfg RetrievalConfig) string {
	var b strings.Builder

	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "以下是检索到的相关参考信息，请在回答时参考：\n"
	}
	b.WriteString(prefix)

	totalChars := 0
	for i, r := range results {
		if totalChars >= cfg.MaxChars {
			break
		}

		chunkText := r.Content
		if totalChars+len(chunkText) > cfg.MaxChars {
			chunkText = chunkText[:cfg.MaxChars-totalChars] + "..."
		}

		fmt.Fprintf(&b, "\n--- 参考片段 %d (相关度: %.2f) ---\n", i+1, r.Score)
		if cfg.DomainHint != "" {
			fmt.Fprintf(&b, "[来源: %s/%s]\n", cfg.DomainHint, r.DocID)
		}
		b.WriteString(chunkText)
		b.WriteString("\n")
		totalChars += len(chunkText) + 80 // 80 for header overhead (actual 56-76 bytes)
	}

	return b.String()
}

// InjectContext prepends the retrieval context as a system message,
// inserted after the last existing system message.
// Deduplication: skips injection when a system message already starts
// with the same prefix (fix 4.6).
func InjectContext(req *agentcore.ProviderRequest, contextBlock string) {
	if contextBlock == "" {
		return
	}

	// Deduplication check: skip if knowledge context already injected.
	if hasKnowledgeContext(req.Messages, contextBlock) {
		return
	}

	sysMsg := agentcore.Message{
		Role:    agentcore.RoleSystem,
		Content: contextBlock,
	}

	insertIdx := 0
	for i, msg := range req.Messages {
		if msg.Role == agentcore.RoleSystem {
			insertIdx = i + 1
		}
	}

	req.Messages = append(
		req.Messages[:insertIdx],
		append([]agentcore.Message{sysMsg}, req.Messages[insertIdx:]...)...,
	)
}

// hasKnowledgeContext checks if any system message already contains the
// knowledge context block content (by prefix match on the first 40 chars).
func hasKnowledgeContext(msgs []agentcore.Message, contextBlock string) bool {
	if len(contextBlock) == 0 {
		return false
	}
	prefixLen := 40
	if len(contextBlock) < prefixLen {
		prefixLen = len(contextBlock)
	}
	prefix := contextBlock[:prefixLen]
	for _, msg := range msgs {
		if msg.Role == agentcore.RoleSystem && len(msg.Content) >= prefixLen &&
			msg.Content[:prefixLen] == prefix {
			return true
		}
	}
	return false
}
