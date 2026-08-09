package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"

	"github.com/xujian519/mady/domains"
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

func (s *Server) handleNewSession(ctx context.Context, req *JSONRPCRequest) {
	var params NewSessionParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}

	cwd, err := sanitizeCWD(params.CWD)
	if err != nil {
		s.writeError(req.ID, -32602, "Invalid CWD", err.Error())
		return
	}

	state, err := s.sessionMgr.CreateSession(ctx, cwd, "")
	if err != nil {
		s.logger.Error("create session failed", "err", err)
		s.writeError(req.ID, -32603, "Internal error", err.Error())
		return
	}

	result := NewSessionResult{
		SessionID: state.SessionID,
		Models:    s.buildModelState(state),
		Modes:     s.buildModeState(),
	}

	s.writeResponse(req.ID, result)
}

func (s *Server) handleLoadSession(ctx context.Context, req *JSONRPCRequest) {
	var params LoadSessionParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}

	state := s.sessionMgr.UpdateCWD(params.SessionID, params.CWD)
	if state == nil {
		_, err := s.sessionMgr.RestoreSession(ctx, params.SessionID)
		if err != nil {
			s.writeError(req.ID, -32002, "Session not found", params.SessionID)
			return
		}
		state = s.sessionMgr.UpdateCWD(params.SessionID, params.CWD)
	}

	result := LoadSessionResult{
		Models: s.buildModelState(state),
		Modes:  s.buildModeState(),
	}

	s.writeResponse(req.ID, result)
}

func (s *Server) handleResumeSession(ctx context.Context, req *JSONRPCRequest) {
	var params ResumeSessionParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}

	state := s.sessionMgr.UpdateCWD(params.SessionID, params.CWD)
	if state == nil {
		_, err := s.sessionMgr.RestoreSession(ctx, params.SessionID)
		if err != nil {
			s.logger.Warn("resume session not found, creating new", "session_id", params.SessionID)
			state, err = s.sessionMgr.CreateSession(ctx, params.CWD, "")
			if err != nil {
				s.writeError(req.ID, -32603, "Internal error", err.Error())
				return
			}
		} else {
			state = s.sessionMgr.UpdateCWD(params.SessionID, params.CWD)
		}
	}

	result := ResumeSessionResult{
		Models: s.buildModelState(state),
		Modes:  s.buildModeState(),
	}

	s.writeResponse(req.ID, result)
}

func (s *Server) handleForkSession(ctx context.Context, req *JSONRPCRequest) {
	var params ForkSessionParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}

	cwd, err := sanitizeCWD(params.CWD)
	if err != nil {
		s.writeError(req.ID, -32602, "Invalid CWD", err.Error())
		return
	}

	state, err := s.sessionMgr.ForkSession(ctx, params.SessionID, cwd)
	if err != nil {
		s.logger.Error("fork session failed", "err", err)
		s.writeError(req.ID, -32603, "Internal error", err.Error())
		return
	}

	result := ForkSessionResult{
		SessionID: state.SessionID,
		Models:    s.buildModelState(state),
		Modes:     s.buildModeState(),
	}

	s.writeResponse(req.ID, result)
}

func (s *Server) handleListSessions(req *JSONRPCRequest) {
	var params ListSessionsParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}

	sessions := s.sessionMgr.ListSessions(params.CWD)

	result := ListSessionsResult{
		Sessions: sessions,
	}

	s.writeResponse(req.ID, result)
}

func (s *Server) handlePrompt(ctx context.Context, req *JSONRPCRequest) {
	var params struct {
		SessionID string            `json:"sessionId"`
		Prompt    []json.RawMessage `json:"prompt"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}

	state := s.sessionMgr.GetSession(params.SessionID)
	if state == nil {
		s.logger.Error("prompt session not found", "session_id", params.SessionID)
		s.writeResponse(req.ID, PromptResponse{StopReason: "refusal"})
		return
	}

	// Route tool-call authorization to the client (editor) when supported.
	if pa, ok := state.Agent.(PermissionAware); ok {
		sid := params.SessionID
		pa.SetPermissionRequester(func(toolCallID, name string, rawInput any) bool {
			outcome, err := s.RequestPermission(sid, PermissionToolCall{
				ToolCallID: toolCallID,
				Title:      name,
				Kind:       ToolKind(name),
				Status:     "pending",
				RawInput:   rawInput,
			}, DefaultPermissionOptions())
			if err != nil || outcome == nil || outcome.Outcome != "selected" {
				// error/canceled → deny (these are dangerous-tool gates)
				s.recordPermissionDecision(ctx, sid, name, rawInput, domains.DecisionRejected, "canceled_or_error")
				return false
			}
			allow := strings.HasPrefix(outcome.OptionID, "allow")
			s.recordPermissionDecision(ctx, sid, name, rawInput, permissionDecisionFor(allow), outcome.OptionID)
			return allow
		})
	}

	// Route file reads/writes through the editor when the client supports it.
	if fsa, ok := state.Agent.(FileSystemAware); ok && s.clientSupportsFS() {
		fsa.SetFileSystem(&sessionFS{server: s, sessionID: params.SessionID})
	}

	if state.IsRunning() {
		s.logger.Warn("session already running, canceling previous", "session_id", params.SessionID)
		state.Cancel()
		s.sessionMgr.SetIdle(params.SessionID)
	}

	userText := extractPromptContent(params.Prompt)
	if strings.TrimSpace(userText) == "" {
		s.writeResponse(req.ID, PromptResponse{StopReason: "end_turn"})
		return
	}

	s.logger.Info("ACP prompt", "session_id", params.SessionID, "text", truncateStr(userText, 100))

	s.sendNotification(params.SessionID, "session/update", SessionNotification{
		SessionID: params.SessionID,
		Update: SessionUpdate{
			SessionUpdate: "user_message_chunk",
			Content:       TextContentBlock{Type: "text", Text: userText},
		},
	})

	agentCtx, cancel := context.WithCancel(ctx)
	s.sessionMgr.SetRunning(params.SessionID, cancel)

	core := state.Agent.Core()
	unregisterEvents := RegisterEventListeners(params.SessionID, core, func(method string, p any) {
		s.sendNotification(params.SessionID, method, p)
	})

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Default().Error("[acp] agent run goroutine panicked", "panic", r, "stack", string(debug.Stack()))
			}
		}()
		defer unregisterEvents()
		defer func() {
			s.sessionMgr.SetIdle(params.SessionID)
		}()

		result, err := state.Agent.Run(agentCtx, userText)
		if err != nil {
			s.logger.Error("agent run failed", "err", err)
		}

		if result != "" {
			s.sendNotification(params.SessionID, "session/update", SessionNotification{
				SessionID: params.SessionID,
				Update: SessionUpdate{
					SessionUpdate: "agent_message_chunk",
					Content:       TextContentBlock{Type: "text", Text: result},
				},
			})
		}

		// 透传模型结束原因，让客户端能判断输出是否因 max_tokens 截断
		// 或流异常而可能不完整。
		finishReason := ""
		if core := state.Agent.Core(); core != nil {
			finishReason = core.LastFinishReason()
		}

		s.writeResponse(req.ID, PromptResponse{
			StopReason:   "end_turn",
			FinishReason: finishReason,
		})
	}()
}

func (s *Server) handleCancel(req *JSONRPCRequest) {
	var params CancelParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}

	state := s.sessionMgr.GetSession(params.SessionID)
	if state != nil {
		state.Cancel()
		s.logger.Info("ACP session canceled", "session_id", params.SessionID)
	}

	s.writeResponse(req.ID, nil)
}

// sessionUpdateStep handles the common three-step pattern for updating a
// session property: unmarshal params → validate session exists → update → rebuild agent.
func (s *Server) sessionUpdateStep(req *JSONRPCRequest, label string,
	unmarshalFn func(data []byte) (sessionID, value string, err error),
	updateFn func(sessionID, value string) error,
	rebuildFn func(state *sessionState, value string) error,
) {
	sessionID, value, err := unmarshalFn(req.Params)
	if err != nil {
		s.writeError(req.ID, -32602, "Invalid params", err.Error())
		return
	}

	if s.sessionMgr.GetSession(sessionID) == nil {
		s.writeError(req.ID, -32002, "Session not found", sessionID)
		return
	}

	if err := updateFn(sessionID, value); err != nil {
		s.writeError(req.ID, -32003, fmt.Sprintf("Update %s failed", label), err.Error())
		return
	}

	// Rebuild agent so the new config takes effect
	state := s.sessionMgr.GetSession(sessionID)
	if _, ok := state.Agent.(Rebuildable); ok {
		if err := rebuildFn(state, value); err != nil {
			slog.Error("rebuild agent for "+label+" change", "err", err)
		}
	}

	s.writeResponse(req.ID, nil)
}

func (s *Server) handleSetMode(req *JSONRPCRequest) {
	s.sessionUpdateStep(req, "mode",
		func(data []byte) (string, string, error) {
			var p SetSessionModeParams
			if data != nil {
				if err := json.Unmarshal(data, &p); err != nil {
					return "", "", err
				}
			}
			return p.SessionID, p.ModeID, nil
		},
		s.sessionMgr.UpdateMode,
		func(st *sessionState, v string) error {
			return st.Agent.(Rebuildable).Rebuild(v, st.Model)
		},
	)
}

func (s *Server) handleSetModel(req *JSONRPCRequest) {
	s.sessionUpdateStep(req, "model",
		func(data []byte) (string, string, error) {
			var p SetSessionModelParams
			if data != nil {
				if err := json.Unmarshal(data, &p); err != nil {
					return "", "", err
				}
			}
			return p.SessionID, p.ModelID, nil
		},
		s.sessionMgr.UpdateModel,
		func(st *sessionState, v string) error {
			return st.Agent.(Rebuildable).Rebuild(st.Mode, v)
		},
	)
}

// ---------------------------------------------------------------------------
// State builders: buildModelState, buildModeState, sendNotification

func (s *Server) buildModelState(state *sessionState) *SessionModelState {
	if state == nil {
		return nil
	}
	return &SessionModelState{
		AvailableModels: []ModelInfo{
			{
				ModelID:     state.Model,
				Name:        state.Model,
				Description: "current",
			},
		},
		CurrentModelID: state.Model,
	}
}

func (s *Server) buildModeState() *SessionModeState {
	modes := s.sessionMgr.agentFactory.AvailableModes()
	return &SessionModeState{
		CurrentModeID:  s.sessionMgr.agentFactory.DefaultMode(),
		AvailableModes: modes,
	}
}

func (s *Server) sendNotification(_ string, method string, params any) {
	s.writeNotification(method, params)
}

// ---------------------------------------------------------------------------
// Utilities: extractPromptContent, validateClientCapabilities

// extractPromptContent flattens an ACP prompt (an array of content blocks) into
// a single text prompt. It handles every block type Zed may send: plain text,
// embedded resources (file contents inlined), resource links (file references),
// and images/audio (acknowledged so the model knows an attachment was sent).
func extractPromptContent(blocks []json.RawMessage) string {
	var parts []string
	for _, raw := range blocks {
		var b struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			URI      string `json:"uri"`
			Name     string `json:"name"`
			Resource *struct {
				URI  string `json:"uri"`
				Text string `json:"text"`
			} `json:"resource"`
		}
		if err := json.Unmarshal(raw, &b); err != nil {
			continue
		}
		switch b.Type {
		case "text":
			if b.Text != "" {
				parts = append(parts, b.Text)
			}
		case "resource":
			if b.Resource == nil {
				continue
			}
			if b.Resource.Text != "" {
				parts = append(parts, fmt.Sprintf("<file uri=%q>\n%s\n</file>", b.Resource.URI, b.Resource.Text))
			} else if b.Resource.URI != "" {
				parts = append(parts, fmt.Sprintf("[referenced resource: %s]", b.Resource.URI))
			}
		case "resource_link":
			if b.URI == "" {
				continue
			}
			if b.Name != "" {
				parts = append(parts, fmt.Sprintf("[referenced file: %s (%s)]", b.Name, b.URI))
			} else {
				parts = append(parts, fmt.Sprintf("[referenced file: %s]", b.URI))
			}
		case "image":
			parts = append(parts, "[image attached]")
		case "audio":
			parts = append(parts, "[audio attached]")
		}
	}
	return strings.Join(parts, "\n")
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
