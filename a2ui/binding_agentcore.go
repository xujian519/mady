package a2ui

import (
	"log/slog"

	"github.com/xujian519/mady/agentcore"
)

// ToAgentCoreEvent converts an A2UI envelope into an agentcore.A2UIEvent
// suitable for emission on the agent event bus. The converter in agui will
// turn it into a CUSTOM event with name "a2ui" for the AG-UI stream.
func ToAgentCoreEvent(env Envelope) *agentcore.A2UIEvent {
	if env.Version == "" {
		env.Version = Version
	}
	// envelopeToMap serializes the struct to a generic map so the event bus
	// can carry it without depending on a2ui types. See binding_a2a.go.
	m, err := envelopeToMap(env)
	if err != nil {
		slog.Default().Warn("a2ui: envelopeToMap failed", "err", err)
		m = map[string]any{"error": err.Error(), "version": env.Version}
	}
	return agentcore.NewA2UIEvent(m)
}
