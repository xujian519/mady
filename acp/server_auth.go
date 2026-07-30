package acp

import (
	"context"
	"fmt"
	"path/filepath"
)

// ---------------------------------------------------------------------------
// Authentication helpers: authRequired, noopAuthProvider, sanitizeCWD

// authRequired 报告服务端是否配置了需要客户端认证的 AuthProvider。
// 未声明任何认证方式（如本地开发的 noop provider）时返回 false。
func (s *Server) authRequired() bool {
	return len(s.authProv.AuthMethods()) > 0
}

// noopAuthProvider is the default AuthProvider for local development. It
// advertises no authentication methods and always rejects authentication
// attempts, so the server's authenticated gate remains disabled.
type noopAuthProvider struct{}

func (n *noopAuthProvider) AuthMethods() []any {
	return []any{}
}

func (n *noopAuthProvider) Authenticate(_ context.Context, _ AuthenticateParams) (*AuthenticateResult, error) {
	return nil, fmt.Errorf("authentication not configured: no auth provider registered")
}

// sanitizeCWD 清洗 CWD 路径，防止目录遍历（../）攻击。
// 空 CWD 默认 "."；经 filepath.Clean 归一化后转为绝对路径。
func sanitizeCWD(cwd string) (string, error) {
	if cwd == "" {
		cwd = "."
	}
	cleaned := filepath.Clean(cwd)
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("cwd abs: %w", err)
	}
	return abs, nil
}
