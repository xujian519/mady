package graph

import (
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// StepStreamConcat concatenates terminal node output streams into a single
// stream. Each chunk from the merged terminal streams is forwarded as-is.
// Useful for streaming graph execution to get a unified output view.
func StepStreamConcat(streams ...*agentcore.StreamReader[string]) *agentcore.StreamReader[string] {
	return agentcore.Merge(streams...)
}

// StreamChunkCollector drains a stream and joins chunks with the given separator.
// This is useful for turning a streaming graph result back into a complete string.
func StreamChunkCollector(s *agentcore.StreamReader[string], sep string) (string, error) {
	items, err := s.Collect()
	if err != nil {
		return "", err
	}
	return strings.Join(items, sep), nil
}
