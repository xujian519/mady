package acp

import (
	"context"
	"encoding/json"
	"fmt"
)

// ---------------------------------------------------------------------------
// Request dispatch: handleRequest

func (s *Server) handleRequest(ctx context.Context, req *JSONRPCRequest) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("panic in ACP handler: %v", r)
			s.logger.Error("ACP handler panic", "err", err)
			s.writeError(req.ID, -32603, "Internal error", err.Error())
		}
	}()
	// 认证门禁：AuthProvider 声明了认证方式时，initialize/authenticate
	// 之外的所有方法必须先完成 authenticate，否则拒绝（fail-closed）。
	if s.authRequired() && !s.authenticated.Load() &&
		req.Method != "initialize" && req.Method != "authenticate" {
		s.writeError(req.ID, -32000, "Authentication required", req.Method)
		return
	}
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "authenticate":
		s.handleAuthenticate(ctx, req)
	case "session/new":
		s.handleNewSession(ctx, req)
	case "session/load":
		s.handleLoadSession(ctx, req)
	case "session/resume":
		s.handleResumeSession(ctx, req)
	case "session/fork":
		s.handleForkSession(ctx, req)
	case "session/list":
		s.handleListSessions(req)
	case "session/prompt":
		s.handlePrompt(ctx, req)
	case "session/cancel":
		s.handleCancel(req)
	case "session/set_mode":
		s.handleSetMode(req)
	case "session/set_model":
		s.handleSetModel(req)
	default:
		s.logger.Warn("unknown ACP method", "method", req.Method)
		s.writeError(req.ID, -32601, "Method not found", req.Method)
	}
}

// ---------------------------------------------------------------------------
// Method handlers: handleInitialize, handleAuthenticate, handleNewSession, ...

func (s *Server) handleInitialize(req *JSONRPCRequest) {
	var params InitializeParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}

	clientName := "unknown"
	if params.ClientInfo != nil {
		clientName = params.ClientInfo.Name
	}
	// Validate client capabilities against server-configured allowlist.
	// Unrecognized or unapproved capabilities are rejected (fail-closed).
	if err := s.validateClientCapabilities(params.ClientCapabilities); err != nil {
		s.writeError(req.ID, -32602, "Client capabilities rejected", err.Error())
		return
	}
	s.clientCaps.Store(params.ClientCapabilities)
	s.logger.Info("ACP initialize", "client", clientName)

	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		AgentCapabilities: AgentCapabilities{
			LoadSession: true,
			PromptCapabilities: &PromptCapabilities{
				Image: false,
			},
			SessionCapabilities: &SessionCapabilities{
				Fork:   &SessionForkCapabilities{},
				List:   &SessionListCapabilities{},
				Resume: &SessionResumeCapabilities{},
			},
		},
		AuthMethods: s.authProv.AuthMethods(),
	}

	s.writeResponse(req.ID, result)
}

func (s *Server) handleAuthenticate(ctx context.Context, req *JSONRPCRequest) {
	var params AuthenticateParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}

	// Delegate to the configured AuthProvider. If none is configured,
	// authentication is rejected (fail-closed).
	if s.authProv == nil {
		s.writeError(req.ID, -32001, "Authentication not configured", "no auth provider")
		return
	}
	result, err := s.authProv.Authenticate(ctx, params)
	if err != nil {
		s.writeError(req.ID, -32001, "Authentication failed", err.Error())
		return
	}
	// 认证成功：放行后续所有方法的认证门禁。
	s.authenticated.Store(true)
	s.writeResponse(req.ID, result)
}

// validateClientCapabilities checks that client-declared FS capabilities are in
// the server-configured allowlist. If no allowlist is configured (nil map), all
// FS capabilities are rejected (fail-closed). Terminal capability is always accepted.
func (s *Server) validateClientCapabilities(caps *ClientCapabilities) error {
	if caps == nil {
		return nil // no capabilities declared, nothing to validate
	}
	if caps.FS != nil {
		if s.allowedFSCaps == nil {
			return fmt.Errorf("filesystem capabilities are not allowed on this server")
		}
		if caps.FS.ReadTextFile && !s.allowedFSCaps["FS.ReadTextFile"] {
			return fmt.Errorf("ReadTextFile capability is not allowed")
		}
		if caps.FS.WriteTextFile && !s.allowedFSCaps["FS.WriteTextFile"] {
			return fmt.Errorf("WriteTextFile capability is not allowed")
		}
	}
	return nil
}
