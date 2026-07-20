package desktop

// toolResult is the internal result type for computer use tools.
type toolResult struct {
	Content string `json:"content"`
	Details any    `json:"details,omitempty"`
}

func result(content string, details any) (any, error) {
	return toolResult{Content: content, Details: details}, nil
}
