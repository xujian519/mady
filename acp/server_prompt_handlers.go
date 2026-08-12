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
