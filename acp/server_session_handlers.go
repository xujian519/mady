package acp

import (
	"context"
	"encoding/json"
)

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
