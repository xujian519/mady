package acp

import (
	"encoding/json"
	"fmt"
)

// ---------------------------------------------------------------------------
// Response helpers: writeResponse, writeNotification, writeError
//
// These write JSON-RPC response frames to the server's writer under the
// writerMu lock.

func (s *Server) writeResponse(id any, result any) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		s.writeError(id, -32603, "Internal error", err.Error())
		return
	}
	resp.Result = resultBytes

	data, marshalErr := json.Marshal(resp)
	if marshalErr != nil {
		s.logger.Error("acp: failed to marshal response", "err", marshalErr)
		return
	}
	s.writerMu.Lock()
	_, _ = fmt.Fprintf(s.writer, "%s\n", data)
	s.writerMu.Unlock()
}

func (s *Server) writeNotification(method string, params any) {
	paramsBytes, marshalErr := json.Marshal(params)
	if marshalErr != nil {
		s.logger.Error("acp: failed to marshal notification params", "err", marshalErr)
		return
	}
	notif := JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsBytes,
	}
	data, marshalErr2 := json.Marshal(notif)
	if marshalErr2 != nil {
		s.logger.Error("acp: failed to marshal notification", "err", marshalErr2)
		return
	}
	s.writerMu.Lock()
	_, _ = fmt.Fprintf(s.writer, "%s\n", data)
	s.writerMu.Unlock()
}

func (s *Server) writeError(id any, code int, message string, data any) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	respData, marshalErr := json.Marshal(resp)
	if marshalErr != nil {
		s.logger.Error("acp: failed to marshal error response", "err", marshalErr)
		return
	}
	s.writerMu.Lock()
	_, _ = fmt.Fprintf(s.writer, "%s\n", respData)
	s.writerMu.Unlock()
}
