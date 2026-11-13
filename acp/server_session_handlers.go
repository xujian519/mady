package acp

import (
	"context"
	"encoding/json"
)

func (s *Server) handleNewSession(ctx context.Context, req *JSONRPCRequest) {
	var params NewSessionParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			// 参数解析失败→回写 Invalid params 错误帧，协议面不返回 Go error。
			s.writeError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}

	cwd, err := sanitizeCWD(params.CWD)
	if err != nil {
		// 工作目录不可用（越界/不存在）→回写 Invalid CWD 错误帧，不进入会话创建。
		s.writeError(req.ID, -32602, "Invalid CWD", err.Error())
		return
	}

	state, err := s.sessionMgr.CreateSession(ctx, cwd, "")
	if err != nil {
		// 创建失败→记 Error 并回写 Internal error 错误帧，不向调用栈上抛。
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
			// 参数解析失败→回写 Invalid params 错误帧，协议面不返回 Go error。
			s.writeError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}

	state := s.sessionMgr.UpdateCWD(params.SessionID, params.CWD)
	if state == nil {
		_, err := s.sessionMgr.RestoreSession(ctx, params.SessionID)
		if err != nil {
			// 历史会话无法恢复→回写 Session not found 错误帧（load 语义要求严格存在）。
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
			// 参数解析失败→回写 Invalid params 错误帧，协议面不返回 Go error。
			s.writeError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}

	state := s.sessionMgr.UpdateCWD(params.SessionID, params.CWD)
	if state == nil {
		_, err := s.sessionMgr.RestoreSession(ctx, params.SessionID)
		if err != nil {
			// 历史会话不存在→按 resume 语义降级为新建会话，已记 Warn。
			s.logger.Warn("resume session not found, creating new", "session_id", params.SessionID)
			state, err = s.sessionMgr.CreateSession(ctx, params.CWD, "")
			if err != nil {
				// 兜底新建也失败→回写 Internal error 错误帧。
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
			// 参数解析失败→回写 Invalid params 错误帧，协议面不返回 Go error。
			s.writeError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
	}

	cwd, err := sanitizeCWD(params.CWD)
	if err != nil {
		// 工作目录不可用（越界/不存在）→回写 Invalid CWD 错误帧，不进入会话创建。
		s.writeError(req.ID, -32602, "Invalid CWD", err.Error())
		return
	}

	state, err := s.sessionMgr.ForkSession(ctx, params.SessionID, cwd)
	if err != nil {
		// 分叉失败（源会话不存在等）→记 Error 并回写 Internal error 错误帧。
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
			// 参数解析失败→回写 Invalid params 错误帧，协议面不返回 Go error。
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
