package acp

import (
	"encoding/json"
	"time"
)

// ---------------------------------------------------------------------------
// File operations (via client): clientSupportsFS, ReadTextFile,
// WriteTextFile, sessionFS

// clientSupportsFS reports whether the client advertised filesystem capability,
// meaning the agent should read/write through the editor (seeing unsaved
// buffers) instead of touching disk directly.
func (s *Server) clientSupportsFS() bool {
	caps := s.clientCaps.Load()
	return caps != nil && caps.FS != nil &&
		(caps.FS.ReadTextFile || caps.FS.WriteTextFile)
}

// ReadTextFile reads a file through the client (editor), seeing unsaved buffers.
func (s *Server) ReadTextFile(sessionID, path string) ([]byte, error) {
	raw, err := s.sendRequest("fs/read_text_file", map[string]any{
		"sessionId": sessionID,
		"path":      path,
	}, 30*time.Second)
	if err != nil {
		return nil, err
	}
	var res struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return []byte(res.Content), nil
}

// WriteTextFile writes a file through the client (editor).
func (s *Server) WriteTextFile(sessionID, path string, content []byte) error {
	_, err := s.sendRequest("fs/write_text_file", map[string]any{
		"sessionId": sessionID,
		"path":      path,
		"content":   string(content),
	}, 30*time.Second)
	return err
}

// sessionFS adapts the server's fs methods to the per-session FileSystem.
type sessionFS struct {
	server    *Server
	sessionID string
}

func (f *sessionFS) ReadTextFile(path string) ([]byte, error) {
	return f.server.ReadTextFile(f.sessionID, path)
}

func (f *sessionFS) WriteTextFile(path string, content []byte) error {
	return f.server.WriteTextFile(f.sessionID, path, content)
}
