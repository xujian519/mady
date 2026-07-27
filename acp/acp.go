package acp

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// AgentInfo identifies an ACP server implementation to connected clients.
// AgentInfo describes the agent identity for the ACP protocol handshake.
type AgentInfo struct {
	Name    string
	Version string
}

// AuthProvider validates client credentials during the authenticate handshake.
// AuthProvider authenticates incoming ACP connections.
type AuthProvider interface {
	AuthMethods() []any
	// Authenticate validates the given credentials and returns a result.
	// An error indicates authentication was rejected.
	Authenticate(ctx context.Context, params AuthenticateParams) (*AuthenticateResult, error)
}

// AgentFactory creates AgentInstance values for new and restored sessions.
// AgentFactory creates and configures agent instances for ACP sessions.
type AgentFactory interface {
	CreateAgent(ctx context.Context, sessionID, cwd, model, mode string) (AgentInstance, error)
	DefaultModel() string
	DefaultMode() string
	AvailableModes() []SessionMode
}

// AgentInstance wraps a running agent with its model and mode metadata.
// AgentInstance represents a running agent instance within an ACP session.
type AgentInstance interface {
	Run(ctx context.Context, input string) (string, error)
	Core() *agentcore.Agent
	Model() string
	Mode() string
}

// PermissionAware is an optional interface an AgentInstance can implement to
// route tool-call authorization to the ACP client (editor). When set, the
// instance consults fn before executing a tool that requires permission; fn
// returns true to allow, false to deny.
type PermissionAware interface {
	SetPermissionRequester(fn func(toolCallID, name string, rawInput any) bool)
}

// FileSystem is the editor-backed filesystem exposed to an agent instance so
// its read/write tools can see unsaved buffers via the ACP client.
type FileSystem interface {
	ReadTextFile(path string) ([]byte, error)
	WriteTextFile(path string, content []byte) error
}

// Rebuildable is an optional interface an AgentInstance can implement to
// support runtime mode/model changes. When not implemented, SetSessionMode
// and SetSessionModel only update persisted metadata.
type Rebuildable interface {
	Rebuild(mode, model string) error
}

// FileSystemAware is an optional interface an AgentInstance can implement to
// route file reads/writes through the ACP client (editor). Set only when the
// client advertised filesystem capability.
type FileSystemAware interface {
	SetFileSystem(fs FileSystem)
}

// SessionStore persists and retrieves session metadata.
// SessionStore persists and retrieves session metadata to and from disk.
type SessionStore interface {
	LoadSessionMeta(sessionID string) (SessionMeta, error)
	SaveSessionMeta(meta SessionMeta) error
	ListSessions(cwd string) []SessionMeta
}

// SessionMeta records the persistent metadata for a single ACP session.
// SessionMeta contains the persisted metadata for an ACP session.
type SessionMeta struct {
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
	Model     string `json:"model"`
	Mode      string `json:"mode"`
	Title     string `json:"title,omitempty"`
	UpdatedAt string `json:"updated_at"`
}
