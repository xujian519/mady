package guardian

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionName is the registration name for the guardian extension.
const ExtensionName = "guardian"

// GuardianExtension wraps a Guardian Session as an agentcore Extension.
// It registers as a Middleware that intercepts non-read-only tool calls
// for AI safety review.
type GuardianExtension struct {
	session *Session
	agent   *agentcore.Agent
}

var (
	_ agentcore.Extension          = (*GuardianExtension)(nil)
	_ agentcore.MiddlewareProvider = (*GuardianExtension)(nil)
)

// NewExtension creates a Guardian extension from a Session.
func NewExtension(session *Session) *GuardianExtension {
	return &GuardianExtension{session: session}
}

// Name implements agentcore.Extension.
func (e *GuardianExtension) Name() string { return ExtensionName }

// Init implements agentcore.Extension.
func (e *GuardianExtension) Init(_ context.Context, agent *agentcore.Agent) error {
	e.agent = agent
	return nil
}

// Dispose implements agentcore.Extension.
func (e *GuardianExtension) Dispose() error { return nil }

// Middleware implements agentcore.MiddlewareProvider.
func (e *GuardianExtension) Middleware() []agentcore.Middleware {
	if e.session == nil {
		return nil
	}
	return []agentcore.Middleware{e.guardianMiddleware}
}

func (e *GuardianExtension) guardianMiddleware(next agentcore.ExecuteFunc) agentcore.ExecuteFunc {
	return func(ctx context.Context, tc agentcore.ToolCall) (string, error) {
		readOnly := false
		if e.agent != nil {
			if tool, ok := e.agent.GetTool(tc.Name); ok {
				readOnly = agentcore.ToolReadOnly(tool, json.RawMessage(tc.Arguments))
			}
		}

		if !e.session.shouldReview(tc.Name, readOnly) {
			return next(ctx, tc)
		}

		transcript := extractTranscript(ctx)

		assessment, err := e.session.Review(ctx, tc.Name, json.RawMessage(tc.Arguments), transcript)
		if err != nil {
			return fmt.Sprintf("blocked: 安全审查失败 — %v", err), nil
		}

		if assessment.IsDenied() {
			reason := assessment.Rationale
			if reason == "" {
				reason = "安全审查未通过"
			}
			return fmt.Sprintf("blocked: %s", reason), nil
		}

		return next(ctx, tc)
	}
}

type transcriptKey struct{}

// WithTranscript injects a conversation transcript into the context for
// the Guardian to use as evidence during review.
func WithTranscript(ctx context.Context, transcript string) context.Context {
	return context.WithValue(ctx, transcriptKey{}, transcript)
}

func extractTranscript(ctx context.Context) string {
	if v, ok := ctx.Value(transcriptKey{}).(string); ok {
		return v
	}
	return ""
}

// FormatTranscript builds a compact transcript from recent messages.
// Only the last maxMessages are included to keep the review prompt short.
func FormatTranscript(msgs []agentcore.Message, maxMessages int) string {
	if len(msgs) == 0 {
		return ""
	}
	if maxMessages > 0 && len(msgs) > maxMessages {
		msgs = msgs[len(msgs)-maxMessages:]
	}
	var sb strings.Builder
	for _, m := range msgs {
		if m.Content == "" {
			continue
		}
		sb.WriteString(string(m.Role))
		sb.WriteString(": ")
		content := m.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	return sb.String()
}
